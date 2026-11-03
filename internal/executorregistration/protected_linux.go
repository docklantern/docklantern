package executorregistration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/docklantern/docklantern/internal/executorhost"
)

const ProtectedRegistrationDirectory = "/var/lib/docklantern/executor/registrations"

var ErrUntrustedStore = errors.New("untrusted executor registration store")

// Record binds the validated, executor-owned registration to its exact
// protected JSON bytes. This hash is evidence only, not permission to deploy.
type Record struct {
	Registration Registration
	SHA256       string
}

// LoadProtected is the only production registration reader. It never accepts
// a source path or writable directory from monitoring/proposal input. The
// executor must independently validate actual Docker/Compose/OCI state later.
func LoadProtected(id string) (Record, error) {
	return readProtected(ProtectedRegistrationDirectory, id)
}

// readProtected uses directory-relative no-follow opens and verifies the
// owner/mode/type of every directory and the opened file. This protects the
// registration file against path substitution by an unprivileged monitor.
// The host root/operator, still trusted under ADR 0008, can change records.
func readProtected(directory, id string) (Record, error) {
	var result Record
	if !ValidID(id) || directory == "" || directory[0] != '/' {
		return result, ErrUntrustedStore
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return result, ErrUntrustedStore
	}
	defer func() { syscall.Close(fd) }()
	if !trustedStat(fd, true, false) {
		return result, ErrUntrustedStore
	}
	parts := strings.Split(strings.TrimPrefix(directory, "/"), "/")
	if len(parts) == 0 || len(parts) > 32 {
		return result, ErrUntrustedStore
	}
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return result, ErrUntrustedStore
		}
		next, openErr := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if openErr != nil {
			return result, ErrUntrustedStore
		}
		syscall.Close(fd)
		fd = next
		if !trustedStat(fd, true, index == len(parts)-1) {
			return result, ErrUntrustedStore
		}
	}
	opened, err := syscall.Openat(fd, id+".json", syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return result, ErrUntrustedStore
	}
	file := os.NewFile(uintptr(opened), "protected-executor-registration")
	defer file.Close()
	if !trustedStat(opened, false, false) {
		return result, ErrUntrustedStore
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxRegistrationBytes+1))
	if err != nil || len(data) == 0 || len(data) > MaxRegistrationBytes {
		return result, ErrUntrustedStore
	}
	registration, err := Parse(data)
	if err != nil || registration.RegistrationID != id {
		return result, ErrUntrustedStore
	}
	// A protected record cannot be accepted until each operator-owned Compose
	// source path is also preflight-validated. No source is opened for writing.
	if err := registration.ValidateTrustedSources(executorhost.RealFS{}); err != nil {
		return result, ErrUntrustedStore
	}
	hash := sha256.Sum256(data)
	return Record{Registration: registration, SHA256: "sha256:" + hex.EncodeToString(hash[:])}, nil
}

func trustedStat(fd int, directory, private bool) bool {
	var stat syscall.Stat_t
	if syscall.Fstat(fd, &stat) != nil || stat.Uid != 0 || stat.Mode&0o022 != 0 {
		return false
	}
	if directory {
		return stat.Mode&syscall.S_IFMT == syscall.S_IFDIR && (!private || stat.Mode&0o077 == 0)
	}
	return stat.Mode&syscall.S_IFMT == syscall.S_IFREG && stat.Nlink == 1 && stat.Mode&0o177 == 0
}
