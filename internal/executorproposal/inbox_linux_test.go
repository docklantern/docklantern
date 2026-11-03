package executorproposal

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// CI-only disposable root-owned filesystem fixture. The production fixed
// inbox is NEVER created or modified. This test does not contact Docker.
func TestReadInboxRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicitly approved, disposable root-owned test fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-proposal-fixture-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { os.RemoveAll(root) })
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	handoff := filepath.Join(root, "handoff")
	if err := os.Mkdir(handoff, 0o700); err != nil { t.Fatal(err) }
	dir := filepath.Join(handoff, "proposals")
	if err := os.Mkdir(dir, 0o700); err != nil { t.Fatal(err) }
	const monitorUID = uint32(65534) // unprivileged, nonroot fixture owner
	if err := os.Chown(dir, int(monitorUID), -1); err != nil { t.Fatal(err) }
	name := sampleProposal().ProposalID
	file := filepath.Join(dir, name+".json")
	valid := sampleBytes(t)
	write := func(data []byte) {
		t.Helper()
		if err := os.WriteFile(file, data, 0o600); err != nil { t.Fatal(err) }
		if err := os.Chown(file, int(monitorUID), -1); err != nil { t.Fatal(err) }
		if err := os.Chmod(file, 0o600); err != nil { t.Fatal(err) }
	}
	expectRejected := func(message string) {
		t.Helper()
		if _, err := readInbox(dir, name, monitorUID, sampleNow); !errors.Is(err, ErrUnsafeInbox) {
			t.Fatalf("%s: untrusted inbox accepted: %v", message, err)
		}
	}
	write(valid)
	got, err := readInbox(dir, name, monitorUID, sampleNow)
	if err != nil || got.Proposal.ProposalID != name || got.Size != len(valid) {
		t.Fatalf("safe inbox rejected: %#v %v", got, err)
	}
	// Changing even nonauthoritative requested_tag changes exact byte hash.
	edited := bytes.Replace(valid, []byte("latest"), []byte("beta01"), 1)
	write(edited)
	again, err := readInbox(dir, name, monitorUID, sampleNow)
	if err != nil || got.SHA256 == again.SHA256 {
		t.Fatalf("edited proposal preserved hash: %v", err)
	}
	write(valid)
	if _, err := readInbox(dir, name, 0, sampleNow); !errors.Is(err, ErrUnsafeInbox) {
		t.Fatal("root monitor UID accepted")
	}
	if _, err := readInbox(dir, "../etc/passwd", monitorUID, sampleNow); !errors.Is(err, ErrUnsafeInbox) {
		t.Fatal("path traversal ID accepted")
	}
	if _, err := readInbox(dir, name, monitorUID+1, sampleNow); !errors.Is(err, ErrUnsafeInbox) {
		t.Fatal("wrong monitor UID accepted")
	}
	if _, err := readInbox(dir, name, monitorUID, sampleNow.Add(20*time.Minute)); !errors.Is(err, ErrUnsafeInbox) {
		t.Fatal("expired proposal accepted")
	}
	if err := os.Chmod(file, 0o640); err != nil { t.Fatal(err) }
	expectRejected("group-readable proposal")
	write(valid)
	if err := os.Chmod(file, 0o700); err != nil { t.Fatal(err) }
	expectRejected("owner-executable proposal")
	write(valid)
	if err := os.Chmod(file, 0o400); err != nil { t.Fatal(err) }
	expectRejected("non-0600 proposal")
	write(valid)
	if err := os.Chown(file, 0, -1); err != nil { t.Fatal(err) }
	expectRejected("root-owned proposal must not be spoofed as monitor")
	write(valid)
	hardlink := filepath.Join(dir, "untrusted.bak")
	if err := os.Link(file, hardlink); err != nil { t.Fatal(err) }
	expectRejected("hardlinked proposal")
	if err := os.Remove(hardlink); err != nil { t.Fatal(err) }
	if err := os.Remove(file); err != nil { t.Fatal(err) }
	if err := os.Symlink("/etc/passwd", file); err != nil { t.Fatal(err) }
	expectRejected("symlink proposal")
	if err := os.Remove(file); err != nil { t.Fatal(err) }
	if err := syscall.Mkfifo(file, 0o600); err != nil { t.Fatal(err) }
	if err := os.Chown(file, int(monitorUID), -1); err != nil { t.Fatal(err) }
	expectRejected("FIFO proposal")
	if err := os.Remove(file); err != nil { t.Fatal(err) }
	write(bytes.Repeat([]byte("x"), MaxProposalBytes+1))
	expectRejected("oversized proposal")
	write(valid)
	if err := os.Chmod(handoff, 0o770); err != nil { t.Fatal(err) }
	expectRejected("group-writable parent")
	if err := os.Chmod(handoff, 0o700); err != nil { t.Fatal(err) }
	if err := os.Chmod(dir, 0o770); err != nil { t.Fatal(err) }
	expectRejected("monitor-writable group")
	if err := os.Chmod(dir, 0o700); err != nil { t.Fatal(err) }
	// Atomic replacement by monitor before reading has a different hash; no
	// later phase may treat the prior preview hash as authorization.
	if err := os.Remove(file); err != nil { t.Fatal(err) }
	write(edited)
	swapped, err := readInbox(dir, name, monitorUID, sampleNow)
	if err != nil || swapped.SHA256 == got.SHA256 {
		t.Fatalf("replacement not reflected in snapshot fingerprint: %v", err)
	}
	write(valid)
	// At 65 .json entries, reject rather than silently evict or select any.
	extras := make([]string, 0, MaxPending)
	for i := 0; i < MaxPending; i++ {
		extra := filepath.Join(dir, "plan-extra-"+itoa(i)+".json")
		if err := os.WriteFile(extra, valid, 0o600); err != nil { t.Fatal(err) }
		extras = append(extras, extra)
	}
	expectRejected("65 pending proposals")
	for _, extra := range extras {
		if err := os.Remove(extra); err != nil { t.Fatal(err) }
	}
	if err := os.WriteFile(filepath.Join(dir, "unsafe.name.json"), valid, 0o600); err != nil { t.Fatal(err) }
	expectRejected("invalid opaque proposal filename")
	if err := os.Remove(filepath.Join(dir, "unsafe.name.json")); err != nil { t.Fatal(err) }
	// A symlink at the final directory component must also fail no-follow.
	realDir := dir + "-real"
	if err := os.Rename(dir, realDir); err != nil { t.Fatal(err) }
	if err := os.Symlink(realDir, dir); err != nil { t.Fatal(err) }
	expectRejected("symlink inbox")
	if err := os.Remove(dir); err != nil { t.Fatal(err) }
	if err := os.Rename(realDir, dir); err != nil { t.Fatal(err) }
	if _, err := readInbox(dir, name, monitorUID, sampleNow); err != nil {
		t.Fatalf("fixture not restored: %v", err)
	}
}

func itoa(n int) string {
	if n == 0 { return "0" }
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}
	return string(buf[i:])
}
