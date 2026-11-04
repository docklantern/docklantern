// Package deploymentactivity reads sanitized host-executor receipts from a
// separately provided read-only mount. It has no connection to Docker,
// executor commands, proposals, journal mutation or web POST actions.
package deploymentactivity

import (
 "errors"
 "io"
 "os"
 "sort"
 "strings"
 "syscall"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

const HostReceiptMirror = "/run/docklantern/executor-receipts"
const maxFiles = 1000
const maxDisplay = 30
const StaleAfter = 24 * time.Hour

var ErrUntrustedReceiptMirror=errors.New("executor activity receipts cannot be verified")

type Row struct{
 OperationID string
 Service string
 Phase string
 Outcome string
 CandidateDigest string
 UpdatedAt string
 Stale bool
}
type Snapshot struct{
 Available bool
 Rows []Row
}

// Read never creates the mirror or follows a relative/configured path.
// An absent mount means unknown activity, not evidence of no deployment.
func Read(now time.Time)(Snapshot,error){
 return readAt(HostReceiptMirror,now,0)
}

func readAt(path string,now time.Time,uid uint32)(Snapshot,error){
 var empty Snapshot
 if now.IsZero(){return empty,ErrUntrustedReceiptMirror}
 fd,err:=syscall.Open(path,syscall.O_RDONLY|syscall.O_DIRECTORY|
  syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if err==syscall.ENOENT{return empty,nil}
 if err!=nil{return empty,ErrUntrustedReceiptMirror}
 directory:=os.NewFile(uintptr(fd),"executor-receipt-mirror")
 defer directory.Close()
 var stat syscall.Stat_t
 if syscall.Fstat(fd,&stat)!=nil||stat.Uid!=uid||
  stat.Mode&syscall.S_IFMT!=syscall.S_IFDIR||
  stat.Mode&0o022!=0 {return empty,ErrUntrustedReceiptMirror}
 entries,err:=directory.ReadDir(maxFiles+1)
 if err!=nil&&!errors.Is(err,io.EOF){return empty,ErrUntrustedReceiptMirror}
 if len(entries)>maxFiles{return empty,ErrUntrustedReceiptMirror}
 result:=Snapshot{Available:true,Rows:make([]Row,0,len(entries))}
 for _,entry:=range entries{
  name:=entry.Name()
  if strings.HasPrefix(name,".receipt-")&&strings.HasSuffix(name,".tmp"){
   // A pending atomic publication is not an activity record.
   continue
  }
  if !strings.HasSuffix(name,"-v1.json")||
   !executorregistration.ValidID(strings.TrimSuffix(name,"-v1.json")){
   return empty,ErrUntrustedReceiptMirror
  }
  filefd,err:=syscall.Openat(fd,name,syscall.O_RDONLY|
   syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC,0)
  if err!=nil{return empty,ErrUntrustedReceiptMirror}
  var before,after syscall.Stat_t
  if syscall.Fstat(filefd,&before)!=nil||before.Uid!=uid||
   before.Mode&syscall.S_IFMT!=syscall.S_IFREG||
   before.Mode&0o7777!=executorjournal.ReceiptFileMode||
   before.Nlink!=1||before.Size<1||
   before.Size>executorjournal.MaxReceiptBytes{
   _=syscall.Close(filefd)
   return empty,ErrUntrustedReceiptMirror
  }
  file:=os.NewFile(uintptr(filefd),"sanitized-executor-receipt")
  data,readErr:=io.ReadAll(io.LimitReader(file,executorjournal.MaxReceiptBytes+1))
  statErr:=syscall.Fstat(filefd,&after)
  closeErr:=file.Close()
  if readErr!=nil||statErr!=nil||closeErr!=nil||
   int64(len(data))!=before.Size||
   before.Ino!=after.Ino||before.Dev!=after.Dev||
   before.Size!=after.Size||before.Mtim!=after.Mtim||
   before.Ctim!=after.Ctim{
   return empty,ErrUntrustedReceiptMirror
  }
  receipt,err:=executorjournal.ParseReceipt(data)
  if err!=nil||receipt.OperationID+"-v1.json"!=name{
   return empty,ErrUntrustedReceiptMirror
  }
  at,err:=time.Parse(time.RFC3339Nano,receipt.UpdatedAt)
  if err!=nil||at.After(now.Add(time.Minute)){
   return empty,ErrUntrustedReceiptMirror
  }
  result.Rows=append(result.Rows,Row{
   OperationID:receipt.OperationID,Service:receipt.Service,
   Phase:receipt.Phase,Outcome:receipt.Outcome,
   CandidateDigest:receipt.CandidateDigest,UpdatedAt:receipt.UpdatedAt,
   Stale:now.Sub(at)>StaleAfter,
  })
 }
 sort.Slice(result.Rows,func(i,j int)bool{
  return result.Rows[i].UpdatedAt>result.Rows[j].UpdatedAt
 })
 if len(result.Rows)>maxDisplay{result.Rows=result.Rows[:maxDisplay]}
 return result,nil
}
