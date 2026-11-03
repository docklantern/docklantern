package executorproposal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	MaxInboxEntries = 128
	OrphanTempAge   = 30 * time.Minute
)

var ErrUnsafePublication = errors.New("untrusted proposal publication or retention request")

// PublishProposal may be called only by the dedicated *nonroot* monitor
// identity. It publishes data, never executable instructions or approval.
// The inbox must be independently provisioned; this code never chmods or
// creates host directories, and is NOT wired to the monitoring web app yet.
func PublishProposal(proposal Proposal, now time.Time) (Record, error) {
	uid := os.Geteuid()
	if uid <= 0 {
		return Record{}, ErrUnsafePublication
	}
	data, err := json.Marshal(proposal)
	if err != nil {
		return Record{}, ErrUnsafePublication
	}
	return publishInbox(InboxDirectory, uint32(uid), data, now)
}

// CleanupExpired removes only expired, valid proposal v1 files and orphaned
// old publication temp files. It never processes executor-owned files.
// No automatic/background timer is installed by this library.
func CleanupExpired(now time.Time) (int, error) {
	uid := os.Geteuid()
	if uid <= 0 {
		return 0, ErrUnsafePublication
	}
	return cleanupInbox(InboxDirectory, uint32(uid), now)
}

// publishInbox is an internal, testable monitor-side operation. The final
// directory must already be 0700 and owned by the selected UID. All parent
// directories are root-owned; a caller cannot choose arbitrary host paths
// through the production API.
func publishInbox(directory string, uid uint32, data []byte, now time.Time) (Record, error) {
	var empty Record
	if now.IsZero() || len(data) == 0 || len(data) > MaxProposalBytes {
		return empty, ErrUnsafePublication
	}
	record, err := Parse(data, now)
	if err != nil || !validOpaqueID(record.Proposal.ProposalID) {
		return empty, ErrUnsafePublication
	}
	fd, err := openMonitorInbox(directory, uid)
	if err != nil {
		return empty, ErrUnsafePublication
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return empty, ErrUnsafePublication
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	count, _, err := listInboxNames(fd)
	if err != nil || count >= MaxPending || !trustedDirectory(fd, uid, true) {
		return empty, ErrUnsafePublication
	}

	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return empty, ErrUnsafePublication
	}
	temp := ".proposal-" + hex.EncodeToString(randBytes) + ".tmp"
	tempfd, err := syscall.Openat(fd, temp,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600)
	if err != nil {
		return empty, ErrUnsafePublication
	}
	defer syscall.Unlinkat(fd, temp) // only ever remove our own random temp
	file := os.NewFile(uintptr(tempfd), "monitor-proposal-temporary")
	_, writeErr := file.Write(data)
	if writeErr != nil {
		file.Close()
		return empty, ErrUnsafePublication
	}
	if err := syscall.Fchmod(tempfd, 0o600); err != nil {
		file.Close()
		return empty, ErrUnsafePublication
	}
	if _, ok := trustedProposalFile(tempfd, uid); !ok {
		file.Close()
		return empty, ErrUnsafePublication
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return empty, ErrUnsafePublication
	}
	if err := file.Close(); err != nil {
		return empty, ErrUnsafePublication
	}
	if !trustedDirectory(fd, uid, true) {
		return empty, ErrUnsafePublication
	}
	// NOREPLACE prevents same-ID overwrites, even across noncooperating
	// publishers. A proposal's previewed name/bytes are never replaced by
	// our publisher. The reader still treats all monitor files as untrusted.
	if err := unix.Renameat2(fd, temp, fd, record.Proposal.ProposalID+".json", unix.RENAME_NOREPLACE); err != nil {
		return empty, ErrUnsafePublication
	}
	if err := syscall.Fsync(fd); err != nil {
		// Publication may already be visible. Fail closed and never unlink
		// an ambiguous final record.
		return empty, ErrUnsafePublication
	}
	return record, nil
}

// cleanupInbox scans a bounded set under the same cooperating process lock.
// An unsafe or malformed file fails closed instead of being removed. All
// deletions are confined to the already opened private directory.
func cleanupInbox(directory string, uid uint32, now time.Time) (int, error) {
	if now.IsZero() {
		return 0, ErrUnsafePublication
	}
	fd, err := openMonitorInbox(directory, uid)
	if err != nil {
		return 0, ErrUnsafePublication
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return 0, ErrUnsafePublication
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	_, names, err := listInboxNames(fd)
	if err != nil {
		return 0, ErrUnsafePublication
	}
	toDelete := make([]string, 0)
	for _, name := range names {
		if strings.HasSuffix(name, ".json") {
			if !validOpaqueID(strings.TrimSuffix(name, ".json")) {
				return 0, ErrUnsafePublication
			}
			data, before, err := readMonitorFile(fd, name, uid)
			if err != nil {
				return 0, ErrUnsafePublication
			}
			// Parse against its canonical creation timestamp so expired
			// records can be validated without bypassing strict v1 rules.
			var envelope struct {
				CreatedAt string `json:"created_at"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil {
				return 0, ErrUnsafePublication
			}
			created, err := parseTimestamp(envelope.CreatedAt)
			if err != nil {
				return 0, ErrUnsafePublication
			}
			record, err := Parse(data, created)
			if err != nil || record.Proposal.ProposalID != strings.TrimSuffix(name, ".json") {
				return 0, ErrUnsafePublication
			}
			expiry, err := parseTimestamp(record.Proposal.ExpiresAt)
			if err != nil {
				return 0, ErrUnsafePublication
			}
			if !expiry.After(now) {
				if err := verifyStillSameFile(fd, name, uid, before); err != nil {
					return 0, ErrUnsafePublication
				}
				toDelete = append(toDelete, name)
			}
		} else if validTempName(name) {
			// A crashed publisher may leave an unfinished temp. Never delete
			// a recent one; require both mtime and ctime older than 30 min.
			st, err := openTempStat(fd, name, uid)
			if err != nil {
				return 0, ErrUnsafePublication
			}
			if oldEnough(now, st.Mtim) && oldEnough(now, st.Ctim) {
				toDelete = append(toDelete, name)
			}
		} else {
			// Reject unexpected files rather than removing or following
			// anything not owned by this narrow publication protocol.
			return 0, ErrUnsafePublication
		}
	}

	deleted := 0
	for _, name := range toDelete {
		if !trustedDirectory(fd, uid, true) {
			return deleted, ErrUnsafePublication
		}
		// One final descriptor identity check minimizes races between scan
		// and unlink. A hostile process with the SAME monitor UID can still
		// race names; its files are untrusted and never execution authority.
		if strings.HasSuffix(name, ".json") {
			_, st, err := readMonitorFile(fd, name, uid)
			if err != nil {
				return deleted, ErrUnsafePublication
			}
			// A changed timestamp/content can invalidate cleanup: re-read
			// and validate freshness immediately below.
			if st.Size == 0 {
				return deleted, ErrUnsafePublication
			}
		} else {
			if _, err := openTempStat(fd, name, uid); err != nil {
				return deleted, ErrUnsafePublication
			}
		}
		if err := syscall.Unlinkat(fd, name); err != nil {
			return deleted, ErrUnsafePublication
		}
		deleted++
	}
	if deleted > 0 && syscall.Fsync(fd) != nil {
		return deleted, ErrUnsafePublication
	}
	return deleted, nil
}

func openMonitorInbox(directory string, uid uint32) (int, error) {
	if !validInboxPath(directory) || uid == 0 && directory == InboxDirectory {
		return -1, ErrUnsafePublication
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrUnsafePublication
	}
	if !trustedDirectory(fd, 0, false) {
		syscall.Close(fd)
		return -1, ErrUnsafePublication
	}
	parts := strings.Split(strings.TrimPrefix(directory, "/"), "/")
	if len(parts) < 2 || len(parts) > 32 {
		syscall.Close(fd)
		return -1, ErrUnsafePublication
	}
	for i, name := range parts {
		if name == "" || name == "." || name == ".." {
			syscall.Close(fd)
			return -1, ErrUnsafePublication
		}
		next, err := syscall.Openat(fd, name,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if err != nil {
			return -1, ErrUnsafePublication
		}
		fd = next
		owner := uint32(0)
		private := false
		if i == len(parts)-1 {
			owner, private = uid, true
		}
		if !trustedDirectory(fd, owner, private) {
			syscall.Close(fd)
			return -1, ErrUnsafePublication
		}
	}
	return fd, nil
}

func listInboxNames(fd int) (int, []string, error) {
	dup, err := syscall.Openat(fd, ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return 0, nil, ErrUnsafePublication
	}
	file := os.NewFile(uintptr(dup), "bounded-monitor-proposal-inbox")
	entries, err := file.ReadDir(MaxInboxEntries + 1)
	_ = file.Close()
	if err != nil && !errors.Is(err, io.EOF) || len(entries) > MaxInboxEntries {
		return 0, nil, ErrUnsafePublication
	}
	count := 0
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			if !validOpaqueID(strings.TrimSuffix(name, ".json")) {
				return 0, nil, ErrUnsafePublication
			}
			count++
		} else if !validTempName(name) {
			return 0, nil, ErrUnsafePublication
		}
		names = append(names, name)
	}
	return count, names, nil
}

func validTempName(name string) bool {
	const prefix, suffix = ".proposal-", ".tmp"
	if len(name) != len(prefix)+32+len(suffix) ||
		!strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	for _, c := range name[len(prefix):len(prefix)+32] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func readMonitorFile(fd int, name string, uid uint32) ([]byte, syscall.Stat_t, error) {
	opened, err := syscall.Openat(fd, name,
		syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, syscall.Stat_t{}, ErrUnsafePublication
	}
	file := os.NewFile(uintptr(opened), "monitor-inbox-entry")
	defer file.Close()
	before, ok := trustedProposalFile(opened, uid)
	if !ok {
		return nil, syscall.Stat_t{}, ErrUnsafePublication
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxProposalBytes+1))
	if err != nil || int64(len(data)) != before.Size || len(data) > MaxProposalBytes {
		return nil, syscall.Stat_t{}, ErrUnsafePublication
	}
	after, ok := trustedProposalFile(opened, uid)
	if !ok || !sameSnapshot(before, after) {
		return nil, syscall.Stat_t{}, ErrUnsafePublication
	}
	return data, after, nil
}

func verifyStillSameFile(fd int, name string, uid uint32, expected syscall.Stat_t) error {
	_, latest, err := readMonitorFile(fd, name, uid)
	if err != nil || !sameSnapshot(expected, latest) {
		return ErrUnsafePublication
	}
	return nil
}

func openTempStat(fd int, name string, uid uint32) (syscall.Stat_t, error) {
	opened, err := syscall.Openat(fd, name,
		syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return syscall.Stat_t{}, ErrUnsafePublication
	}
	defer syscall.Close(opened)
	var st syscall.Stat_t
	if syscall.Fstat(opened, &st) != nil || st.Uid != uid ||
		st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Mode&0o7777 != 0o600 ||
		st.Nlink != 1 || st.Size < 0 || st.Size > MaxProposalBytes {
		return syscall.Stat_t{}, ErrUnsafePublication
	}
	return st, nil
}

func oldEnough(now time.Time, ts syscall.Timespec) bool {
	then := time.Unix(ts.Sec, ts.Nsec)
	return now.After(then) && now.Sub(then) > OrphanTempAge
}

// safeInboxPath is only used by test fixture paths. Production always uses
// InboxDirectory and disallows caller-supplied location parameters.
var _ = filepath.Clean
