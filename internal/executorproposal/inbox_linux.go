package executorproposal

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const InboxDirectory = "/var/lib/docklantern/handoff/proposals"

var ErrUnsafeInbox = errors.New("untrusted proposal inbox")

// LoadInbox only accepts an opaque proposal ID and independently trusted,
// nonroot monitor UID. No caller-provided path can reach the production reader.
// Its result remains untrusted and must never trigger a Docker write.
func LoadInbox(id string, trustedMonitorUID uint32, now time.Time) (Record, error) {
	return readInbox(InboxDirectory, id, trustedMonitorUID, now)
}

// readInbox opens each root-owned ancestor by descriptor without following
// symlinks. The final 0700 inbox is owned by the independently trusted monitor
// UID. The selected regular 0600 file must have exactly one link. Monitoring
// can edit/replace its own files: Phase 2c must reread/rehash before approval.
func readInbox(directory, id string, uid uint32, now time.Time) (Record, error) {
	var empty Record
	if !validInboxPath(directory) || !validOpaqueID(id) || uid == 0 || now.IsZero() {
		return empty, ErrUnsafeInbox
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return empty, ErrUnsafeInbox
	}
	defer func() {
		if fd >= 0 {
			syscall.Close(fd)
		}
	}()
	if !trustedDirectory(fd, 0, false) {
		return empty, ErrUnsafeInbox
	}
	components := strings.Split(strings.TrimPrefix(directory, "/"), "/")
	if len(components) == 0 || len(components) > 32 {
		return empty, ErrUnsafeInbox
	}
	for i, name := range components {
		if name == "" || name == "." || name == ".." {
			return empty, ErrUnsafeInbox
		}
		next, openErr := syscall.Openat(fd, name,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if openErr != nil {
			return empty, ErrUnsafeInbox
		}
		syscall.Close(fd)
		fd = next
		owner := uint32(0)
		private := false
		if i == len(components)-1 {
			owner, private = uid, true
		}
		if !trustedDirectory(fd, owner, private) {
			return empty, ErrUnsafeInbox
		}
	}
	// Use a duplicate directory FD for bounded enumeration. The original FD
	// stays open for the selected openat call, independent of filename lookup.
	dup, err := syscall.Openat(fd, ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return empty, ErrUnsafeInbox
	}
	listFile := os.NewFile(uintptr(dup), "proposal-directory-enumeration")
	const maxTotalInboxEntries = 128
	entries, readErr := listFile.ReadDir(maxTotalInboxEntries + 1)
	listFile.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || len(entries) > maxTotalInboxEntries {
		return empty, ErrUnsafeInbox
	}
	pending := 0
	found := false
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			opaque := strings.TrimSuffix(name, ".json")
			if !validOpaqueID(opaque) {
				return empty, ErrUnsafeInbox
			}
			pending++
			if opaque == id {
				found = true
			}
		}
	}
	if !found || pending > MaxPending {
		return empty, ErrUnsafeInbox
	}
	selected, err := syscall.Openat(fd, id+".json",
		syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return empty, ErrUnsafeInbox
	}
	file := os.NewFile(uintptr(selected), "untrusted-proposal")
	defer file.Close()
	before, ok := trustedProposalFile(selected, uid)
	if !ok {
		return empty, ErrUnsafeInbox
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxProposalBytes+1))
	if err != nil || len(data) == 0 || len(data) > MaxProposalBytes || int64(len(data)) != before.Size {
		return empty, ErrUnsafeInbox
	}
	after, valid := trustedProposalFile(selected, uid)
	if !valid || !sameSnapshot(before, after) || !trustedDirectory(fd, uid, true) {
		return empty, ErrUnsafeInbox
	}
	record, err := Parse(data, now)
	if err != nil || record.Proposal.ProposalID != id {
		return empty, ErrUnsafeInbox
	}
	return record, nil
}

func validInboxPath(directory string) bool {
	return filepath.IsAbs(directory) && directory != "/" &&
		len(directory) <= 512 && filepath.Clean(directory) == directory
}

func validOpaqueID(id string) bool {
	// Reuse the strict pattern without leaking caller-controlled paths into
	// host diagnostics. Duplicated predicate is intentionally small.
	if len(id) == 0 || len(id) > 63 || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, ch := range id[1:] {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			return false
		}
	}
	return true
}

func trustedDirectory(fd int, owner uint32, private bool) bool {
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR ||
		st.Uid != owner || st.Mode&0o022 != 0 {
		return false
	}
	return !private || st.Mode&0o077 == 0 && st.Mode&0o700 == 0o700
}

func trustedProposalFile(fd int, owner uint32) (syscall.Stat_t, bool) {
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil ||
		st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Uid != owner ||
		st.Nlink != 1 || st.Mode&0o7777 != 0o600 ||
		st.Size < 1 || st.Size > MaxProposalBytes {
		return st, false
	}
	return st, true
}

func sameSnapshot(a, b syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid &&
		a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size &&
		a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
