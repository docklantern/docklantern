package executorpreview

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// CI-only isolated root fixture: never touches the fixed production
// /var/lib/docklantern path, Docker socket, or monitor container.
func TestProtectedMonitorUIDRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PREVIEW_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicitly enabled root-owned disposable fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-monitor-identity-")
	if err != nil {t.Fatal(err)}
	t.Cleanup(func(){_ = os.RemoveAll(root)})
	if err:=os.Chmod(root,0o700);err!=nil {t.Fatal(err)}
	executor:=filepath.Join(root,"executor")
	if err:=os.Mkdir(executor,0o700);err!=nil {t.Fatal(err)}
	file:=filepath.Join(executor,"monitor.uid")
	write:=func(body string){
		t.Helper()
		if err:=os.WriteFile(file,[]byte(body),0o600);err!=nil {t.Fatal(err)}
		if err:=os.Chown(file,0,-1);err!=nil {t.Fatal(err)}
		if err:=os.Chmod(file,0o600);err!=nil {t.Fatal(err)}
	}
	reject:=func(label string){
		t.Helper()
		if _,err:=loadMonitorUIDFrom(file);!errors.Is(err,ErrPreviewDenied){
			t.Fatalf("%s: unsafe monitor UID accepted: %v",label,err)
		}
	}
	write("65534\n")
	uid,err:=loadMonitorUIDFrom(file)
	if err!=nil || uid!=65534 {t.Fatalf("trusted UID file rejected: %d %v",uid,err)}
	for _,invalid:=range []string{
		"0\n","00042\n","-1\n","+1\n","root\n","1 2\n",
		"1\n2\n","4294967296\n","1\n\n","1234567890123"," 42\n",
	} {
		write(invalid)
		reject("invalid UID contents")
	}
	write("65534\n")
	for _,mode:=range []os.FileMode{0o640,0o400,0o700,0o666,0o604}{
		if err:=os.Chmod(file,mode);err!=nil {t.Fatal(err)}
		reject("unsafe identity file mode")
		write("65534\n")
	}
	if err:=os.Chown(file,65534,-1);err!=nil {t.Fatal(err)}
	reject("monitor-owned identity file")
	write("65534\n")
	link:=filepath.Join(executor,"other")
	if err:=os.Link(file,link);err!=nil {t.Fatal(err)}
	reject("hardlinked identity file")
	if err:=os.Remove(link);err!=nil {t.Fatal(err)}
	if err:=os.Remove(file);err!=nil {t.Fatal(err)}
	if err:=os.Symlink("/etc/passwd",file);err!=nil {t.Fatal(err)}
	reject("symlink identity file")
	if err:=os.Remove(file);err!=nil {t.Fatal(err)}
	if err:=syscall.Mkfifo(file,0o600);err!=nil {t.Fatal(err)}
	reject("FIFO identity file")
	if err:=os.Remove(file);err!=nil {t.Fatal(err)}
	write("65534\n")
	if err:=os.Chmod(executor,0o750);err!=nil {t.Fatal(err)}
	reject("nonprivate executor parent")
	if err:=os.Chmod(executor,0o700);err!=nil {t.Fatal(err)}
	if err:=os.Chmod(root,0o770);err!=nil {t.Fatal(err)}
	reject("writable ancestor")
	if err:=os.Chmod(root,0o700);err!=nil {t.Fatal(err)}
	real:=executor+"-real"
	if err:=os.Rename(executor,real);err!=nil {t.Fatal(err)}
	if err:=os.Symlink(real,executor);err!=nil {t.Fatal(err)}
	reject("symlink ancestor")
	if err:=os.Remove(executor);err!=nil {t.Fatal(err)}
	if err:=os.Rename(real,executor);err!=nil {t.Fatal(err)}
	if again,err:=loadMonitorUIDFrom(file);err!=nil || again!=65534 {
		t.Fatalf("identity fixture not restored: %d %v",again,err)
	}
	for _,bad:=range []string{"","/","etc/passwd","/root/../etc/passwd","/root//executor/monitor.uid"}{
		if _,err:=loadMonitorUIDFrom(bad);!errors.Is(err,ErrPreviewDenied) {
			t.Fatalf("unsafe identity path accepted: %q",bad)
		}
	}
}
