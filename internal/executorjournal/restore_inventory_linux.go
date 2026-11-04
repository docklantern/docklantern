package executorjournal

import (
 "context"
 "database/sql"
 "errors"
 "io"
 "os"
 "strconv"
 "strings"
 "syscall"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

const maxRestoreInventoryEntries = 10000

// auditRestoreInventoryAt detects files outside the candidate journal's
// reference set, notably newer generation/receipt files hidden by an older
// snapshot. This is private CI-only read-only evidence, NEVER permission to
// replace active state. It must run under a separately proven host-exclusive
// maintenance fence in any future operational integration.
func auditRestoreInventoryAt(ctx context.Context, candidatePath, overrideRoot, receiptDir string) (RestoreComparison,error) {
 blocked:=RestoreComparison{Status:RestoreNeedsReconciliation,ReasonCode:"inverse_inventory_unavailable",BlocksNewWork:true}
 if ctx==nil || ctx.Err()!=nil || os.Geteuid()!=0 || candidatePath=="" {
  return blocked,ErrUnavailable
 }
 // First verify that references actually present in the candidate are
 // backed by protected, content-matching files.
 referenced,err:=auditRestoredArtifactReferencesAt(ctx,candidatePath,overrideRoot,receiptDir)
 if err!=nil{return blocked,ErrUnavailable}
 if referenced.Status!=RestoreRowsMatchUnqualified{return referenced,nil}

 db,err:=sql.Open("sqlite","file:"+candidatePath+"?mode=ro&immutable=1")
 if err!=nil{return blocked,ErrUnavailable}
 defer db.Close()
 db.SetMaxOpenConns(1)
 tx,err:=db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return blocked,ErrUnavailable}
 defer tx.Rollback()

 // An override generation is represented by a single immutable manifest
 // even when multiple stage events refer to it.
 overrides:=make(map[string]map[string]bool)
 rows,err:=tx.QueryContext(ctx,`SELECT project_id,generation
 FROM override_generation_events GROUP BY project_id,generation`)
 if err!=nil{return blocked,ErrUnavailable}
 count:=0
 for rows.Next(){
  count++
  if count>maxRestoreInventoryEntries{rows.Close();return blocked,ErrUnavailable}
  var project string
  var generation int64
  if rows.Scan(&project,&generation)!=nil || !executorregistration.ValidID(project) ||
   generation<1 || generation>999999999999999999 {
   rows.Close();return blocked,ErrUnavailable
  }
  if overrides[project]==nil{overrides[project]=make(map[string]bool)}
  overrides[project][overrideFilename(generation)]=true
 }
 err=rows.Err()
 rows.Close()
 if err!=nil{return blocked,ErrUnavailable}

 // A final receipt requires a published row. Pending-only is unresolved
 // even if a file happens to exist after an interrupted rename.
 receiptNames:=make(map[string]bool)
 rows,err=tx.QueryContext(ctx,`SELECT operation_id,receipt_version,publication_state
 FROM receipt_publications`)
 if err!=nil{return blocked,ErrUnavailable}
 count=0
 for rows.Next(){
  count++
  if count>maxRestoreInventoryEntries{rows.Close();return blocked,ErrUnavailable}
  var id,state string
  var version int
  if rows.Scan(&id,&version,&state)!=nil || !executorregistration.ValidID(id) ||
    version!=1 || (state!="pending" && state!="published") {
   rows.Close();return blocked,ErrUnavailable
  }
  if state=="published"{receiptNames[id+"-v1.json"]=true}
 }
 err=rows.Err()
 rows.Close()
 if err!=nil{return blocked,ErrUnavailable}
 if ctx.Err()!=nil{return blocked,ErrUnavailable}

 // Refuse all unknown entries, incomplete temp files, other directory
 // types, and even valid-looking but unreferenced generations. A candidate
 // journal is never permitted to erase the evidence of a later operation.
 rootfd,err:=openPrivateBackupDir(overrideRoot)
 if err!=nil{return blocked,ErrUnavailable}
 defer syscall.Close(rootfd)
 projectEntries,err:=readInventoryNames(rootfd,maxRestoreInventoryEntries)
 if err!=nil{return blocked,ErrUnavailable}
 total:=0
 for _,project:=range projectEntries {
  if !executorregistration.ValidID(project){
   blocked.ReasonCode="override_inventory_extra_or_unsafe"
   return blocked,nil
  }
  projectfd,e:=openOverrideProjectDir(overrideRoot,project)
  if e!=nil {
   blocked.ReasonCode="override_inventory_extra_or_unsafe"
   return blocked,nil
  }
  names,e:=readInventoryNames(projectfd,maxRestoreInventoryEntries)
  syscall.Close(projectfd)
  if e!=nil{return blocked,ErrUnavailable}
  total+=len(names)
  if total>maxRestoreInventoryEntries{return blocked,ErrUnavailable}
  for _,name:=range names {
   if !validOverrideInventoryName(name) || !overrides[project][name] {
    blocked.ReasonCode="override_inventory_extra_or_unsafe"
    return blocked,nil
   }
  }
  if len(names)!=len(overrides[project]){
   blocked.ReasonCode="override_inventory_incomplete"
   return blocked,nil
  }
  delete(overrides,project)
 }
 if len(overrides)!=0 {
  blocked.ReasonCode="override_inventory_incomplete"
  return blocked,nil
 }

 receiptFD,err:=openReceiptOutbox(receiptDir)
 if err!=nil{return blocked,ErrUnavailable}
 defer syscall.Close(receiptFD)
 names,err:=readInventoryNames(receiptFD,maxRestoreInventoryEntries)
 if err!=nil{return blocked,ErrUnavailable}
 for _,name:=range names {
  allowed,final:=receiptFilenameAllowed(name)
  if !allowed||!final||!receiptNames[name] {
   blocked.ReasonCode="receipt_inventory_extra_or_unsafe"
   return blocked,nil
  }
 }
 if len(names)!=len(receiptNames) {
  blocked.ReasonCode="receipt_inventory_incomplete"
  return blocked,nil
 }
 if ctx.Err()!=nil{return blocked,ErrUnavailable}
 return RestoreComparison{Status:RestoreRowsMatchUnqualified,
  ReasonCode:"bidirectional_artifacts_match_unqualified",BlocksNewWork:true},nil
}

func validOverrideInventoryName(name string) bool {
 if len(name)!=len("g000000000000000001.json") ||
  !strings.HasPrefix(name,"g")||!strings.HasSuffix(name,".json"){return false}
 digits:=strings.TrimSuffix(strings.TrimPrefix(name,"g"),".json")
 for _,ch:=range digits {if ch<'0'||ch>'9'{return false}}
 gen,err:=strconv.ParseInt(digits,10,64)
 return err==nil&&gen>=1&&gen<=999999999999999999&&overrideFilename(gen)==name
}

// Duplicate the directory FD so inventory scanning never alters another
// reader's directory position. Bound entry counts and fail closed on errors.
// Filesystem objects are subsequently verified by trusted nofollow openers.
func readInventoryNames(dirfd,max int)([]string,error){
 dup,err:=syscall.Openat(dirfd,".",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
 if err!=nil{return nil,ErrUnavailable}
 f:=os.NewFile(uintptr(dup),"protected-inventory")
 defer f.Close()
 entries,err:=f.ReadDir(max+1)
 if err!=nil&&!errors.Is(err,io.EOF){return nil,ErrUnavailable}
 if len(entries)>max{return nil,ErrUnavailable}
 names:=make([]string,0,len(entries))
 for _,entry:=range entries{names=append(names,entry.Name())}
 return names,nil
}
