// Package executorhost defines read-only security checks for the planned
// host-native executor. Nothing here invokes Docker, launches processes,
// creates files, or grants deployment authority.
package executorhost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var ErrUntrusted = errors.New("untrusted host filesystem path")

// Entry describes a single Lstat result. Entry does not establish the
// stability of a pathname across multiple lookups.
type Entry struct {
	Mode fs.FileMode
	UID  uint32
}

// Reader can be replaced by an in-memory fixture in tests. RealFS uses
// Lstat, not Stat, so symlinks cannot silently escape the checked path.
type Reader interface {
	Lstat(path string) (Entry, error)
}

type RealFS struct{}

func (RealFS) Lstat(path string) (Entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Entry{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Entry{}, fmt.Errorf("%w: missing Unix ownership metadata", ErrUntrusted)
	}
	return Entry{Mode: info.Mode(), UID: stat.Uid}, nil
}

type leafKind int

const (
	directory leafKind = iota
	regularFile
)

// validate walks all path components from / using Lstat. For the last
// component, ownership must be explicitly trusted; intermediary directories
// must be owned by one of ancestorOwners. No component may be a symlink,
// group-writable, or world-writable. This is a preflight check, NOT a
// TOCTOU-safe handle or authorization to invoke Compose on the pathname.
func validate(reader Reader, path string, ancestorOwners, leafOwners []uint32, leaf leafKind, executable bool) error {
	if !filepath.IsAbs(path) || path == "/" || filepath.Clean(path) != path {
		return fmt.Errorf("%w: path must be absolute and canonical", ErrUntrusted)
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	current := "/"
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("%w: invalid path component", ErrUntrusted)
		}
		current = filepath.Join(current, part)
		entry, err := reader.Lstat(current)
		if err != nil {
			return fmt.Errorf("%w: cannot inspect path component: %v", ErrUntrusted, err)
		}
		if entry.Mode&fs.ModeSymlink != 0 || entry.Mode.Perm()&0o022 != 0 {
			return fmt.Errorf("%w: symlink or writable component", ErrUntrusted)
		}
		last := i == len(parts)-1
		if !last {
			if !entry.Mode.IsDir() || !containsOwner(ancestorOwners, entry.UID) {
				return fmt.Errorf("%w: unsafe ancestor", ErrUntrusted)
			}
			continue
		}
		if !containsOwner(leafOwners, entry.UID) {
			return fmt.Errorf("%w: unexpected final owner", ErrUntrusted)
		}
		switch leaf {
		case directory:
			if !entry.Mode.IsDir() {
				return fmt.Errorf("%w: expected directory", ErrUntrusted)
			}
		case regularFile:
			if !entry.Mode.IsRegular() {
				return fmt.Errorf("%w: expected regular file", ErrUntrusted)
			}
		}
		if executable && (entry.Mode.Perm()&0o111 == 0 || entry.Mode&(fs.ModeSetuid|fs.ModeSetgid) != 0) {
			return fmt.Errorf("%w: unsafe executable permissions", ErrUntrusted)
		}
	}
	return nil
}

func containsOwner(owners []uint32, uid uint32) bool {
	for _, allowed := range owners {
		if uid == allowed {
			return true
		}
	}
	return false
}

// ValidateTrustedExecutable rejects binaries that are not root-owned regular
// executables under root-owned, non-writable, nonsymlink ancestors. It does
// not execute the binary, validate signatures, inspect plugin behavior, or
// pin an inode for subsequent use. Those belong to later integration gates.
func ValidateTrustedExecutable(reader Reader, path string) error {
	return validate(reader, path, []uint32{0}, []uint32{0}, regularFile, true)
}

// ValidatePrivateDirectory validates a pre-existing, root-owned 0700
// directory. Diagnostics never create or change trusted host directories.
// This remains a pathname check and does not guarantee inode stability.
func ValidatePrivateDirectory(reader Reader, path string) error {
	if err := validate(reader, path, []uint32{0}, []uint32{0}, directory, false); err != nil {
		return err
	}
	entry, err := reader.Lstat(path)
	if err != nil || entry.Mode.Perm()&0o077 != 0 {
		return fmt.Errorf("%w: directory must be private", ErrUntrusted)
	}
	return nil
}

// ValidateTrustedSource checks an operator-approved source path without
// following symlinks or trusting group-writable ancestors. It is a policy
// preflight only: the executor must still implement stable file descriptors
// or trusted snapshots and revalidate just before any future Docker action.
func ValidateTrustedSource(reader Reader, path string, trustedUID uint32) error {
	return validate(reader, path, []uint32{0, trustedUID}, []uint32{0, trustedUID}, regularFile, false)
}

// Layout uses an executor-private root. Only the proposal directory may
// belong to the unprivileged monitor, with no shared writable ancestor.
// The caller must ensure the monitor does not mount the private/receipt
// directory read-write; filesystem mode bits do NOT prove mount isolation.
type Layout struct {
	Root       string
	MonitorUID uint32
}

func (l Layout) Validate(reader Reader) error {
	if l.MonitorUID == 0 {
		return fmt.Errorf("%w: monitor must have distinct nonroot UID", ErrUntrusted)
	}
	if err := validate(reader, l.Root, []uint32{0}, []uint32{0}, directory, false); err != nil {
		return err
	}
	for _, name := range []string{"handoff", "handoff/receipts", "executor", "executor/registrations", "executor/journal", "executor/locks", "executor/overrides"} {
		p := filepath.Join(l.Root, name)
		if err := validate(reader, p, []uint32{0}, []uint32{0}, directory, false); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if name == "executor" || strings.HasPrefix(name, "executor/") {
			entry, err := reader.Lstat(p)
			if err != nil || entry.Mode.Perm()&0o077 != 0 {
				return fmt.Errorf("%w: executor private directory is accessible to non-owners", ErrUntrusted)
			}
		}
	}
	return validate(reader, filepath.Join(l.Root, "handoff/proposals"), []uint32{0}, []uint32{l.MonitorUID}, directory, false)
}

// MinimalChildEnvironment deliberately ignores inherited caller environment
// variables. Later subprocess code must always use this exact environment
// and fixed absolute executable paths instead of exec.LookPath or shelling out.
// All strings are owned by this package; proposals cannot modify the socket,
// context, Docker credential store, PATH, or Compose settings.
func MinimalChildEnvironment(_ []string) []string {
	return []string{
		"PATH=/usr/bin:/bin",
		"HOME=/var/lib/docklantern/executor",
		"DOCKER_CONFIG=/var/lib/docklantern/executor/docker-config",
		"DOCKER_HOST=unix:///var/run/docker.sock",
		"LANG=C",
		"LC_ALL=C",
	}
}
