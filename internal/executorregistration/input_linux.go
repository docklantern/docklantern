package executorregistration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var ErrUntrustedRegistrationInput = errors.New("untrusted host registration input")

const MaxInputPathBytes = 512

// RegistrationInput is a read-only snapshot of exact bytes from a trusted
// root-owned input file. The input itself is NOT a protected registration.
type RegistrationInput struct {
	Registration Registration
	SHA256       string
}

// RegisterFromFile is the host-only CLI action. It refuses nonroot callers
// before accessing any file. The exported protected publisher separately
// requires a controlling terminal and explicit per-record confirmation.
// Neither this function nor the publisher reads or writes Docker state.
func RegisterFromFile(path, previousSHA256 string) (Record, error) {
	if os.Geteuid() != 0 {
		return Record{}, ErrUntrustedRegistrationInput
	}
	input, err := readTrustedRegistrationInput(path)
	if err != nil {
		return Record{}, ErrUntrustedRegistrationInput
	}
	// Reopen the original root-controlled input after the operator types
	// the TTY challenge. Re-check the exact bytes, not parsed JSON equality.
	// This is not a stable source snapshot against a hostile root operator.
	recheck := func() error {
		current, err := readTrustedRegistrationInput(path)
		if err != nil || current.SHA256 != input.SHA256 {
			return ErrUntrustedRegistrationInput
		}
		return nil
	}
	return publishProtectedWithVerifier(input.Registration, previousSHA256, recheck)
}

// readTrustedRegistrationInput uses descriptor-relative opens and fstat.
// Ancestors and the file must be root-owned; directories must not be group-
// or world-writable; the singly linked file must have exactly 0600 mode.
// A GUI/monitor-owned input can never pass these ownership checks.
func readTrustedRegistrationInput(path string) (RegistrationInput, error) {
	var empty RegistrationInput
	if !trustedInputPath(path) {
		return empty, ErrUntrustedRegistrationInput
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 32 {
		return empty, ErrUntrustedRegistrationInput
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return empty, ErrUntrustedRegistrationInput
	}
	defer func() {
		if fd >= 0 {
			_ = syscall.Close(fd)
		}
	}()
	if !trustedStat(fd, true, false) {
		return empty, ErrUntrustedRegistrationInput
	}
	for _, part := range parts[:len(parts)-1] {
		next, openErr := syscall.Openat(fd, part,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if openErr != nil {
			return empty, ErrUntrustedRegistrationInput
		}
		_ = syscall.Close(fd)
		fd = next
		if !trustedStat(fd, true, false) {
			return empty, ErrUntrustedRegistrationInput
		}
	}
	opened, err := syscall.Openat(fd, parts[len(parts)-1],
		syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return empty, ErrUntrustedRegistrationInput
	}
	file := os.NewFile(uintptr(opened), "trusted-registration-input")
	defer file.Close()

	before, ok := trustedInputStat(opened)
	if !ok {
		return empty, ErrUntrustedRegistrationInput
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxRegistrationBytes+1))
	if err != nil || len(data) == 0 || len(data) > MaxRegistrationBytes ||
		int64(len(data)) != before.Size {
		return empty, ErrUntrustedRegistrationInput
	}
	after, ok := trustedInputStat(opened)
	if !ok || !sameInputStat(before, after) {
		return empty, ErrUntrustedRegistrationInput
	}
	reg, err := Parse(data)
	if err != nil {
		return empty, ErrUntrustedRegistrationInput
	}
	hash := sha256.Sum256(data)
	return RegistrationInput{
		Registration: reg,
		SHA256:       "sha256:" + hex.EncodeToString(hash[:]),
	}, nil
}

// An operator-chosen path must be absolute, already canonical, and never a
// dash, /dev/stdin, a proc-fd alias or a relative directory traversal.
func trustedInputPath(path string) bool {
	if len(path) == 0 || len(path) > MaxInputPathBytes || !filepath.IsAbs(path) ||
		path == "/" || filepath.Clean(path) != path || strings.IndexByte(path, 0) != -1 {
		return false
	}
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(components) < 2 || len(components) > 32 {
		return false
	}
	for _, part := range components {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func trustedInputStat(fd int) (syscall.Stat_t, bool) {
	var st syscall.Stat_t
	err := syscall.Fstat(fd, &st)
	if err != nil || st.Uid != 0 || st.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		st.Mode&0o7777 != 0o600 || st.Nlink != 1 ||
		st.Size < 1 || st.Size > MaxRegistrationBytes {
		return st, false
	}
	return st, true
}

func sameInputStat(a, b syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid &&
		a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size &&
		a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
