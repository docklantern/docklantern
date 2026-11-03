package executorregistration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// CI-only isolated host filesystem test. It deliberately does not use the
// default /var/lib/docklantern path or contact Docker/Compose.
func TestProtectedStoreRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_REGISTRATION_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit disposable root-owned filesystem fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-registration-fixture-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { os.RemoveAll(root) })
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	store, project := filepath.Join(root, "registrations"), filepath.Join(root, "project")
	for _, dir := range []string{store, project} {
		if err := os.Mkdir(dir, 0o700); err != nil { t.Fatal(err) }
	}
	compose, env := filepath.Join(project, "compose.yaml"), filepath.Join(project, ".env")
	for _, f := range []string{compose, env} {
		if err := os.WriteFile(f, []byte("# inert fixture\n"), 0o600); err != nil { t.Fatal(err) }
	}
	registration := fixture()
	registration.ProjectDirectory, registration.ComposeFiles, registration.EnvFiles, registration.TrustedSourceUID = project, []string{compose}, []string{env}, uid(0)
	payload, err := json.Marshal(registration)
	if err != nil { t.Fatal(err) }
	file := filepath.Join(store, registration.RegistrationID+".json")
	write := func(data []byte) {
		t.Helper()
		if err := os.WriteFile(file, data, 0o600); err != nil { t.Fatal(err) }
		if err := os.Chmod(file, 0o600); err != nil { t.Fatal(err) }
	}
	write(payload)
	got, err := readProtected(store, registration.RegistrationID)
	if err != nil { t.Fatalf("protected source unexpectedly rejected: %v", err) }
	h := sha256.Sum256(payload)
	if got.Registration.RegistrationID != registration.RegistrationID || got.SHA256 != "sha256:"+hex.EncodeToString(h[:]) {
		t.Fatal("protected loader returned wrong identity or bytes")
	}
	link := filepath.Join(root, "second-hardlink")
	if err := os.Link(file, link); err != nil { t.Fatal(err) }
	if _, err := readProtected(store, registration.RegistrationID); !errors.Is(err, ErrUntrustedStore) { t.Fatal("hardlinked registration accepted") }
	if err := os.Remove(link); err != nil { t.Fatal(err) }
	if err := os.Chmod(file, 0o640); err != nil { t.Fatal(err) }
	if _, err := readProtected(store, registration.RegistrationID); !errors.Is(err, ErrUntrustedStore) { t.Fatal("group-readable registration accepted") }
	write(payload)
	if err := os.Remove(file); err != nil { t.Fatal(err) }
	if err := os.Symlink(compose, file); err != nil { t.Fatal(err) }
	if _, err := readProtected(store, registration.RegistrationID); !errors.Is(err, ErrUntrustedStore) { t.Fatal("symlink registration accepted") }
	if err := os.Remove(file); err != nil { t.Fatal(err) }
	write(payload)
	if err := os.Chmod(store, 0o770); err != nil { t.Fatal(err) }
	if _, err := readProtected(store, registration.RegistrationID); !errors.Is(err, ErrUntrustedStore) { t.Fatal("group-writable registration directory accepted") }
	if err := os.Chmod(store, 0o700); err != nil { t.Fatal(err) }
	if err := os.Chmod(project, 0o775); err != nil { t.Fatal(err) }
	if _, err := readProtected(store, registration.RegistrationID); !errors.Is(err, ErrUntrustedStore) { t.Fatal("unsafe Compose source directory accepted") }
	if err := os.Chmod(project, 0o700); err != nil { t.Fatal(err) }
	write([]byte(`{"schema_version":1,"schema_version":1}`))
	if _, err := readProtected(store, registration.RegistrationID); !errors.Is(err, ErrUntrustedStore) { t.Fatal("malformed registration accepted") }
	write(payload)
	if _, err := readProtected(store, "../escape"); !errors.Is(err, ErrUntrustedStore) { t.Fatal("path traversal ID accepted") }
	if _, err := readProtected(store, "nonexistent"); !errors.Is(err, ErrUntrustedStore) { t.Fatal("unregistered ID accepted") }
}
