package executorjournal

import (
 "context"
 "os"
 "path/filepath"
 "testing"
)

func TestRestoreInverseInventoryMatchesButNeverAuthorizesRootFixture(t *testing.T){
 _,candidate,overrides,receipts,_,_:=artifactAuditFixture(t)
 result,err:=auditRestoreInventoryAt(context.Background(),candidate,overrides,receipts)
 if err!=nil||result.Status!=RestoreRowsMatchUnqualified||
  result.ReasonCode!="bidirectional_artifacts_match_unqualified"||!result.BlocksNewWork {
  t.Fatalf("match bypassed restore block: %#v %v",result,err)
 }
}

func TestRestoreInverseInventoryRejectsLaterArtifactsRootFixture(t *testing.T){
 cases:=[]struct{name,expected string;change func(*testing.T,string,string,string,string)}{
  {"newer_override","override_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    p:=filepath.Join(filepath.Dir(manifest),overrideFilename(2))
    if e:=os.WriteFile(p,[]byte("later generation"),0o600);e!=nil{t.Fatal(e)}
  }},
  {"unrecorded_project","override_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    p:=filepath.Join(overrides,"later-project")
    if e:=os.Mkdir(p,0o700);e!=nil{t.Fatal(e)}
    if e:=os.WriteFile(filepath.Join(p,overrideFilename(1)),[]byte("later"),0o600);e!=nil{t.Fatal(e)}
  }},
  {"unfinished_override","override_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    if e:=os.WriteFile(filepath.Join(filepath.Dir(manifest),".staged-unknown.tmp"),[]byte("pending"),0o600);e!=nil{t.Fatal(e)}
  }},
  {"extra_receipt","receipt_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    if e:=os.WriteFile(filepath.Join(receipts,"later-op-v1.json"),[]byte("later"),0o644);e!=nil{t.Fatal(e)}
  }},
  {"unfinished_receipt","receipt_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    if e:=os.WriteFile(filepath.Join(receipts,".receipt-00000000000000000000000000000001.tmp"),[]byte("pending"),0o600);e!=nil{t.Fatal(e)}
  }},
  {"symlinked_project","override_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    if e:=os.Symlink("/etc",filepath.Join(overrides,"later-project"));e!=nil{t.Fatal(e)}
  }},
  {"unexpected_receipt_directory","receipt_inventory_extra_or_unsafe",func(t *testing.T,overrides,receipts,manifest,receipt string){
    if e:=os.Mkdir(filepath.Join(receipts,"unexpected"),0o700);e!=nil{t.Fatal(e)}
  }},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  _,candidate,overrides,receipts,manifest,receipt:=artifactAuditFixture(t)
  tc.change(t,overrides,receipts,manifest,receipt)
  result,err:=auditRestoreInventoryAt(context.Background(),candidate,overrides,receipts)
  if err!=nil||result.Status!=RestoreNeedsReconciliation||
   result.ReasonCode!=tc.expected||!result.BlocksNewWork {
   t.Fatalf("unreferenced artifact accepted: %#v %v",result,err)
  }
 })}
}

func TestRestoreInverseInventoryDelegatesMissingReferenceRootFixture(t *testing.T){
 _,candidate,overrides,receipts,manifest,_:=artifactAuditFixture(t)
 if e:=os.Remove(manifest);e!=nil{t.Fatal(e)}
 result,err:=auditRestoreInventoryAt(context.Background(),candidate,overrides,receipts)
 if err!=nil||result.Status!=RestoreNeedsReconciliation||!result.BlocksNewWork {
  t.Fatalf("missing referenced override accepted: %#v %v",result,err)
 }
}
