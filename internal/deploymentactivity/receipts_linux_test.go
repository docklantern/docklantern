package deploymentactivity

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
)

func writeFixture(t *testing.T,dir,id,outcome string,at time.Time){
 t.Helper()
 record:=executorjournal.Receipt{
  SchemaVersion:executorjournal.ReceiptSchemaVersion,
  OperationID:id,ProposalID:"proposal-demo",RegistrationID:"registration-demo",
  Service:"web",Phase:"VERIFY",Outcome:outcome,
  CandidateDigest:"sha256:"+strings.Repeat("a",64),
  UpdatedAt:at.UTC().Format(time.RFC3339Nano),
  MessageCode:"operation_pending",
 }
 if outcome=="needs_reconciliation"{record.MessageCode="verification_incomplete"}
 b,e:=json.Marshal(record)
 if e!=nil{t.Fatal(e)}
 if e=os.WriteFile(filepath.Join(dir,id+"-v1.json"),b,0o644);e!=nil{t.Fatal(e)}
}
func TestMissingMirrorIsUnknownNotSuccessful(t *testing.T){
 now:=time.Now().UTC()
 out,err:=readAt(filepath.Join(t.TempDir(),"not-mounted"),now,uint32(os.Geteuid()))
 if err!=nil||out.Available||len(out.Rows)!=0{
  t.Fatalf("missing outbox misrepresented: %#v %v",out,err)
 }
}
func TestBoundedSanitizedActivity(t *testing.T){
 dir:=t.TempDir()
 now:=time.Now().UTC()
 writeFixture(t,dir,"operation-a","pending",now.Add(-time.Minute))
 writeFixture(t,dir,"operation-b","needs_reconciliation",now.Add(-48*time.Hour))
 out,err:=readAt(dir,now,uint32(os.Geteuid()))
 if err!=nil||!out.Available||len(out.Rows)!=2{t.Fatalf("activity read: %#v %v",out,err)}
 if out.Rows[0].OperationID!="operation-a"||out.Rows[0].Stale||
  out.Rows[1].OperationID!="operation-b"||!out.Rows[1].Stale{
  t.Fatalf("stale/sort classification: %#v",out)
 }
 for _,row:=range out.Rows{
  if row.Outcome=="succeeded"||row.Outcome=="verified_healthy"{
   t.Fatal("receipt falsely reported a Docker update success")
  }
 }
}
func TestReceiptMirrorRejectsForgedOrUnsafeEntries(t *testing.T){
 now:=time.Now().UTC()
 cases:=[]struct{name string;change func(*testing.T,string)}{
  {"unexpected_filename",func(t *testing.T,dir string){os.WriteFile(filepath.Join(dir,"random.txt"),[]byte("raw"),0o644)}},
  {"symlink",func(t *testing.T,dir string){os.Symlink("/etc/passwd",filepath.Join(dir,"operation-a-v1.json"))}},
  {"writable_directory",func(t *testing.T,dir string){os.Chmod(dir,0o777)}},
  {"forged_success",func(t *testing.T,dir string){
   writeFixture(t,dir,"operation-a","pending",now)
   p:=filepath.Join(dir,"operation-a-v1.json")
   b,_:=os.ReadFile(p)
   b=[]byte(strings.Replace(string(b),"\"pending\"","\"succeeded\"",1))
   os.WriteFile(p,b,0o644)
  }},
  {"future_receipt",func(t *testing.T,dir string){writeFixture(t,dir,"operation-a","pending",now.Add(time.Hour))}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  dir:=t.TempDir()
  tc.change(t,dir)
  if v,e:=readAt(dir,now,uint32(os.Geteuid()));e==nil||v.Available{
   t.Fatalf("unsafe receipt mirror accepted: %#v %v",v,e)
  }
 })}
}
