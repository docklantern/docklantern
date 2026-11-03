package executorproposal

import (
	"bytes"
	"fmt"
	"encoding/json"
	"io"
	"os/exec"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// The fixture is intentionally root-owned and isolated in /root, never the
// production inbox. A nonroot runtime identity must be validated separately
// before enabling the publisher in the monitoring process.
func TestProposalPublicationRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit disposable root-owned CI fixture")
	}
	dir := rootInboxFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	p := proposalForPublish(now, "candidate-first")
	payload := marshalProposal(t, p)
	first, err := publishInbox(dir, 0, payload, now)
	if err != nil || first.Proposal.ProposalID != p.ProposalID {
		t.Fatalf("first publish rejected: %#v %v", first, err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "candidate-first.json"))
	if err != nil || !bytes.Equal(onDisk, payload) {
		t.Fatalf("not published atomically: %v", err)
	}
	if _, err := publishInbox(dir, 0, payload, now); err == nil {
		t.Fatal("same ID was published twice")
	}
	changed := proposalForPublish(now, "candidate-first")
	changed.Candidate.RequestedTag = "beta"
	if _, err := publishInbox(dir, 0, marshalProposal(t, changed), now); err == nil {
		t.Fatal("same-ID candidate replaced published proposal")
	}
	still, err := os.ReadFile(filepath.Join(dir, "candidate-first.json"))
	if err != nil || !bytes.Equal(still, payload) {
		t.Fatal("failed replacement modified original bytes")
	}
	if _, err := publishInbox(dir, 0, payload, now.Add(11*time.Minute)); err == nil {
		t.Fatal("expired proposal was accepted")
	}
	if _, err := publishInbox(dir, 0, []byte("{}"), now); err == nil {
		t.Fatal("malformed proposal published")
	}
	if _, err := PublishProposal(p, now); err == nil {
		t.Fatal("production monitor publisher accepted root EUID")
	}
	if _, err := CleanupExpired(now); err == nil {
		t.Fatal("production cleanup accepted root EUID")
	}
	removed, err := cleanupInbox(dir, 0, now.Add(11*time.Minute))
	if err != nil || removed != 1 {
		t.Fatalf("expired entry not removed: deleted=%d err=%v", removed, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "candidate-first.json")); !os.IsNotExist(err) {
		t.Fatal("expired entry not removed from opened private inbox")
	}
}

func TestProposalQuotaAndConcurrentPublicationRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit disposable root-owned CI fixture")
	}
	dir := rootInboxFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	// Races must never exceed 64 pending files. A busy nonblocking lock may
	// reject otherwise eligible operations, which is an acceptable fail-closed
	// behavior rather than exceeding capacity.
	const concurrent = 20
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := publishInbox(dir, 0, marshalProposalValue(proposalForPublish(now, fmt.Sprintf("par-%d", i))), now)
			if err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != int(successes.Load()) || len(entries) > MaxPending {
		t.Fatalf("concurrent publication corrupted quota: entries=%d success=%d", len(entries), successes.Load())
	}
	for i := concurrent; len(entries) < MaxPending; i++ {
		if _, err := publishInbox(dir, 0, marshalProposalValue(proposalForPublish(now, fmt.Sprintf("fill-%d", i))), now); err != nil {
			t.Fatalf("quota fill failed at %d entries: %v", len(entries), err)
		}
		entries, err = os.ReadDir(dir)
		if err != nil { t.Fatal(err) }
	}
	if _, err := publishInbox(dir, 0, marshalProposalValue(proposalForPublish(now, "over-capacity")), now); err == nil {
		t.Fatal("65th pending proposal accepted")
	}
	// A cleanup before expiration cannot remove a single valid proposal.
	if removed, err := cleanupInbox(dir, 0, now); err != nil || removed != 0 {
		t.Fatalf("removed current proposals: n=%d err=%v", removed, err)
	}
	removed, err := cleanupInbox(dir, 0, now.Add(11*time.Minute))
	if err != nil || removed != MaxPending {
		t.Fatalf("expiration sweep did not recover quota: %d %v", removed, err)
	}
	if _, err := publishInbox(dir, 0, marshalProposalValue(proposalForPublish(now.Add(12*time.Minute), "new-candidate")), now.Add(12*time.Minute)); err != nil {
		t.Fatal("failed to publish after expiry cleanup:", err)
	}
}

func TestProposalUnsafeFilesAndFailClosedRetentionRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit disposable root-owned CI fixture")
	}
	dir := rootInboxFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	p := proposalForPublish(now, "do-not-delete")
	payload := marshalProposal(t, p)
	target := filepath.Join(dir, p.ProposalID+".json")
	// An unauthorized filename must never be replaced or cleaned up.
	if err := os.Symlink("/etc/passwd", target); err != nil { t.Fatal(err) }
	if _, err := publishInbox(dir, 0, payload, now); err == nil {
		t.Fatal("publisher overwrote symlink")
	}
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("cleanup followed symlink")
	}
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was modified")
	}
	if err := os.Remove(target); err != nil { t.Fatal(err) }
	if err := syscall.Mkfifo(target, 0o600); err != nil { t.Fatal(err) }
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("cleanup accepted FIFO")
	}
	if err := os.Remove(target); err != nil { t.Fatal(err) }
	if err := os.WriteFile(target, []byte("{"), 0o600); err != nil { t.Fatal(err) }
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("malformed file was deleted")
	}
	if err := os.Remove(target); err != nil { t.Fatal(err) }
	if err := os.WriteFile(target, payload, 0o600); err != nil { t.Fatal(err) }
	hard := filepath.Join(dir, ".proposal-"+strings.Repeat("f",32)+".tmp")
	if err := os.Link(target, hard); err != nil { t.Fatal(err) }
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("hardlinked file accepted for cleanup")
	}
	if err := os.Remove(hard); err != nil { t.Fatal(err) }
	if err := os.Chmod(target, 0o640); err != nil { t.Fatal(err) }
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("group-readable file accepted for cleanup")
	}
	if err := os.Chmod(target, 0o600); err != nil { t.Fatal(err) }
	if err := os.Chmod(dir, 0o770); err != nil { t.Fatal(err) }
	if _, err := publishInbox(dir, 0, payload, now); err == nil {
		t.Fatal("group-writable inbox accepted by publisher")
	}
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("group-writable inbox accepted by cleaner")
	}
	if err := os.Chmod(dir, 0o700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(dir, "escape.json"), []byte("invalid"), 0o600); err != nil { t.Fatal(err) }
	if _, err := cleanupInbox(dir, 0, now.Add(11*time.Minute)); err == nil {
		t.Fatal("invalid file allowed partial cleanup")
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatal("an unrelated invalid entry caused valid file deletion")
	}
}

func TestProposalTemporaryFilesAreBoundedAndOnlyCleanedWhenOld(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit disposable root-owned CI fixture")
	}
	dir := rootInboxFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	name := ".proposal-" + strings.Repeat("a", 32) + ".tmp"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o600); err != nil { t.Fatal(err) }
	if removed, err := cleanupInbox(dir, 0, now); err != nil || removed != 0 {
		t.Fatalf("removed fresh publication temp: %d %v", removed, err)
	}
	if err := os.Chtimes(path, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil { t.Fatal(err) }
	// A backdated mtime alone cannot bypass the protected ctime age.
	if removed, err := cleanupInbox(dir, 0, now); err != nil || removed != 0 {
		t.Fatalf("removed temp using only a forged old mtime: %d %v", removed, err)
	}
	if removed, err := cleanupInbox(dir, 0, now.Add(45*time.Minute)); err != nil || removed != 1 {
		t.Fatalf("old orphan not removed: %d %v", removed, err)
	}
}

func rootInboxFixture(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("/root", "docklantern-proposal-publisher-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if err := os.Chmod(base, 0o700); err != nil { t.Fatal(err) }
	dir := filepath.Join(base, "inbox")
	if err := os.Mkdir(dir, 0o700); err != nil { t.Fatal(err) }
	return dir
}

func proposalForPublish(now time.Time, id string) Proposal {
	p := sampleProposal()
	p.ProposalID = id
	p.CreatedAt = now.Add(-time.Minute).Format(time.RFC3339Nano)
	p.ExpiresAt = now.Add(10*time.Minute).Format(time.RFC3339Nano)
	return p
}

func marshalProposal(t *testing.T, p Proposal) []byte {
	t.Helper()
	return marshalProposalValue(p)
}

func marshalProposalValue(p Proposal) []byte {
	b, _ := json.Marshal(p)
	return b
}

// TestActualUnprivilegedMonitorPublisherRootFixture verifies the *monitor*
// publisher code with EUID 65534 on an isolated tree under /opt, while a
// separate root process reads the published record as the future executor
// would. No production file or Docker socket is accessed.
func TestActualUnprivilegedMonitorPublisherRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires root-owned disposable fixture on isolated CI")
	}
	base, err := os.MkdirTemp("/opt", "docklantern-monitor-publisher-")
	if err != nil {t.Fatal(err)}
	t.Cleanup(func(){_ = os.RemoveAll(base)})
	if err:=os.Chmod(base,0o755);err!=nil {t.Fatal(err)}
	dir:=filepath.Join(base,"inbox")
	if err:=os.Mkdir(dir,0o700);err!=nil {t.Fatal(err)}
	if err:=os.Chown(dir,65534,65534);err!=nil {t.Fatal(err)}
	original,err:=os.Executable()
	if err!=nil {t.Fatal(err)}
	from,err:=os.Open(original)
	if err!=nil {t.Fatal(err)}
	defer from.Close()
	binary:=filepath.Join(base,"proposal-fixture-test")
	to,err:=os.OpenFile(binary,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0o700)
	if err!=nil {t.Fatal(err)}
	if _,err:=io.Copy(to,from);err!=nil {to.Close();t.Fatal(err)}
	if err:=to.Close();err!=nil {t.Fatal(err)}
	if err:=os.Chmod(binary,0o755);err!=nil {t.Fatal(err)}
	perform:=func(action string){
		t.Helper()
		cmd:=exec.Command(binary,"-test.run=^TestUnprivilegedMonitorPublisherChild$","-test.count=1")
		cmd.Dir="/"
		cmd.Env=[]string{
			"PATH=/usr/bin:/bin",
			"DOCKLANTERN_PROPOSAL_CHILD=1",
			"DOCKLANTERN_PROPOSAL_ACTION="+action,
			"DOCKLANTERN_PROPOSAL_TEST_INBOX="+dir,
		}
		cmd.SysProcAttr=&syscall.SysProcAttr{Credential:&syscall.Credential{Uid:65534,Gid:65534}}
		if out,err:=cmd.CombinedOutput();err!=nil {
			t.Fatalf("nonroot monitor child %s failed: %v, output=%s",action,err,out)
		}
	}
	perform("publish")
	record,err:=readInbox(dir,"nonroot-demo",65534,time.Now())
	if err!=nil || record.Proposal.ProposalID!="nonroot-demo" {
		t.Fatalf("root could not inspect nonroot proposal: %v %#v",err,record)
	}
	perform("cleanup")
	if _,err:=os.Lstat(filepath.Join(dir,"nonroot-demo.json"));!os.IsNotExist(err){
		t.Fatal("monitor cleanup did not remove expired proposal")
	}
}

func TestUnprivilegedMonitorPublisherChild(t *testing.T) {
	if os.Getenv("DOCKLANTERN_PROPOSAL_CHILD")!="1" {
		t.Skip("only executed as isolated nonroot subprocess")
	}
	if os.Geteuid()!=65534 {
		t.Fatal("child did not run as distinct nonroot UID")
	}
	dir:=os.Getenv("DOCKLANTERN_PROPOSAL_TEST_INBOX")
	// Exercise the separately validated path/lock preconditions first, so
	// failures in an actual nonroot filesystem are diagnosable in CI without
	// exposing private host state or weakening production fail-closed errors.
	fd, openErr := openMonitorInbox(dir, 65534)
	if openErr != nil {
		t.Fatalf("nonroot protected inbox open rejected: %v", openErr)
	}
	if lockErr := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); lockErr != nil {
		syscall.Close(fd)
		t.Fatalf("nonroot inbox directory lock rejected: %v", lockErr)
	}
	_ = syscall.Flock(fd, syscall.LOCK_UN)
	_ = syscall.Close(fd)
	now:=time.Now().UTC().Truncate(time.Second)
	switch os.Getenv("DOCKLANTERN_PROPOSAL_ACTION"){
	case "publish":
		_,err:=publishInbox(dir,65534,marshalProposalValue(proposalForPublish(now,"nonroot-demo")),now)
		if err!=nil {t.Fatal(err)}
	case "cleanup":
		removed,err:=cleanupInbox(dir,65534,now.Add(11*time.Minute))
		if err!=nil || removed!=1 {t.Fatalf("nonroot expiry cleanup: %d %v",removed,err)}
	default:
		t.Fatal("invalid isolated child action")
	}
}
