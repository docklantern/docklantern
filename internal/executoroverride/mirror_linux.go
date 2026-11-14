package executoroverride

import (
    "crypto/rand"
    "encoding/hex"
    "errors"
    "io"
    "os"
    "path/filepath"
    "strings"
    "syscall"

    "github.com/docklantern/docklantern/internal/executorregistration"
    "golang.org/x/sys/unix"
)

// PublicOverrideFile is only managed for explicitly registered, root-owned,
// single-service projects. Default Compose commands from the project root
// include it. Operators using -f must explicitly list this file last.
const PublicOverrideFile = "compose.override.yaml"

var ErrPersistentMirror = errors.New("trusted persistent Compose override unavailable")

// VerifyCurrent checks the exact on-disk desired generation against the
// already protected journal. A missing, foreign, stale or manipulated file
// blocks further deployment; it is never silently adopted or repaired.
func VerifyCurrent(reg executorregistration.Registration, previousDigest string, hasPrevious bool) error {
    dirfd, err := openRootProject(reg)
    if err != nil { return ErrPersistentMirror }
    defer syscall.Close(dirfd)
    return verifyAt(dirfd, reg, previousDigest, hasPrevious)
}

// PublishVerified atomically publishes a post-VERIFY image-only override.
// The caller must hold the trusted project's exclusive lease and journal
// COMMIT intent, supply the last committed digest, and durably COMMIT only
// AFTER this function returns. Filesystem and SQLite transactions are NOT
// atomic together: a crash between them requires manual reconciliation.
func PublishVerified(reg executorregistration.Registration, previousDigest string,
    hasPrevious bool, candidateDigest string) error {
    if os.Geteuid() != 0 || !reg.PersistentOverride {
        return ErrPersistentMirror
    }
    service := reg.Services[0]
    data, err := Render(reg, service.Name, service.ApprovedRepository, candidateDigest)
    if err != nil { return ErrPersistentMirror }
    dirfd, err := openRootProject(reg)
    if err != nil { return ErrPersistentMirror }
    defer syscall.Close(dirfd)
    if verifyAt(dirfd, reg, previousDigest, hasPrevious) != nil {
        return ErrPersistentMirror
    }
    nonce := make([]byte, 16)
    if _, err = rand.Read(nonce); err != nil { return ErrPersistentMirror }
    tmp := ".docklantern-"+hex.EncodeToString(nonce)+".tmp"
    fd, err := syscall.Openat(dirfd, tmp, syscall.O_WRONLY|syscall.O_CREAT|
        syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
    if err != nil { return ErrPersistentMirror }
    defer syscall.Unlinkat(dirfd, tmp)
    file := os.NewFile(uintptr(fd), "staged-image-only-override")
    n, writeErr := file.Write(data)
    if writeErr != nil || n != len(data) || syscall.Fchmod(fd, 0o644) != nil ||
        file.Sync() != nil {
        _ = file.Close()
        return ErrPersistentMirror
    }
    if file.Close() != nil { return ErrPersistentMirror }
    // A second verification detects external modification before replacement.
    if verifyAt(dirfd, reg, previousDigest, hasPrevious) != nil {
        return ErrPersistentMirror
    }
    // Never overwrite an unknown first-generation override. Subsequent
    // replacement is permitted only after verification of exact prior bytes.
    flags := uint(0)
    if !hasPrevious { flags = unix.RENAME_NOREPLACE }
    if err = unix.Renameat2(dirfd, tmp, dirfd, PublicOverrideFile, flags); err != nil {
        return ErrPersistentMirror
    }
    // Once renamed, failures must leave evidence in place; never unlink a
    // possibly active final override or claim a successful journal COMMIT.
    if syscall.Fsync(dirfd) != nil || verifyAt(dirfd, reg, candidateDigest, true) != nil {
        return ErrPersistentMirror
    }
    return nil
}

func verifyAt(dirfd int, reg executorregistration.Registration, digest string, present bool) error {
    if !present {
        var st unix.Stat_t
        err := unix.Fstatat(dirfd, PublicOverrideFile, &st, unix.AT_SYMLINK_NOFOLLOW)
        if errors.Is(err, syscall.ENOENT) { return nil }
        return ErrPersistentMirror
    }
    if !validDigest(digest) { return ErrPersistentMirror }
    opened, err := syscall.Openat(dirfd, PublicOverrideFile,
        syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
    if err != nil { return ErrPersistentMirror }
    file := os.NewFile(uintptr(opened), "committed-image-only-override")
    defer file.Close()
    var before, after syscall.Stat_t
    if syscall.Fstat(opened, &before) != nil || before.Uid != 0 ||
        before.Mode&syscall.S_IFMT != syscall.S_IFREG ||
        before.Mode&0o7777 != 0o644 || before.Nlink != 1 ||
        before.Size <= 0 || before.Size > 1024 {
        return ErrPersistentMirror
    }
    data, err := io.ReadAll(io.LimitReader(file, 1025))
    if err != nil || int64(len(data)) != before.Size ||
        syscall.Fstat(opened, &after) != nil ||
        before.Dev != after.Dev || before.Ino != after.Ino ||
        before.Size != after.Size || before.Mtim != after.Mtim ||
        before.Ctim != after.Ctim {
        return ErrPersistentMirror
    }
    service := reg.Services[0]
    return VerifyExact(data, reg, service.Name, service.ApprovedRepository, digest)
}

// openRootProject protects every ancestor and the leaf from symlink traversal
// and untrusted writes. A non-root source owner is deliberately not supported
// for persistent management, even if beta.6's manual mode trusts that UID.
func openRootProject(reg executorregistration.Registration) (int, error) {
    if os.Geteuid() != 0 || !reg.PersistentOverride ||
        DefaultFileContract(reg) != nil || reg.TrustedSourceUID == nil ||
        *reg.TrustedSourceUID != 0 || !filepath.IsAbs(reg.ProjectDirectory) ||
        filepath.Clean(reg.ProjectDirectory) != reg.ProjectDirectory ||
        len(reg.ProjectDirectory) > 512 {
        return -1, ErrPersistentMirror
    }
    parts := strings.Split(strings.TrimPrefix(reg.ProjectDirectory, "/"), "/")
    if len(parts) < 2 || len(parts) > 32 { return -1, ErrPersistentMirror }
    fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
    if err != nil { return -1, ErrPersistentMirror }
    good := false
    defer func() { if !good { _ = syscall.Close(fd) } }()
    if !rootOwnedDirectory(fd) { return -1, ErrPersistentMirror }
    for _, part := range parts {
        if part == "" || part == "." || part == ".." { return -1, ErrPersistentMirror }
        next, e := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|
            syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
        if e != nil { return -1, ErrPersistentMirror }
        _ = syscall.Close(fd)
        fd = next
        if !rootOwnedDirectory(fd) { return -1, ErrPersistentMirror }
    }
    good = true
    return fd, nil
}

func rootOwnedDirectory(fd int) bool {
    var st syscall.Stat_t
    return syscall.Fstat(fd, &st) == nil &&
        st.Mode&syscall.S_IFMT == syscall.S_IFDIR && st.Uid == 0 &&
        st.Mode&0o022 == 0
}
