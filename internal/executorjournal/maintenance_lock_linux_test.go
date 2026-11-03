package executorjournal

import (
 "bufio"
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "syscall"
 "testing"
 "time"
)

func maintenanceFixture(t *testing.T) string {
 t.Helper()
 root,e:=os.MkdirTemp("/root","docklantern-maintenance-")
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = os.RemoveAll(root)})
 if e=os.Chmod(root,0o700);e!=nil{t.Fatal(e)}
 dir:=filepath.Join(root,"locks")
 if e=os.Mkdir(dir,0o700);e!=nil{t.Fatal(e)}
 return dir
}

func TestMaintenanceLockRootFixture(t *testing.T) {
 backupRootEnabled(t)
 dir:=maintenanceFixture(t)
 var zero MaintenanceLease
 if e:=zero.Release();!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("zero lease released fd: %v",e)}
 one,e:=acquireMaintenanceLeaseAt(dir,false)
 if e!=nil{t.Fatal(e)}
 two,e:=acquireMaintenanceLeaseAt(dir,false)
 if e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,true);!errors.Is(e,ErrMaintenanceBusy){
  t.Fatalf("exclusive bypassed two shared leases: %v",e)
 }
 if e=one.Release();e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,true);!errors.Is(e,ErrMaintenanceBusy){
  t.Fatalf("exclusive bypassed remaining reader: %v",e)
 }
 if e=two.Release();e!=nil{t.Fatal(e)}
 exclusive,e:=acquireMaintenanceLeaseAt(dir,true)
 if e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,false);!errors.Is(e,ErrMaintenanceBusy){t.Fatalf("reader bypassed exclusive: %v",e)}
 if _,e=acquireMaintenanceLeaseAt(dir,true);!errors.Is(e,ErrMaintenanceBusy){t.Fatalf("writer bypassed exclusive: %v",e)}
 child:=exec.Command(os.Args[0],"-test.run=^TestMaintenanceLockChild$","-test.count=1")
 child.Env=append(os.Environ(),"DOCKLANTERN_MAINT_CHILD=1","DOCKLANTERN_MAINT_DIR="+dir,"DOCKLANTERN_MAINT_MODE=expect-locked")
 if out,e:=child.CombinedOutput();e!=nil{t.Fatalf("cross-process lock bypassed: %v %s",e,out)}
 if e=exclusive.Release();e!=nil{t.Fatal(e)}
 if e=exclusive.Release();!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("double-release accepted: %v",e)}
 // Terminating another process releases the lock without removing its inode.
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
 defer cancel()
 holder:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestMaintenanceLockChild$","-test.count=1")
 holder.Env=append(os.Environ(),"DOCKLANTERN_MAINT_CHILD=1","DOCKLANTERN_MAINT_DIR="+dir,"DOCKLANTERN_MAINT_MODE=hold")
 out,e:=holder.StdoutPipe()
 if e!=nil{t.Fatal(e)}
 if e=holder.Start();e!=nil{t.Fatal(e)}
 defer func(){if holder.Process!=nil{_ = holder.Process.Kill();_ = holder.Wait()}}()
 line,e:=bufio.NewReader(out).ReadString('\n')
 if e!=nil||line!="LOCKED\n"{t.Fatalf("child did not acquire exclusive: %q %v",line,e)}
 if _,e=acquireMaintenanceLeaseAt(dir,false);!errors.Is(e,ErrMaintenanceBusy){t.Fatalf("child lock bypassed: %v",e)}
 _ = holder.Process.Kill()
 _ = holder.Wait()
 after,e:=acquireMaintenanceLeaseAt(dir,false)
 if e!=nil{t.Fatalf("SIGKILL did not free lock: %v",e)}
 if e=after.Release();e!=nil{t.Fatal(e)}
 path:=filepath.Join(dir,maintenanceLockName)
 if stat,e:=os.Lstat(path);e!=nil||stat.Mode().Perm()!=0o600{t.Fatalf("lock inode not preserved: %#v %v",stat,e)}
 if e=os.Chmod(path,0o640);e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,true);!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("unsafe mode accepted: %v",e)}
 if e=os.Chmod(path,0o600);e!=nil{t.Fatal(e)}
 if e=os.Link(path,filepath.Join(dir,"alias"));e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,true);!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("hardlink accepted: %v",e)}
 if e=os.Remove(filepath.Join(dir,"alias"));e!=nil{t.Fatal(e)}
 if e=os.Chmod(dir,0o770);e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,false);!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("writable ancestor accepted: %v",e)}
 if e=os.Chmod(dir,0o700);e!=nil{t.Fatal(e)}
 if e=os.Remove(path);e!=nil{t.Fatal(e)}
 if e=os.Symlink("/etc/passwd",path);e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,false);!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("symlink accepted: %v",e)}
 if e=os.Remove(path);e!=nil{t.Fatal(e)}
 if e=syscall.Mkfifo(path,0o600);e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(dir,false);!errors.Is(e,ErrMaintenanceUnavailable){t.Fatalf("fifo accepted: %v",e)}
}

func TestMaintenanceLockChild(t *testing.T) {
 if os.Getenv("DOCKLANTERN_MAINT_CHILD")!="1"{t.Skip("fixture child only")}
 dir:=os.Getenv("DOCKLANTERN_MAINT_DIR")
 switch os.Getenv("DOCKLANTERN_MAINT_MODE"){
 case "expect-locked":
  if l,e:=acquireMaintenanceLeaseAt(dir,false);!errors.Is(e,ErrMaintenanceBusy){
   if l!=nil{_ = l.Release()}
   t.Fatalf("cross-process exclusive bypassed: %v",e)
  }
 case "hold":
  l,e:=acquireMaintenanceLeaseAt(dir,true)
  if e!=nil{t.Fatal(e)}
  defer l.Release()
  fmt.Println("LOCKED")
  for{time.Sleep(time.Hour)}
 default:t.Fatal("invalid fixture mode")
 }
}
