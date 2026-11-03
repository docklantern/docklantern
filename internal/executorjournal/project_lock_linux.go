package executorjournal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/docklantern/docklantern/internal/executorregistration"
)

const HostProjectLocksDirectory = "/var/lib/docklantern/executor/locks"

var (
	ErrLockUnavailable = errors.New("protected executor project lock unavailable")
	ErrProjectLocked   = errors.New("executor project lock is held")
)

// ProjectLock is a nontransferable, host-local advisory lock. The kernel
// releases it on close or process termination, including SIGKILL. A lock file
// is NEVER deleted on release: unlinking it could produce two lock inodes.
// It does not constrain an external Docker CLI or unauthorized root process.
type ProjectLock struct {
	mu sync.Mutex
	fd int
	projectID string
	released bool
}

// AcquireProjectLock is root-only and always uses the executor-private fixed
// locks directory. It never creates an ancestor or takes a CLI-supplied path.
// There is no Docker operation or monitoring IPC in this package.
func AcquireProjectLock(projectID string) (*ProjectLock, error) {
	return acquireProjectLockAt(HostProjectLocksDirectory, projectID)
}

func acquireProjectLockAt(directory, projectID string) (*ProjectLock, error) {
	if os.Geteuid() != 0 || !executorregistration.ValidID(projectID) ||
		!filepath.IsAbs(directory) || directory == "/" ||
		len(directory) > 512 || filepath.Clean(directory) != directory {
		return nil, ErrLockUnavailable
	}
	parts := strings.Split(strings.TrimPrefix(directory, "/"), "/")
	if len(parts) < 2 || len(parts) > 32 { return nil, ErrLockUnavailable }
	dirfd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil { return nil, ErrLockUnavailable }
	defer func() { _ = syscall.Close(dirfd) }()
	if !trustedJournalDir(dirfd, false) { return nil, ErrLockUnavailable }
	for i, component := range parts {
		if component == "" || component == "." || component == ".." { return nil, ErrLockUnavailable }
		next, err := syscall.Openat(dirfd, component,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil { return nil, ErrLockUnavailable }
		_ = syscall.Close(dirfd)
		dirfd = next
		if !trustedJournalDir(dirfd, i == len(parts)-1) { return nil, ErrLockUnavailable }
	}
	name := projectID + ".lock"
	created := false
	fd, err := syscall.Openat(dirfd, name,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err == nil {
		created = true
	} else if errors.Is(err, syscall.EEXIST) {
		fd, err = syscall.Openat(dirfd, name, syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	}
	if err != nil { return nil, ErrLockUnavailable }
	keep := false
	defer func() { if !keep { _ = syscall.Close(fd) } }()
	if !trustedJournalFile(fd) { return nil, ErrLockUnavailable }
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrProjectLocked
		}
		return nil, ErrLockUnavailable
	}
	// Check the lock file name still resolves to the opened inode after the
	// lock is taken. The private root-only directory excludes monitor races.
	current, err := syscall.Openat(dirfd, name,
		syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil { return nil, ErrLockUnavailable }
	var first, second syscall.Stat_t
	err1 := syscall.Fstat(fd, &first)
	err2 := syscall.Fstat(current, &second)
	ok := trustedJournalFile(current)
	_ = syscall.Close(current)
	if err1 != nil || err2 != nil || !ok ||
		first.Dev != second.Dev || first.Ino != second.Ino ||
		!trustedJournalDir(dirfd, true) { return nil, ErrLockUnavailable }
	if created && (syscall.Fsync(fd) != nil || syscall.Fsync(dirfd) != nil) {
		return nil, ErrLockUnavailable
	}
	keep = true
	return &ProjectLock{fd:fd,projectID:projectID}, nil
}

// Release releases exactly one acquired file descriptor. There is no
// delete/rename of the persistent lock file and no success receipt.
func (l *ProjectLock) Release() error {
	if l == nil { return ErrLockUnavailable }
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released { return ErrLockUnavailable }
	l.released = true
	unlockErr := syscall.Flock(l.fd, syscall.LOCK_UN)
	closeErr := syscall.Close(l.fd)
	l.fd = -1
	if unlockErr != nil || closeErr != nil { return ErrLockUnavailable }
	return nil
}
