package executorjournal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type simulatedActualReader struct {
	state ActualEvidence
	fail error
	calls int
	afterRead func()
}
func (f *simulatedActualReader) InspectActual(ctx context.Context, projectID, serviceID string) (ActualEvidence,error) {
	f.calls++
	if f.afterRead != nil { f.afterRead() }
	if err:=ctx.Err(); err!=nil { return ActualEvidence{},err }
	if f.fail!=nil {return ActualEvidence{},f.fail}
	return f.state,nil
}

// These rows are injected ONLY by test code; no Phase 3b API can record a
// baseline, promote an override, finalize an operation or invoke Docker.
func reconciliationFixture(t *testing.T, baseline, override bool) (*Store,*simulatedActualReader) {
	t.Helper()
	s,_:=journalFixture(t)
	ctx:=context.Background()
	op:=operationFixture("reconcile-op","reconcile-project")
	if err:=s.Begin(ctx,op);err!=nil {t.Fatal(err)}
	if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));err!=nil {t.Fatal(err)}
	if err:=s.Complete(ctx,op.ID,"PREPARE",fixedTime.Add(2*time.Second));err!=nil {t.Fatal(err)}
	actual:=ActualEvidence{
		ProjectID:op.ProjectID,ServiceID:op.ServiceID,
		ContainerID:strings.Repeat("a",64),
		ImageID:"sha256:"+strings.Repeat("b",64),
		Platform:"linux/amd64",
		SourceSHA256:"sha256:"+strings.Repeat("c",64),
		DesiredImageSHA256:"sha256:"+strings.Repeat("d",64),
		HealthGrade:"healthy",
		OverrideGeneration:0,
		CommittedOverrideSHA256:"sha256:"+strings.Repeat("e",64),
		ObservedAt:fixedTime.Add(4*time.Second),
	}
	if baseline {
		_,err:=s.db.Exec(`INSERT INTO baselines
			(operation_id,container_id,image_id,platform,source_fingerprint,
			desired_image_fingerprint,health_grade,recorded_ns)
			VALUES (?,?,?,?,?,?,?,?)`,
			op.ID,actual.ContainerID,actual.ImageID,actual.Platform,
			actual.SourceSHA256,actual.DesiredImageSHA256,actual.HealthGrade,
			fixedTime.Add(3*time.Second).UnixNano())
		if err!=nil {t.Fatal(err)}
	}
	if override {
		_,err:=s.db.Exec(`INSERT INTO override_records
			(project_id,generation,committed_sha256) VALUES(?,?,?)`,
			op.ProjectID,int64(0),actual.CommittedOverrideSHA256)
		if err!=nil {t.Fatal(err)}
	}
	return s,&simulatedActualReader{state:actual}
}

func TestInspectOnlyReconciliationMatchesButNeverGrantsSuccess(t *testing.T) {
	s,f:=reconciliationFixture(t,true,true)
	ctx:=context.Background()
	var eventsBefore, baselinesBefore, overridesBefore int
	for i, table:=range []string{"phase_events","baselines","override_records"} {
		var count int
		if err:=s.db.QueryRow("SELECT count(*) FROM "+table).Scan(&count);err!=nil {t.Fatal(err)}
		switch i {case 0:eventsBefore=count;case 1:baselinesBefore=count;case 2:overridesBefore=count}
	}
	report,err:=s.ReconcileReadOnly(ctx,"reconcile-op",f,fixedTime.Add(5*time.Second))
	if err!=nil||report.Status!=CheckObservationsMatch||report.ReasonCode!="unqualified_match" ||
		!report.BlocksNewWork||report.ProjectID!="reconcile-project"||f.calls!=1{
		t.Fatalf("safe read-only comparison failed: %#v %v calls=%d",report,err,f.calls)
	}
	snapshot,err:=s.Inspect(ctx,"reconcile-op")
	if err!=nil||snapshot.Operation.Outcome!=OutcomePending||!snapshot.BlocksNewWork {
		t.Fatalf("reconciliation mutated journal authority: %#v %v",snapshot,err)
	}
	for i, table:=range []string{"phase_events","baselines","override_records"} {
		var count int
		if err:=s.db.QueryRow("SELECT count(*) FROM "+table).Scan(&count);err!=nil {t.Fatal(err)}
		want:=[]int{eventsBefore,baselinesBefore,overridesBefore}[i]
		if count!=want {t.Errorf("reconciliation mutated %s: %d vs %d",table,count,want)}
	}
	if err:=s.Begin(ctx,operationFixture("replay-op","reconcile-project"));!errors.Is(err,ErrProjectBlocked) {
		t.Fatalf("matching inspection unblocked pending operation: %v",err)
	}
}

func TestReconciliationFailsClosedOnMissingAndDivergentEvidence(t *testing.T) {
	now:=fixedTime.Add(5*time.Second)
	cases:=[]struct{
		name,reason string
		baseline,override bool
		edit func(*simulatedActualReader)
	}{
		{"missing-baseline","baseline_missing",false,true,nil},
		{"missing-override","override_missing",true,false,nil},
		{"wrong-project","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.ProjectID="elsewhere"}},
		{"wrong-service","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.ServiceID="other"}},
		{"different-container","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.ContainerID=strings.Repeat("0",64)}},
		{"short-container-id","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.ContainerID="abc"}},
		{"different-image","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.ImageID="sha256:"+strings.Repeat("0",64)}},
		{"different-platform","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.Platform="linux/arm64"}},
		{"changed-source","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.SourceSHA256="sha256:"+strings.Repeat("0",64)}},
		{"changed-desired-image","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.DesiredImageSHA256="sha256:"+strings.Repeat("0",64)}},
		{"wrong-health-grade","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.HealthGrade="unhealthy"}},
		{"missing-health-grade","baseline_mismatch",true,true,func(f *simulatedActualReader){f.state.HealthGrade=""}},
		{"restored-old-db-new-override","override_mismatch",true,true,func(f *simulatedActualReader){f.state.OverrideGeneration=1}},
		{"restored-old-override","override_mismatch",true,true,func(f *simulatedActualReader){f.state.CommittedOverrideSHA256="sha256:"+strings.Repeat("f",64)}},
		{"malformed-override-hash","override_mismatch",true,true,func(f *simulatedActualReader){f.state.CommittedOverrideSHA256="secret"}},
		{"stale-backend","backend_stale",true,true,func(f *simulatedActualReader){f.state.ObservedAt=now.Add(-11*time.Second)}},
		{"future-backend","backend_stale",true,true,func(f *simulatedActualReader){f.state.ObservedAt=now.Add(time.Second)}},
		{"missing-timestamp","backend_stale",true,true,func(f *simulatedActualReader){f.state.ObservedAt=time.Time{}}},
		{"backend-error-sanitized","backend_unavailable",true,true,func(f *simulatedActualReader){f.fail=errors.New("raw secret token from Docker")}},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			s,f:=reconciliationFixture(t,tc.baseline,tc.override)
			if tc.edit!=nil {tc.edit(f)}
			report,err:=s.ReconcileReadOnly(context.Background(),"reconcile-op",f,now)
			if err!=nil||report.Status!=CheckNeedsReconciliation||
				report.ReasonCode!=tc.reason||!report.BlocksNewWork {
				t.Fatalf("expected fail-closed %s, got %#v %v",tc.reason,report,err)
			}
			if strings.Contains(report.ReasonCode,"secret")||
				strings.Contains(report.ReasonCode,"/var/") {
				t.Fatal("untrusted evidence leaked")
			}
			if !tc.baseline || !tc.override {
				if f.calls!=0 {t.Fatal("called backend before required journal evidence existed")}
			}
		})
	}
}

func TestReconciliationRejectsIncompletePromotionAcrossGenerations(t *testing.T) {
	s,f:=reconciliationFixture(t,true,true)
	ctx:=context.Background()
	// A newer uncommitted override generation must NOT be adopted or ignored.
	_,err:=s.db.Exec(`INSERT INTO override_records
		(project_id,generation,committed_sha256,previous_sha256,
		staged_operation_id,promotion_intent_ns)
		VALUES (?,?,?,?,?,?)`,
		"reconcile-project",1,"sha256:"+strings.Repeat("f",64),
		f.state.CommittedOverrideSHA256,"reconcile-op",fixedTime.Add(3*time.Second).UnixNano())
	if err!=nil {t.Fatal(err)}
	f.state.OverrideGeneration=1
	f.state.CommittedOverrideSHA256="sha256:"+strings.Repeat("f",64)
	report,err:=s.ReconcileReadOnly(ctx,"reconcile-op",f,fixedTime.Add(5*time.Second))
	if err!=nil||report.Status!=CheckNeedsReconciliation||
		report.ReasonCode!="override_promotion_ambiguous"||f.calls!=0 {
		t.Fatalf("ambiguous promotion falsely matched: %#v %v",report,err)
	}
	// A later committed record does not erase a prior ambiguous generation.
	_,err=s.db.Exec(`INSERT INTO override_records
		(project_id,generation,committed_sha256,promotion_intent_ns,promotion_complete_ns)
		VALUES(?,?,?,?,?)`,
		"reconcile-project",2,"sha256:"+strings.Repeat("d",64),
		fixedTime.Add(3*time.Second).UnixNano(),fixedTime.Add(4*time.Second).UnixNano())
	if err!=nil {t.Fatal(err)}
	report,err=s.ReconcileReadOnly(ctx,"reconcile-op",f,fixedTime.Add(5*time.Second))
	if err!=nil||report.ReasonCode!="override_promotion_ambiguous" {
		t.Fatalf("historical ambiguous promotion was ignored: %#v %v",report,err)
	}
}

func TestReconciliationNeverReplaysInterruptedIntent(t *testing.T) {
	s,f:=reconciliationFixture(t,true,true)
	// Journal the next phase, but simulate termination before completion.
	if err:=s.Intent(context.Background(),"reconcile-op","AUTHORIZE",fixedTime.Add(4*time.Second));err!=nil {t.Fatal(err)}
	report,err:=s.ReconcileReadOnly(context.Background(),"reconcile-op",f,fixedTime.Add(5*time.Second))
	if err!=nil||report.Status!=CheckNeedsReconciliation||
		report.ReasonCode!="journal_unresolved"||f.calls!=0{
		t.Fatalf("uncompleted intent bypassed reconciliation: %#v %v",report,err)
	}
	if err:=s.Complete(context.Background(),"reconcile-op","AUTHORIZE",fixedTime.Add(6*time.Second));err!=nil {t.Fatal(err)}
	if err:=s.MarkUncertain(context.Background(),"reconcile-op","AUTHORIZE","external_change",fixedTime.Add(7*time.Second));err!=nil {t.Fatal(err)}
	report,err=s.ReconcileReadOnly(context.Background(),"reconcile-op",f,fixedTime.Add(8*time.Second))
	if err!=nil||report.ReasonCode!="journal_unresolved" {
		t.Fatalf("explicit uncertain outcome was hidden: %#v %v",report,err)
	}
}

func TestReconciliationRechecksJournalAfterSlowBackend(t *testing.T) {
	s,f:=reconciliationFixture(t,true,true)
	f.afterRead=func(){
		if err:=s.MarkUncertain(context.Background(),"reconcile-op","PREPARE","external_change",
			fixedTime.Add(4*time.Second));err!=nil {t.Fatalf("simulated concurrent journal update: %v",err)}
	}
	report,err:=s.ReconcileReadOnly(context.Background(),"reconcile-op",f,fixedTime.Add(5*time.Second))
	if err!=nil||report.Status!=CheckNeedsReconciliation||report.ReasonCode!="journal_changed" {
		t.Fatalf("changed ledger silently matched: %#v %v",report,err)
	}
}

func TestReconciliationRejectsCanceledUnknownAndUntrustedInputs(t *testing.T) {
	s,f:=reconciliationFixture(t,true,true)
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	for _,tc:=range []struct{
		name,id string
		ctx context.Context
		source ActualReader
		at time.Time
	}{
		{"canceled","reconcile-op",ctx,f,fixedTime.Add(5*time.Second)},
		{"unknown-operation","missing-op",context.Background(),f,fixedTime.Add(5*time.Second)},
		{"traversal","../escape",context.Background(),f,fixedTime.Add(5*time.Second)},
		{"nil-source","reconcile-op",context.Background(),nil,fixedTime.Add(5*time.Second)},
		{"zero-clock","reconcile-op",context.Background(),f,time.Time{}},
	}{
		t.Run(tc.name,func(t *testing.T){
			if _,err:=s.ReconcileReadOnly(tc.ctx,tc.id,tc.source,tc.at);!errors.Is(err,ErrUnavailable){
				t.Fatalf("unsafe inspection accepted: %v",err)
			}
		})
	}
}
