package executorregistration

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docklantern/docklantern/internal/executorhost"
)

// PublishProtected is a host-root + controlling-TTY-only operation. It
// creates/updates a REGISTRATION FILE, never a Docker image, container,
// Compose source, journal, or override. Only the host-root register CLI and trusted host code may invoke it; never web.
func PublishProtected(reg Registration, expectedOldHash string) (Record, error) {
	return publishProtectedWithVerifier(reg, expectedOldHash, nil)
}

// publishProtectedWithVerifier binds separately loaded root-owned input bytes
// to the TTY confirmation when called from RegisterFromFile. It is intentionally
// internal: callers cannot omit the trusted file recheck via CLI flags.
func publishProtectedWithVerifier(reg Registration, expectedOldHash string, verifyInput func() error) (Record, error) {
	var empty Record
	if os.Geteuid() != 0 || reg.Validate() != nil ||
		reg.ValidateTrustedSources(executorhost.RealFS{}) != nil {
		return empty, ErrUntrustedStore
	}
	data, err := json.Marshal(reg)
	if err != nil || len(data) == 0 || len(data) > MaxRegistrationBytes {
		return empty, ErrUntrustedStore
	}
	hash := sha256.Sum256(data)
	token := strings.ToUpper(hex.EncodeToString(hash[:6]))
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil { return empty, ErrUntrustedStore }
	defer tty.Close()
	if info, e := tty.Stat(); e != nil || info.Mode()&os.ModeCharDevice == 0 {
		return empty, ErrUntrustedStore
	}
	if _, err := fmt.Fprintf(tty,
		"HOST-LOCAL REGISTRATION CHANGE (NO DOCKER UPDATE)\n"+
			"Registration: %s\nProject: %s\nProject directory: %s\n"+
			"Ordered Compose sources: %v\nEnvironment sources: %v\nProfiles: %v\n"+
			"Existing registration SHA-256 (empty means create only): %s\n",
		reg.RegistrationID, reg.ProjectName, reg.ProjectDirectory,
		reg.ComposeFiles, reg.EnvFiles, reg.Profiles, expectedOldHash); err != nil {
		return empty, ErrUntrustedStore
	}
	for _, service := range reg.Services {
		if _, err := fmt.Fprintf(tty,
			"Service: %s | enabled=%t | repository=%s | platform=%s/%s | running-only=%t\n",
			service.Name, service.DeploymentEnabled, service.ApprovedRepository,
			service.Platform.OS, service.Platform.Architecture, service.AllowRunningOnly); err != nil {
			return empty, ErrUntrustedStore
		}
	}
	if _, err := fmt.Fprintf(tty,
		"New registration SHA-256: sha256:%x\n"+
			"Type REGISTER %s %s to confirm this exact registration change:\n",
		hash, reg.RegistrationID, token); err != nil { return empty, ErrUntrustedStore }
	reply, err := bufio.NewReader(io.LimitReader(tty, 128)).ReadString('\n')
	if err != nil || strings.TrimSuffix(strings.TrimSuffix(reply, "\n"), "\r") !=
		"REGISTER "+reg.RegistrationID+" "+token {
		return empty, ErrUntrustedStore
	}
	// Do not trust a long-lived registration/source approval; recheck after
	// the operator finishes typing.
	if reg.ValidateTrustedSources(executorhost.RealFS{}) != nil { return empty, ErrUntrustedStore }
	// Ensure the typed confirmation still covers the exact bytes being saved.
	confirmed, err := json.Marshal(reg)
	if err != nil || sha256.Sum256(confirmed) != hash { return empty, ErrUntrustedStore }
	if verifyInput != nil && verifyInput() != nil { return empty, ErrUntrustedStore }
	return publishProtected(ProtectedRegistrationDirectory, reg, expectedOldHash)
}

// publishProtected is intentionally unexported so only the guarded host-root
// entry can publish outside CI-only package tests. Parent dirs must already
// exist and be root-owned/private; no permission or directory provisioning.
func publishProtected(directory string, reg Registration, expectedOldHash string) (Record, error) {
	var empty Record
	if os.Geteuid() != 0 || reg.Validate() != nil ||
		!filepath.IsAbs(directory) || directory == "/" || filepath.Clean(directory) != directory ||
		reg.ValidateTrustedSources(executorhost.RealFS{}) != nil {
		return empty, ErrUntrustedStore
	}
	data, err := json.Marshal(reg)
	if err != nil || len(data) == 0 || len(data) > MaxRegistrationBytes {
		return empty, ErrUntrustedStore
	}
	if _, err := Parse(data); err != nil { return empty, ErrUntrustedStore }
	dirfd, err := openProtectedStoreDirectory(directory)
	if err != nil { return empty, ErrUntrustedStore }
	defer syscall.Close(dirfd)
	if err := syscall.Flock(dirfd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return empty, ErrUntrustedStore
	}
	defer syscall.Flock(dirfd, syscall.LOCK_UN)

	filename := reg.RegistrationID + ".json"
	existing, err := syscall.Openat(dirfd, filename,
		syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err == nil {
		syscall.Close(existing)
		if !validSHA256(expectedOldHash) {
			return empty, ErrUntrustedStore
		}
		old, err := readProtected(directory, reg.RegistrationID)
		if err != nil || old.SHA256 != expectedOldHash {
			return empty, ErrUntrustedStore
		}
	} else if !errors.Is(err, syscall.ENOENT) || expectedOldHash != "" {
		return empty, ErrUntrustedStore
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil { return empty, ErrUntrustedStore }
	temp := ".registration-" + hex.EncodeToString(nonce) + ".tmp"
	tmpfd, err := syscall.Openat(dirfd, temp,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil { return empty, ErrUntrustedStore }
	defer syscall.Unlinkat(dirfd, temp)
	file := os.NewFile(uintptr(tmpfd), "protected-registration-temp")
	if _, err := file.Write(data); err != nil {
		file.Close()
		return empty, ErrUntrustedStore
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return empty, ErrUntrustedStore
	}
	if err := file.Close(); err != nil { return empty, ErrUntrustedStore }
	if reg.ValidateTrustedSources(executorhost.RealFS{}) != nil {
		return empty, ErrUntrustedStore
	}
	if err := syscall.Renameat(dirfd, temp, dirfd, filename); err != nil {
		return empty, ErrUntrustedStore
	}
	if err := syscall.Fsync(dirfd); err != nil {
		return empty, ErrUntrustedStore
	}
	record, err := readProtected(directory, reg.RegistrationID)
	if err != nil { return empty, ErrUntrustedStore }
	return record, nil
}

func validSHA256(s string) bool {
	if !strings.HasPrefix(s, "sha256:") || len(s) != 71 { return false }
	for _, b := range s[7:] {
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') { return false }
	}
	return true
}

func openProtectedStoreDirectory(directory string) (int, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory ||
		directory == "/" { return -1, ErrUntrustedStore }
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil { return -1, ErrUntrustedStore }
	if !trustedStat(fd, true, false) { syscall.Close(fd); return -1, ErrUntrustedStore }
	parts := strings.Split(strings.TrimPrefix(directory, "/"), "/")
	if len(parts) == 0 || len(parts) > 32 { syscall.Close(fd); return -1, ErrUntrustedStore }
	for i, p := range parts {
		if p == "" || p == "." || p == ".." { syscall.Close(fd); return -1, ErrUntrustedStore }
		next, err := syscall.Openat(fd, p,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		syscall.Close(fd)
		if err != nil { return -1, ErrUntrustedStore }
		fd = next
		if !trustedStat(fd, true, i == len(parts)-1) {
			syscall.Close(fd)
			return -1, ErrUntrustedStore
		}
	}
	return fd, nil
}
