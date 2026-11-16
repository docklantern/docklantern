package executoroverride

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestPersistentMirrorRootFixture(t *testing.T) {
    if os.Geteuid() != 0 { t.Skip("root-owned source fixture") }
    root, err := os.MkdirTemp("/root", "docklantern-override-")
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = os.RemoveAll(root) })
    reg := fixture()
    reg.TrustedSourceUID = func() *uint32 { uid := uint32(0); return &uid }()
    reg.ProjectDirectory = root
    reg.ComposeFiles = []string{filepath.Join(root, "compose.yaml")}
    reg.EnvFiles = nil
    reg.PersistentOverride = true
    if err := os.WriteFile(reg.ComposeFiles[0], []byte("services:\n  web:\n    image: docker.io/example/hello-go:1.0.0\n"), 0o600); err != nil {
        t.Fatal(err)
    }
    if err := reg.Validate(); err != nil { t.Fatal(err) }
    a := "sha256:" + strings.Repeat("a", 64)
    b := "sha256:" + strings.Repeat("b", 64)
    if err := VerifyCurrent(reg, "", false); err != nil { t.Fatal(err) }
    if err := PublishVerified(reg, "", false, a); err != nil { t.Fatal(err) }
    if err := VerifyCurrent(reg, a, true); err != nil { t.Fatal(err) }
    if err := PublishVerified(reg, a, true, b); err != nil { t.Fatal(err) }
    if err := VerifyCurrent(reg, b, true); err != nil { t.Fatal(err) }
    if err := VerifyCurrent(reg, a, true); err == nil { t.Fatal("accepted stale desired state") }
    if err := PublishVerified(reg, a, true, a); err == nil { t.Fatal("overwrote journal-divergent override") }
    data, err := os.ReadFile(filepath.Join(root, PublicOverrideFile))
    if err != nil { t.Fatal(err) }
    if err := VerifyExact(data, reg, "web", "docker.io/example/hello-go", b); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(root, PublicOverrideFile), []byte("unsafe"), 0o644); err != nil { t.Fatal(err) }
    if err := VerifyCurrent(reg, b, true); err == nil { t.Fatal("accepted tampered file") }
}

func TestPersistentMirrorBlocksPreexistingAndSymlinkedFiles(t *testing.T) {
    if os.Geteuid() != 0 { t.Skip("root-owned source fixture") }
    root, err := os.MkdirTemp("/root", "docklantern-override-")
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = os.RemoveAll(root) })
    reg := fixture()
    uid := uint32(0)
    reg.TrustedSourceUID = &uid
    reg.ProjectDirectory = root
    reg.ComposeFiles = []string{filepath.Join(root, "compose.yaml")}
    reg.EnvFiles = nil
    reg.PersistentOverride = true
    if err := os.Symlink("/etc/passwd", filepath.Join(root, PublicOverrideFile)); err != nil { t.Fatal(err) }
    if err := VerifyCurrent(reg, "", false); err == nil { t.Fatal("accepted foreign override") }
    if err := PublishVerified(reg, "", false, "sha256:"+strings.Repeat("a", 64)); err == nil {
        t.Fatal("overwrote symlink")
    }
}
