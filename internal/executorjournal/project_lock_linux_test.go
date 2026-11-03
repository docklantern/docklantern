package executorjournal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// The fixture stays entirely within /root on the disposable CI runner.
// Never create the production executor lock directory or edit VPS files.
func TestProjectLockRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit disposable root-owned CI fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-project-lock-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	dir := filepath.Join(root, "locks")
	if err := os.Mkdir(dir, 0o700); err != nil { t.Fatal(err) }

	first, err := acquireProjectLockAt(dir, "hello-demo")
	if err != nil { t.Fatalf("first lock failed: %v", err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrProjectLocked) {
		t.Fatalf("same process bypassed lock: %v", err)
	}
	independent, err := acquireProjectLockAt(dir, "other-demo")
	if err != nil { t.Fatalf("independent project incorrectly blocked: %v", err) }
	if err := independent.Release(); err != nil { t.Fatal(err) }
	if err := independent.Release(); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("double-release silently accepted")
	}
	callChild := func(mode, project string) ([]byte,error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProjectLockSubprocess$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"DOCKLANTERN_PROJECT_LOCK_CHILD=1",
			"DOCKLANTERN_PROJECT_LOCK_DIR="+dir,
			"DOCKLANTERN_PROJECT_LOCK_MODE="+mode,
			"DOCKLANTERN_PROJECT_LOCK_ID="+project)
		return cmd.CombinedOutput()
	}
	if out, err := callChild("expect-locked", "hello-demo"); err != nil {
		t.Fatalf("separate process bypassed active lock: %v, %s",err,out)
	}
	if err := first.Release(); err != nil { t.Fatal(err) }

	// Child holds a lock until killed; kernel releases it without unlinking.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProjectLockSubprocess$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"DOCKLANTERN_PROJECT_LOCK_CHILD=1",
		"DOCKLANTERN_PROJECT_LOCK_DIR="+dir,
		"DOCKLANTERN_PROJECT_LOCK_MODE=hold",
		"DOCKLANTERN_PROJECT_LOCK_ID=hello-demo")
	stdout, err := cmd.StdoutPipe()
	if err != nil { t.Fatal(err) }
	if err := cmd.Start(); err != nil { t.Fatal(err) }
	t.Cleanup(func() { if cmd.Process != nil { _ = cmd.Process.Kill() } })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "LOCKED" {
		_ = cmd.Process.Kill(); _ = cmd.Wait()
		t.Fatalf("child never signaled lock acquisition: %q %v",line,err)
	}
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrProjectLocked) {
		t.Fatalf("parent bypassed child lock: %v", err)
	}
	other, err := acquireProjectLockAt(dir, "other-demo")
	if err != nil { t.Fatalf("independent project blocked by child: %v",err) }
	_ = other.Release()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	recovered, err := acquireProjectLockAt(dir, "hello-demo")
	if err != nil { t.Fatalf("SIGKILL did not release kernel lock: %v",err) }
	if err := recovered.Release(); err != nil { t.Fatal(err) }
	path := filepath.Join(dir, "hello-demo.lock")
	if inf, err := os.Lstat(path); err != nil || inf.Mode().Perm() != 0o600 {
		t.Fatalf("lock file should survive release as private 0600: %#v %v",inf,err)
	}
	if err := os.Chmod(path, 0o640); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("group-readable lock file accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil { t.Fatal(err) }
	hard := filepath.Join(dir, "hardlink")
	if err := os.Link(path, hard); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("hardlinked project lock accepted")
	}
	if err := os.Remove(hard); err != nil { t.Fatal(err) }
	if err := os.Chown(path, 65534, -1); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("monitor-owned project lock accepted")
	}
	if err := os.Chown(path, 0, -1); err != nil { t.Fatal(err) }
	if err := os.Remove(path); err != nil { t.Fatal(err) }
	if err := os.Symlink("/etc/passwd", path); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("symlinked project lock accepted")
	}
	if err := os.Remove(path); err != nil { t.Fatal(err) }
	if err := syscall.Mkfifo(path, 0o600); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("nonregular project lock accepted")
	}
	if err := os.Remove(path); err != nil { t.Fatal(err) }
	if err := os.Chmod(dir, 0o770); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("group writable locks directory accepted")
	}
	if err := os.Chmod(dir, 0o700); err != nil { t.Fatal(err) }
	if err := os.Chmod(root, 0o770); err != nil { t.Fatal(err) }
	if _, err := acquireProjectLockAt(dir, "hello-demo"); !errors.Is(err, ErrLockUnavailable) {
		t.Fatal("writable ancestor accepted")
	}
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	for _, id := range []string{"", "../escape", "hello/other", "HELLO", "-bad"} {
		if _, err := acquireProjectLockAt(dir, id); !errors.Is(err, ErrLockUnavailable) {
			t.Errorf("unsafe project ID %q accepted: %v",id,err)
		}
	}
	final, err := acquireProjectLockAt(dir, "hello-demo")
	if err != nil { t.Fatalf("trusted lock path not restored: %v",err) }
	_ = final.Release()
}

func TestProjectLockSubprocess(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROJECT_LOCK_CHILD") != "1" {
		t.Skip("only used by isolated root CI project-lock fixture")
	}
	dir := os.Getenv("DOCKLANTERN_PROJECT_LOCK_DIR")
	id := os.Getenv("DOCKLANTERN_PROJECT_LOCK_ID")
	switch os.Getenv("DOCKLANTERN_PROJECT_LOCK_MODE") {
	case "expect-locked":
		l, err := acquireProjectLockAt(dir,id)
		if l != nil { _ = l.Release() }
		if !errors.Is(err,ErrProjectLocked) { t.Fatalf("interprocess lock bypass: %v",err) }
	case "hold":
		l, err := acquireProjectLockAt(dir,id)
		if err != nil { t.Fatal(err) }
		defer l.Release()
		fmt.Fprintln(os.Stdout,"LOCKED")
		for { time.Sleep(time.Hour) }
	default:
		t.Fatal("unrecognized test-only lock subprocess")
	}
}

func TestZeroValueProjectLockFailsClosed(t *testing.T) {
	var lock ProjectLock
	if err := lock.Release(); !errors.Is(err, ErrLockUnavailable) {
		t.Fatalf("zero-value project lock affected an unrelated descriptor: %v", err)
	}
}

func TestProjectLockGoroutinesNeverOverlapRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires root-owned disposable fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-project-lock-concurrent-")
	if err != nil { t.Fatal(err) }
	defer os.RemoveAll(root)
	if err := os.Chmod(root,0o700); err != nil { t.Fatal(err) }
	dir := filepath.Join(root,"locks")
	if err := os.Mkdir(dir,0o700); err != nil { t.Fatal(err) }
	var active, maximum atomic.Int64
	var wg sync.WaitGroup
	for i:=0;i<16;i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock, err := acquireProjectLockAt(dir,"same-project")
			if errors.Is(err,ErrProjectLocked) { return }
			if err != nil { t.Errorf("unexpected contention error: %v",err); return }
			v := active.Add(1)
			for {
				cur:=maximum.Load()
				if v <= cur || maximum.CompareAndSwap(cur,v) { break }
			}
			time.Sleep(10*time.Millisecond)
			active.Add(-1)
			if err:=lock.Release(); err!=nil { t.Error(err) }
		}()
	}
	wg.Wait()
	if maximum.Load()!=1 || active.Load()!=0 {
		t.Fatalf("concurrent holders overlapped: max=%d active=%d",maximum.Load(),active.Load())
	}
}
