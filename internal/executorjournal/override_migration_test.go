package executorjournal

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestV1ToV2MigrationPreservesJournalAndEnforcesImmutableGenerationEvents(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"prior-journal.sqlite")
	db,err:=sql.Open("sqlite",path)
	if err!=nil {t.Fatal(err)}
	_,err=db.Exec(schemaV1)
	if err!=nil {t.Fatal(err)}
	for _,sqlText:=range []string{
		"PRAGMA application_id = 1145851988",
		"PRAGMA user_version = 1",
		"PRAGMA foreign_keys = ON",
	} {
		if _,err:=db.Exec(sqlText);err!=nil {t.Fatal(err)}
	}
	op:=operationFixture("old-operation","old-project")
	_,err=db.Exec(`INSERT INTO operations
	(operation_id,operation_kind,project_id,registration_id,service_id,proposal_id,
	proposal_sha256,candidate_digest,created_ns,updated_ns,current_phase,outcome)
	VALUES (?,?,?,?,?,?,?,?,?,?,'NONE','pending')`,
		op.ID,op.Kind,op.ProjectID,op.RegistrationID,op.ServiceID,op.ProposalID,
		op.ProposalSHA256,op.CandidateDigest,op.CreatedAt.UnixNano(),op.CreatedAt.UnixNano())
	if err!=nil {t.Fatal(err)}
	if err:=db.Close();err!=nil {t.Fatal(err)}
	s,err:=openSQLite(path)
	if err!=nil {t.Fatalf("existing v1 journal rejected: %v",err)}
	defer s.Close()
	var version int
	if err:=s.db.QueryRow("PRAGMA user_version").Scan(&version);err!=nil||version!=2 {
		t.Fatalf("expected transactional v2 migration: %d %v",version,err)
	}
	snapshot,err:=s.Inspect(context.Background(),op.ID)
	if err!=nil||snapshot.Operation.ID!=op.ID||!snapshot.BlocksNewWork {
		t.Fatalf("migration lost pending operation: %#v %v",snapshot,err)
	}
	for _,name:=range []string{"override_generation_events",
		"override_generation_events_no_update","override_generation_events_no_delete"} {
		var typ string
		if err:=s.db.QueryRow("SELECT type FROM sqlite_master WHERE name=?",name).Scan(&typ);err!=nil {
			t.Fatalf("missing migrated schema object %s: %v",name,err)
		}
	}
	if _,err:=s.db.Exec(`INSERT INTO override_generation_events
	(project_id,generation,operation_id,event_sequence,event_kind,manifest_sha256,
	candidate_digest,previous_generation,previous_committed_sha256,recorded_ns)
	VALUES(?,1,?,1,'stage_intent',?,?,?,?,?)`,
		op.ProjectID,op.ID,"sha256:"+strings.Repeat("c",64),op.CandidateDigest,0,
		"sha256:"+strings.Repeat("a",64),fixedTime.Add(time.Second).UnixNano());err!=nil {t.Fatal(err)}
	for _,attempt:=range []string{
		"UPDATE override_generation_events SET event_kind='staged'",
		"DELETE FROM override_generation_events",
	} {
		if _,err:=s.db.Exec(attempt);err==nil {t.Fatalf("v2 audit history could be rewritten: %s",attempt)}
	}
	if _,err:=s.db.Exec(`INSERT INTO override_generation_events
	(project_id,generation,operation_id,event_sequence,event_kind,manifest_sha256,
	candidate_digest,previous_generation,previous_committed_sha256,recorded_ns)
	VALUES(?,1,?,1,'promotion_completed',?,?,?,?,?)`,
		op.ProjectID,op.ID,"sha256:"+strings.Repeat("c",64),op.CandidateDigest,0,
		"sha256:"+strings.Repeat("a",64),fixedTime.Add(2*time.Second).UnixNano());err==nil {
		t.Fatal("schema falsely permits promotion completion")
	}
}

func TestV1MigrationRejectsDamagedSchemaWithoutAdoptingIt(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"damaged.sqlite")
	db,err:=sql.Open("sqlite",path)
	if err!=nil {t.Fatal(err)}
	if _,err:=db.Exec(schemaV1);err!=nil {t.Fatal(err)}
	for _,stmt:=range []string{
		"PRAGMA application_id = 1145851988",
		"PRAGMA user_version = 1",
		"DROP TRIGGER phase_events_no_delete",
	} {
		if _,err:=db.Exec(stmt);err!=nil {t.Fatal(err)}
	}
	db.Close()
	if _,err:=openSQLite(path);!errors.Is(err,ErrUnsupportedSchema) {
		t.Fatalf("damaged v1 journal silently upgraded: %v",err)
	}
	db,err=sql.Open("sqlite",path)
	if err!=nil {t.Fatal(err)}
	defer db.Close()
	var v int
	if err:=db.QueryRow("PRAGMA user_version").Scan(&v);err!=nil||v!=1 {
		t.Fatalf("failed upgrade modified original version: %d %v",v,err)
	}
}

func TestOverrideManifestRejectsUntrustedAndUnboundedIdentifiers(t *testing.T) {
	valid:=OverrideManifest{
		SchemaVersion:OverrideManifestVersion,ProjectID:"project-demo",
		OperationID:"metadata-op",Generation:1,PreviousGeneration:0,
		PreviousCommittedSHA256:"sha256:"+strings.Repeat("a",64),
		CandidateImageDigest:"sha256:"+strings.Repeat("b",64),
	}
	data,err:=valid.bytes()
	if err!=nil||len(data)==0||len(data)>1024 {t.Fatalf("valid typed metadata rejected: %v",err)}
	for _,edit:=range []func(*OverrideManifest){
		func(m *OverrideManifest){m.SchemaVersion=2},
		func(m *OverrideManifest){m.ProjectID="../tmp"},
		func(m *OverrideManifest){m.OperationID="/root/.env"},
		func(m *OverrideManifest){m.PreviousGeneration=1},
		func(m *OverrideManifest){m.Generation=0},
		func(m *OverrideManifest){m.Generation=1000000000000000000},
		func(m *OverrideManifest){m.PreviousCommittedSHA256="latest"},
		func(m *OverrideManifest){m.CandidateImageDigest="sha256:UPPER"},
	} {
		copy:=valid
		edit(&copy)
		if _,err:=copy.bytes();!errors.Is(err,ErrOverrideStage) {
			t.Errorf("invalid override manifest accepted: %#v %v",copy,err)
		}
	}
}
