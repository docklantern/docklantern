package executorjournal

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReceiptAllowlistAndCanonicalParser(t *testing.T) {
	s,_:=journalFixture(t)
	op:=operationFixture("receipt-demo","receipt-project")
	if err:=s.Begin(context.Background(),op);err!=nil{t.Fatal(err)}
	snap,err:=s.Inspect(context.Background(),op.ID)
	if err!=nil{t.Fatal(err)}
	receipt,err:=receiptFromSnapshot(snap)
	if err!=nil{t.Fatal(err)}
	data,err:=receipt.canonical()
	if err!=nil || len(data)>MaxReceiptBytes {t.Fatalf("canonical receipt rejected: %v",err)}
	parsed,err:=ParseReceipt(data)
	if err!=nil || parsed.Outcome!="pending" || parsed.Phase!="NONE" ||
		parsed.MessageCode!="operation_pending" {
		t.Fatalf("pending journal falsely reported a terminal outcome: %#v %v",parsed,err)
	}
	for _,forbidden:=range []string{
		"project_directory","docker_host","source_path","env_file",
		"stderr","stack_trace","authorization","success",
	} {
		if bytes.Contains(data,[]byte(forbidden)) {t.Errorf("forbidden field leaked: %s",forbidden)}
	}
	// The original operation's metadata is bounded/validated independently;
	// there is no API permitting arbitrary human text in this receipt.
	for _,invalid:=range [][]byte{
		nil,[]byte("{}"),
		append([]byte(" "),data...),
		append(append([]byte(nil),data...),[]byte("\n")...),
		append(append([]byte(nil),data...),[]byte("{}")...),
		[]byte(strings.Replace(string(data),`"schema_version":1`,`"schema_version":999`,1)),
		[]byte(strings.Replace(string(data),`"outcome":"pending"`,`"outcome":"succeeded"`,1)),
		[]byte(strings.Replace(string(data),`"message_code":"operation_pending"`,`"message_code":"secret=TOPSECRET"`,1)),
		[]byte(strings.Replace(string(data),`"phase":"NONE"`,`"phase":"SHELL"`,1)),
		[]byte(strings.Replace(string(data),`"operation_id":"receipt-demo"`,
			`"operation_id":"../escape"`,1)),
		[]byte(strings.Replace(string(data),`"operation_id":"receipt-demo"`,
			`"operation_id":"receipt-demo","operation_id":"receipt-demo"`,1)),
		[]byte(strings.Replace(string(data),`"operation_id":"receipt-demo"`,
			`"operation_id":"receipt-demo","raw_stdout":"password"`,1)),
		bytes.Repeat([]byte("x"),MaxReceiptBytes+1),
	} {
		if _,err:=ParseReceipt(invalid);!errors.Is(err,ErrReceiptUnavailable) {
			t.Errorf("unsafe JSON accepted: %.120q (%v)",invalid,err)
		}
	}
}

func TestReceiptStatusNeverUpgradesJournalOperation(t *testing.T) {
	ctx:=context.Background()
	for _,kind:=range []string{"pending","intent","uncertain"}{
		t.Run(kind,func(t *testing.T){
			s,_:=journalFixture(t)
			op:=operationFixture("status-op","status-project")
			if err:=s.Begin(ctx,op);err!=nil{t.Fatal(err)}
			if kind!="pending"{
				if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));err!=nil{t.Fatal(err)}
			}
			if kind=="uncertain"{
				if err:=s.MarkUncertain(ctx,op.ID,"PREPARE","state_ambiguous",
					fixedTime.Add(2*time.Second));err!=nil{t.Fatal(err)}
			}
			snap,err:=s.Inspect(ctx,op.ID)
			if err!=nil{t.Fatal(err)}
			r,err:=receiptFromSnapshot(snap)
			if err!=nil{t.Fatal(err)}
			want:="pending"
			if kind!="pending"{want=OutcomeNeedsReconciliation}
			if r.Outcome!=want{t.Errorf("receipt misreported %s as %s",kind,r.Outcome)}
			if kind=="intent" && r.MessageCode!="interrupted"{
				t.Fatal("unmatched durable intent was not disclosed")
			}
			if kind=="uncertain" && r.MessageCode!="verification_incomplete"{
				t.Fatal("explicit journal uncertainty was not disclosed")
			}
			if state,err:=s.ReceiptState(ctx,op.ID);err!=nil||state!="unknown"{
				t.Fatalf("absent receipt implied success: %s %v",state,err)
			}
			var published int
			if err:=s.db.QueryRow("SELECT count(*) FROM receipt_publications").Scan(&published);err!=nil||published!=0 {
				t.Fatalf("read-only derivation wrote journal: %d %v",published,err)
			}
		})
	}
}

func TestReceiptReservationAndReplayRejectDifferentJournalBytes(t *testing.T) {
	s,_:=journalFixture(t)
	ctx:=context.Background()
	op:=operationFixture("receipt-once","receipt-project")
	if err:=s.Begin(ctx,op);err!=nil{t.Fatal(err)}
	receipt,err:=s.receiptForOperation(ctx,op.ID)
	if err!=nil{t.Fatal(err)}
	data,err:=receipt.canonical()
	if err!=nil{t.Fatal(err)}
	hash:=receiptHash(data)
	at:=fixedTime.Add(3*time.Second)
	first,err:=s.reserveReceipt(ctx,op.ID,hash,at)
	if err!=nil||first{t.Fatalf("first reservation rejected: %v existing=%t",err,first)}
	first,err=s.reserveReceipt(ctx,op.ID,hash,at)
	if err!=nil||first{t.Fatalf("same-hash pending reservation not idempotent: %v",err)}
	state,err:=s.ReceiptState(ctx,op.ID)
	if err!=nil||state!="pending"{t.Fatalf("expected pending: %s %v",state,err)}
	if err:=s.markReceiptPublished(ctx,op.ID,hash,at);err!=nil{t.Fatal(err)}
	if err:=s.markReceiptPublished(ctx,op.ID,hash,at);err!=nil{t.Fatal(err)}
	state,err=s.ReceiptState(ctx,op.ID)
	if err!=nil||state!="published"{t.Fatalf("publication status missing: %s %v",state,err)}
	var count int
	if err:=s.db.QueryRow("SELECT count(*) FROM receipt_publications").Scan(&count);err!=nil||count!=2 {
		t.Fatalf("duplicate events inserted: %d %v",count,err)
	}
	if _,err:=s.reserveReceipt(ctx,op.ID,"sha256:"+strings.Repeat("0",64),at);
		!errors.Is(err,ErrReceiptUnavailable) {t.Fatal("receipt content replaced")}
	if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(4*time.Second));err!=nil{t.Fatal(err)}
	// Once the journal has advanced, a frozen v1 receipt is not rewritten
	// to conceal the earlier published bytes or its status.
	if _,err:=s.reserveReceipt(ctx,op.ID,hash,at);!errors.Is(err,ErrReceiptUnavailable){
		t.Fatal("updated operation reused obsolete receipt fingerprint")
	}
	if err:=s.markReceiptPublished(ctx,op.ID,hash,at);!errors.Is(err,ErrReceiptUnavailable){
		t.Fatal("changed operation falsely re-published old status")
	}
	snap,err:=s.Inspect(ctx,op.ID)
	if err!=nil||snap.Operation.Outcome!=OutcomePending||!snap.BlocksNewWork{
		t.Fatalf("receipt update changed operation state: %#v %v",snap,err)
	}
}

func TestReceiptNeverPublishesSuccessWithoutIndependentActualVerification(t *testing.T) {
	s,_:=journalFixture(t)
	ctx:=context.Background()
	op:=operationFixture("success-claim","private-project")
	if err:=s.Begin(ctx,op);err!=nil{t.Fatal(err)}
	if _,err:=s.db.Exec("UPDATE operations SET outcome='succeeded' WHERE operation_id=?",op.ID);err!=nil{
		t.Fatal(err)
	}
	if _,err:=s.receiptForOperation(ctx,op.ID);!errors.Is(err,ErrReceiptUnavailable){
		t.Fatalf("unsupported successful Docker claim escaped public receipt: %v",err)
	}
}
