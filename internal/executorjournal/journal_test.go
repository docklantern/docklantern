package executorjournal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var fixedTime = time.Date(2026,11,3,20,0,0,0,time.UTC)

func journalFixture(t *testing.T) (*Store, string) {
	t.Helper()
	path:=filepath.Join(t.TempDir(),"executor.sqlite")
	store,err:=openSQLite(path)
	if err!=nil {t.Fatal(err)}
	t.Cleanup(func(){_ = store.Close()})
	return store,path
}

func operationFixture(id, project string) Operation {
	return Operation{
		ID:id,Kind:"apply",ProjectID:project,RegistrationID:"hello-go-demo",
		ServiceID:"web",ProposalID:"plan-demo",
		ProposalSHA256:"sha256:"+strings.Repeat("a",64),
		CandidateDigest:"sha256:"+strings.Repeat("b",64),
		CreatedAt:fixedTime,
	}
}

func TestSQLiteV1MigrationWALConstraintsAndAppendOnly(t *testing.T) {
	s,path:=journalFixture(t)
	ctx:=context.Background()
	var version int
	if err:=s.db.QueryRow("PRAGMA user_version").Scan(&version);err!=nil||version!=SchemaVersion {
		t.Fatalf("wrong schema version: %d %v",version,err)
	}
	var mode string
	if err:=s.db.QueryRow("PRAGMA journal_mode").Scan(&mode);err!=nil||mode!="wal" {
		t.Fatalf("journal must use WAL: %q %v",mode,err)
	}
	var syncMode int
	if err:=s.db.QueryRow("PRAGMA synchronous").Scan(&syncMode);err!=nil||syncMode!=2 {
		t.Fatalf("journal must use synchronous FULL: %d %v",syncMode,err)
	}
	for _,table:=range []string{"operations","phase_events","baselines","override_records","receipt_publications"} {
		var found string
		if err:=s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?",table).Scan(&found);err!=nil||found!=table {
			t.Fatalf("missing journal table %s: %v",table,err)
		}
	}
	op:=operationFixture("operation-a","project-a")
	if err:=s.Begin(ctx,op);err!=nil {t.Fatal(err)}
	if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));err!=nil {t.Fatal(err)}
	for _,stmt:=range []string{
		"UPDATE phase_events SET phase='COMMIT' WHERE operation_id='operation-a'",
		"DELETE FROM phase_events WHERE operation_id='operation-a'",
		"INSERT INTO phase_events(operation_id,event_sequence,phase,event_kind,event_ns) VALUES('not-real',1,'PREPARE','intent',123)",
		"INSERT INTO phase_events(operation_id,event_sequence,phase,event_kind,event_ns) VALUES('operation-a',2,'APPLY','arbitrary',123)",
	} {
		if _,err:=s.db.Exec(stmt);err==nil {t.Fatalf("DB accepted append-only/foreign-key bypass: %s",stmt)}
	}
	for _, stmt:=range []string{
		"UPDATE operations SET proposal_sha256='sha256:forged' WHERE operation_id='operation-a'",
		"UPDATE operations SET project_id='forged' WHERE operation_id='operation-a'",
		"DELETE FROM operations WHERE operation_id='operation-a'",
	} {
		if _,err:=s.db.Exec(stmt);err==nil {
			t.Fatalf("database accepted mutation of executor authority: %s",stmt)
		}
	}
	// Reopening version 1 must not rerun destructive migrations.
	if err:=s.Close();err!=nil {t.Fatal(err)}
	reopened,err:=openSQLite(path)
	if err!=nil {t.Fatal(err)}
	defer reopened.Close()
	snap,err:=reopened.Inspect(ctx,op.ID)
	if err!=nil||len(snap.Events)!=1 {t.Fatalf("journal lost append-only evidence: %#v %v",snap,err)}
}

func TestOrderedIntentRequiresSeparateCompletionButCannotReportSuccess(t *testing.T) {
	ctx:=context.Background()
	s,_:=journalFixture(t)
	op:=operationFixture("operation-ordered","project-ordered")
	if err:=s.Begin(ctx,op);err!=nil {t.Fatal(err)}
	if err:=s.Intent(ctx,op.ID,"PULL",fixedTime.Add(time.Second));!errors.Is(err,ErrInvalidTransition) {
		t.Fatalf("side-effect phase skipped earlier intents: %v",err)
	}
	if err:=s.Complete(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));!errors.Is(err,ErrInvalidTransition) {
		t.Fatalf("completion without intent accepted: %v",err)
	}
	for i, phase:=range phases {
		at:=fixedTime.Add(time.Duration(2*i+1)*time.Second)
		if err:=s.Intent(ctx,op.ID,phase,at);err!=nil {t.Fatalf("phase %s intent: %v",phase,err)}
		if err:=s.Intent(ctx,op.ID,phase,at);!errors.Is(err,ErrInvalidTransition) {
			t.Fatalf("duplicated %s intent accepted: %v",phase,err)
		}
		if err:=s.Complete(ctx,op.ID,phase,at.Add(time.Second));err!=nil {t.Fatalf("phase %s completion: %v",phase,err)}
		if err:=s.Complete(ctx,op.ID,phase,at.Add(time.Second));!errors.Is(err,ErrInvalidTransition) {
			t.Fatalf("duplicated %s completion accepted: %v",phase,err)
		}
	}
	after,err:=s.Inspect(ctx,op.ID)
	if err!=nil || after.Operation.Outcome!=OutcomePending ||
		after.EffectiveOutcome!=OutcomePending || !after.BlocksNewWork ||
		len(after.Events)!=16 {
		t.Fatalf("phase events falsely certified a Docker deployment: %#v %v",after,err)
	}
	if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Hour));!errors.Is(err,ErrInvalidTransition){
		t.Fatalf("completed operation was replayed: %v",err)
	}
	if err:=s.MarkUncertain(ctx,op.ID,"COMMIT","state_ambiguous",fixedTime.Add(time.Hour));err!=nil {t.Fatal(err)}
	uncertain,err:=s.Inspect(ctx,op.ID)
	if err!=nil || uncertain.EffectiveOutcome!=OutcomeNeedsReconciliation ||
		uncertain.Operation.Outcome!=OutcomeNeedsReconciliation || !uncertain.BlocksNewWork {
		t.Fatalf("uncertain commit not sticky: %#v %v",uncertain,err)
	}
	if err:=s.MarkUncertain(ctx,op.ID,"COMMIT","interrupted",fixedTime.Add(2*time.Hour));!errors.Is(err,ErrInvalidTransition){
		t.Fatalf("duplicate uncertainty marker accepted: %v",err)
	}
}

func TestEveryInterruptedPhaseIsDurableAndBlocksReplay(t *testing.T) {
	ctx:=context.Background()
	for stopAt, phase:=range phases {
		t.Run(phase,func(t *testing.T){
			path:=filepath.Join(t.TempDir(),"executor.sqlite")
			s,err:=openSQLite(path)
			if err!=nil {t.Fatal(err)}
			op:=operationFixture(fmt.Sprintf("op-%d",stopAt),fmt.Sprintf("project-%d",stopAt))
			if err:=s.Begin(ctx,op);err!=nil {t.Fatal(err)}
			for i:=0;i<=stopAt;i++ {
				at:=fixedTime.Add(time.Duration(i*2+1)*time.Second)
				if err:=s.Intent(ctx,op.ID,phases[i],at);err!=nil {t.Fatal(err)}
				if i<stopAt {
					if err:=s.Complete(ctx,op.ID,phases[i],at.Add(time.Second));err!=nil {t.Fatal(err)}
				}
			}
			if err:=s.Close();err!=nil {t.Fatal(err)} // abrupt process exit after journaled intent
			reopened,err:=openSQLite(path)
			if err!=nil {t.Fatal(err)}
			defer reopened.Close()
			observed,err:=reopened.Inspect(ctx,op.ID)
			if err!=nil || observed.EffectiveOutcome!=OutcomeNeedsReconciliation ||
				!observed.BlocksNewWork || observed.Events[len(observed.Events)-1].Kind!=EventIntent {
				t.Fatalf("interrupted %s not visible: %#v %v",phase,observed,err)
			}
			replay:=operationFixture(fmt.Sprintf("replay-%d",stopAt),op.ProjectID)
			if err:=reopened.Begin(ctx,replay);!errors.Is(err,ErrProjectBlocked){
				t.Fatalf("interrupted %s allowed new work: %v",phase,err)
			}
			if err:=reopened.Intent(ctx,op.ID,phase,fixedTime.Add(time.Hour));!errors.Is(err,ErrInvalidTransition){
				t.Fatalf("crashed %s intent replayed: %v",phase,err)
			}
			if err:=reopened.MarkUncertain(ctx,op.ID,phase,"interrupted",fixedTime.Add(time.Hour));err!=nil {
				t.Fatalf("crash outcome could not be made durable: %v",err)
			}
			result,err:=reopened.Inspect(ctx,op.ID)
			if err!=nil || result.Operation.Outcome!=OutcomeNeedsReconciliation ||
				result.Events[len(result.Events)-1].Kind!=EventUncertain {
				t.Fatalf("reconciliation not durable: %#v %v",result,err)
			}
		})
	}
}

func TestBlockedProjectsCanNeverBypassWithNewIdentity(t *testing.T) {
	ctx:=context.Background()
	s,_:=journalFixture(t)
	if err:=s.Begin(ctx,operationFixture("op-first","project-one"));err!=nil {t.Fatal(err)}
	if err:=s.Begin(ctx,operationFixture("op-second","project-one"));!errors.Is(err,ErrProjectBlocked) {
		t.Fatalf("same project accepted competing operation: %v",err)
	}
	if err:=s.Begin(ctx,operationFixture("op-independent","project-two"));err!=nil {
		t.Fatalf("independent project improperly blocked: %v",err)
	}
	if err:=s.Begin(ctx,operationFixture("op-first","project-three"));err==nil{
		t.Fatal("operation ID replay accepted in different project")
	}
	if err:=s.MarkUncertain(ctx,"op-first","PREPARE","storage_uncertain",fixedTime.Add(time.Second));err!=nil {t.Fatal(err)}
	if err:=s.Begin(ctx,operationFixture("op-third","project-one"));!errors.Is(err,ErrProjectBlocked){
		t.Fatalf("sticky reconciliation state allowed new work: %v",err)
	}
}

func TestInvalidInputsAndSanitizedFailure(t *testing.T) {
	ctx:=context.Background()
	s,_:=journalFixture(t)
	cases:=[]Operation{}
	for _,mutation:=range []func(*Operation){
		func(op *Operation){op.ID="../etc/passwd"},
		func(op *Operation){op.Kind="delete"},
		func(op *Operation){op.ProjectID=""},
		func(op *Operation){op.ProposalID="/root/.env"},
		func(op *Operation){op.ProposalSHA256="sha256:BAD"},
		func(op *Operation){op.CandidateDigest="latest"},
		func(op *Operation){op.CreatedAt=time.Time{}},
		func(op *Operation){op.Outcome="succeeded"},
		func(op *Operation){op.CurrentPhase="APPLY"},
	} {
		op:=operationFixture("bad-example","project-bad")
		mutation(&op)
		cases=append(cases,op)
	}
	for i,op:=range cases {
		if err:=s.Begin(ctx,op);!errors.Is(err,ErrInvalidOperation) {
			t.Errorf("invalid operation %d accepted: %v",i,err)
		}
	}
	if err:=s.Begin(ctx,operationFixture("good-op","project-good"));err!=nil {t.Fatal(err)}
	for _,req:=range []struct{phase,kind,code string}{
		{"PULL","intent","none"},
		{"PREPARE","intent","secret=DB_PASSWORD"},
		{"PREPARE","uncertain","arbitrary stack trace"},
		{"PREPARE","unknown","none"},
	} {
		if err:=s.eventTx(ctx,"good-op",req.phase,req.kind,req.code,fixedTime.Add(time.Second));
			!errors.Is(err,ErrInvalidTransition) {
			t.Errorf("accepted unauthorized journal detail: %+v: %v",req,err)
		}
	}
	snap,err:=s.Inspect(ctx,"good-op")
	if err!=nil||len(snap.Events)!=0 {t.Fatalf("invalid input modified durable journal: %#v %v",snap,err)}
}

func TestConcurrentIndependentProjectsRemainIsolated(t *testing.T) {
	s,_:=journalFixture(t)
	const workers=12
	var wg sync.WaitGroup
	errs:=make(chan error,workers)
	for i:=0;i<workers;i++{
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs<-s.Begin(context.Background(),operationFixture(fmt.Sprintf("op-%d",i),fmt.Sprintf("project-%d",i)))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err:=range errs { if err!=nil {t.Errorf("independent fake-backed operation failed: %v",err)} }
}

func TestMigrationRejectsUnknownVersionAndForeignSchema(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"executor.sqlite")
	s,err:=openSQLite(path)
	if err!=nil {t.Fatal(err)}
	if _,err:=s.db.Exec("PRAGMA user_version=999");err!=nil {t.Fatal(err)}
	if err:=s.Close();err!=nil {t.Fatal(err)}
	if _,err:=openSQLite(path);!errors.Is(err,ErrUnsupportedSchema) {
		t.Fatalf("unknown newer schema accepted: %v",err)
	}
	foreign:=filepath.Join(t.TempDir(),"foreign.sqlite")
	db,err:=sql.Open("sqlite",foreign)
	if err!=nil {t.Fatal(err)}
	if _,err:=db.Exec("CREATE TABLE operations (bad_column TEXT)");err!=nil {t.Fatal(err)}
	db.Close()
	if _,err:=openSQLite(foreign);!errors.Is(err,ErrUnavailable) {
		t.Fatalf("foreign SQLite database silently adopted: %v",err)
	}
}
