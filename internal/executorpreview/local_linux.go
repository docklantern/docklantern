package executorpreview

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docklantern/docklantern/internal/executorproposal"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

const MonitorUIDFile = "/var/lib/docklantern/executor/monitor.uid"

// PreviewLocal is the only production preview action. It requires a real
// host-root user and controlling terminal. No monitoring/web process may
// supply the monitor UID or a filesystem path. All files are read-only.
func PreviewLocal(id string) (View, error) {
	if os.Geteuid() != 0 {
		return View{}, ErrPreviewDenied
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return View{}, ErrPreviewDenied
	}
	info, err := tty.Stat()
	_ = tty.Close()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return View{}, ErrPreviewDenied
	}
	uid, err := LoadMonitorUID()
	if err != nil {
		return View{}, ErrPreviewDenied
	}
	return Prepare(context.Background(), id, ProtectedSource{MonitorUID: uid}, time.Now)
}

type ProtectedSource struct {
	MonitorUID uint32
}

func (s ProtectedSource) LoadProposal(id string, now time.Time) (executorproposal.Record, error) {
	if s.MonitorUID == 0 {
		return executorproposal.Record{}, ErrPreviewDenied
	}
	return executorproposal.LoadInbox(id, s.MonitorUID, now)
}

func (s ProtectedSource) LoadRegistration(id string) (executorregistration.Record, error) {
	return executorregistration.LoadProtected(id)
}

var _ Source = ProtectedSource{}

// LoadMonitorUID reads the separately configured root-owned identity of the
// nonroot monitoring process. This file is NOT created by the executor and
// its value can never originate in a proposal, Docker label, GUI or CLI flag.
func LoadMonitorUID() (uint32, error) {
	return loadMonitorUIDFrom(MonitorUIDFile)
}

func loadMonitorUIDFrom(path string) (uint32, error) {
	if !filepath.IsAbs(path) || path == "/" || filepath.Clean(path) != path {
		return 0, ErrPreviewDenied
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 32 {
		return 0, ErrPreviewDenied
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return 0, ErrPreviewDenied
	}
	defer func() {
		if fd >= 0 {
			_ = syscall.Close(fd)
		}
	}()
	if !rootDirectory(fd, false) {
		return 0, ErrPreviewDenied
	}
	for i, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return 0, ErrPreviewDenied
		}
		next, err := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|
			syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			return 0, ErrPreviewDenied
		}
		_ = syscall.Close(fd)
		fd = next
		if !rootDirectory(fd, i == len(parts)-2) {
			return 0, ErrPreviewDenied
		}
	}
	filename := parts[len(parts)-1]
	if filename == "" || filename == "." || filename == ".." {
		return 0, ErrPreviewDenied
	}
	opened, err := syscall.Openat(fd, filename, syscall.O_RDONLY|syscall.O_NOFOLLOW|
		syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return 0, ErrPreviewDenied
	}
	file := os.NewFile(uintptr(opened), "protected-monitor-uid")
	defer file.Close()
	before, ok := rootUIDFile(opened)
	if !ok {
		return 0, ErrPreviewDenied
	}
	data, err := io.ReadAll(io.LimitReader(file, 12))
	if err != nil || len(data) == 0 || len(data) > 11 || int64(len(data)) != before.Size {
		return 0, ErrPreviewDenied
	}
	after, ok := rootUIDFile(opened)
	if !ok || !sameUIDFile(before, after) || !rootDirectory(fd, true) {
		return 0, ErrPreviewDenied
	}
	value := strings.TrimSuffix(string(data), "\n")
	if len(value) == 0 || len(value) > 10 || value[0] == '0' {
		return 0, ErrPreviewDenied
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, ErrPreviewDenied
		}
	}
	number, err := strconv.ParseUint(value, 10, 32)
	if err != nil || number == 0 {
		return 0, ErrPreviewDenied
	}
	return uint32(number), nil
}

func rootDirectory(fd int, private bool) bool {
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR ||
		st.Uid != 0 || st.Mode&0o022 != 0 {
		return false
	}
	return !private || st.Mode&0o077 == 0 && st.Mode&0o700 == 0o700
}

func rootUIDFile(fd int) (syscall.Stat_t, bool) {
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Uid != 0 ||
		st.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		st.Mode&0o7777 != 0o600 || st.Nlink != 1 ||
		st.Size < 1 || st.Size > 11 {
		return st, false
	}
	return st, true
}

func sameUIDFile(a, b syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid &&
		a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size &&
		a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

