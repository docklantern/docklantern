package executorjournal

import (
 "errors"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "syscall"
)

// HostMaintenanceLockDirectory is independently provisioned host-private
// executor storage. No production CLI, daemon or restore action calls these
// primitives in Phase 3c-c.
const HostMaintenanceLockDirectory = "/var/lib/docklantern/executor/locks"
const maintenanceLockName = "_maintenance.lock"

var (
 ErrMaintenanceUnavailable = errors.New("protected executor maintenance lock unavailable")
 ErrMaintenanceBusy = errors.New("executor maintenance lock held")
)

// MaintenanceLease is an advisory cross-process fence. Shared leases are
// intended for future cooperating executor operations, exclusive leases for
// host-wide offline maintenance. This does NOT stop processes that do not
// acquire a lease, root operators, Docker or arbitrary host writes.
type MaintenanceLease struct {
 mu sync.Mutex
 fd int
 exclusive bool
 released bool
}

// AcquireMaintenanceLease only obtains a lock; it does not restore or mutate
// any journal or bypass the existing per-project operation locks.
func AcquireMaintenanceLease(exclusive bool) (*MaintenanceLease,error) {
 return acquireMaintenanceLeaseAt(HostMaintenanceLockDirectory,exclusive)
}

func acquireMaintenanceLeaseAt(directory string, exclusive bool) (*MaintenanceLease,error) {
 if os.Geteuid()!=0 || !filepath.IsAbs(directory) || directory=="/" ||
  filepath.Clean(directory)!=directory || len(directory)>512 {
  return nil,ErrMaintenanceUnavailable
 }
 parts:=strings.Split(strings.TrimPrefix(directory,"/"),"/")
 if len(parts)<2||len(parts)>32{return nil,ErrMaintenanceUnavailable}
 dirfd,err:=syscall.Open("/",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
 if err!=nil{return nil,ErrMaintenanceUnavailable}
 defer syscall.Close(dirfd)
 if !trustedJournalDir(dirfd,false){return nil,ErrMaintenanceUnavailable}
 for i,p:=range parts {
  if p==""||p=="."||p==".."{return nil,ErrMaintenanceUnavailable}
  next,e:=syscall.Openat(dirfd,p,syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
  if e!=nil{return nil,ErrMaintenanceUnavailable}
  syscall.Close(dirfd)
  dirfd=next
  if !trustedJournalDir(dirfd,i==len(parts)-1){return nil,ErrMaintenanceUnavailable}
 }
 created:=false
 fd,err:=syscall.Openat(dirfd,maintenanceLockName,
  syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0o600)
 if err==nil{created=true}else if errors.Is(err,syscall.EEXIST){
  fd,err=syscall.Openat(dirfd,maintenanceLockName,syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 }
 if err!=nil{return nil,ErrMaintenanceUnavailable}
 keep:=false
 defer func(){if !keep{syscall.Close(fd)}}()
 if !trustedJournalFile(fd){return nil,ErrMaintenanceUnavailable}
 how:=syscall.LOCK_SH|syscall.LOCK_NB
 if exclusive{how=syscall.LOCK_EX|syscall.LOCK_NB}
 if err=syscall.Flock(fd,how);err!=nil{
  if errors.Is(err,syscall.EWOULDBLOCK)||errors.Is(err,syscall.EAGAIN){return nil,ErrMaintenanceBusy}
  return nil,ErrMaintenanceUnavailable
 }
 check,err:=syscall.Openat(dirfd,maintenanceLockName,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if err!=nil{return nil,ErrMaintenanceUnavailable}
 var a,b syscall.Stat_t
 e1:=syscall.Fstat(fd,&a)
 e2:=syscall.Fstat(check,&b)
 ok:=trustedJournalFile(check)
 syscall.Close(check)
 if e1!=nil||e2!=nil||!ok||a.Dev!=b.Dev||a.Ino!=b.Ino||!trustedJournalDir(dirfd,true){
  return nil,ErrMaintenanceUnavailable
 }
 if created&&(syscall.Fsync(fd)!=nil||syscall.Fsync(dirfd)!=nil){return nil,ErrMaintenanceUnavailable}
 keep=true
 return &MaintenanceLease{fd:fd,exclusive:exclusive},nil
}

func (l *MaintenanceLease) Release() error {
 if l==nil{return ErrMaintenanceUnavailable}
 l.mu.Lock()
 defer l.mu.Unlock()
 if l.released||l.fd<0{return ErrMaintenanceUnavailable}
 // A zero-value lease must never release descriptor 0.
 if l.fd==0{return ErrMaintenanceUnavailable}
 l.released=true
 unlock:=syscall.Flock(l.fd,syscall.LOCK_UN)
 closeErr:=syscall.Close(l.fd)
 l.fd=-1
 if unlock!=nil||closeErr!=nil{return ErrMaintenanceUnavailable}
 return nil
}
