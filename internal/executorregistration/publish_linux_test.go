package executorregistration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Root/TTY checks protect the only exported production registration writer.
// CI exercises the private atomic publisher exclusively under a disposable
// root-owned fixture; it never uses the real /var/lib/docklantern location.
func TestPublishProtectedRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_REGISTRATION_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicitly enabled root-owned disposable fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-publish-fixture-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	store := filepath.Join(root, "registrations")
	project := filepath.Join(root, "project")
	for _, dir := range []string{store, project} {
		if err := os.Mkdir(dir, 0o700); err != nil { t.Fatal(err) }
	}
	compose := filepath.Join(project, "compose.yaml")
	env := filepath.Join(project, ".env")
	for _, path := range []string{compose, env} {
		if err := os.WriteFile(path, []byte("inert fixture\n"), 0o600); err != nil { t.Fatal(err) }
	}
	reg := fixture()
	reg.ProjectDirectory = project
	reg.ComposeFiles = []string{compose}
	reg.EnvFiles = []string{env}
	reg.TrustedSourceUID = uid(0)
	created, err := publishProtected(store, reg, "")
	if err != nil { t.Fatalf("create failed: %v",err) }
	if created.Registration.RegistrationID != reg.RegistrationID || !strings.HasPrefix(created.SHA256,"sha256:") {
		t.Fatal("created registration identity wrong")
	}
	loaded, err := readProtected(store, reg.RegistrationID)
	if err != nil || loaded.SHA256 != created.SHA256 {
		t.Fatalf("created private record not readable: %v",err)
	}
	if _,err:=publishProtected(store,reg,"");!errors.Is(err,ErrUntrustedStore) {
		t.Fatal("blind overwrite without expected previous content allowed")
	}
	if _,err:=publishProtected(store,reg,"sha256:"+strings.Repeat("0",64));!errors.Is(err,ErrUntrustedStore) {
		t.Fatal("wrong expected old hash allowed")
	}
	reg.Services[0].AllowRunningOnly = true
	updated,err:=publishProtected(store,reg,created.SHA256)
	if err != nil || updated.SHA256 == created.SHA256 || !updated.Registration.Services[0].AllowRunningOnly {
		t.Fatalf("compare-and-replace failed: %#v %v",updated,err)
	}
	if _,err:=publishProtected(store,reg,created.SHA256);!errors.Is(err,ErrUntrustedStore) {
		t.Fatal("replayed old registration hash accepted")
	}
	// Failure must preserve the last known-good protected registration.
	stable, err:=readProtected(store,reg.RegistrationID)
	if err!=nil || stable.SHA256!=updated.SHA256 { t.Fatal("rejected write changed protected registration") }
	if err:=os.Chmod(store,0o770);err!=nil {t.Fatal(err)}
	if _,err:=publishProtected(store,reg,updated.SHA256);!errors.Is(err,ErrUntrustedStore) {
		t.Fatal("group-writable registration store accepted")
	}
	if err:=os.Chmod(store,0o700);err!=nil {t.Fatal(err)}
	if err:=os.Chmod(project,0o775);err!=nil {t.Fatal(err)}
	if _,err:=publishProtected(store,reg,updated.SHA256);!errors.Is(err,ErrUntrustedStore) {
		t.Fatal("group-writable source project accepted")
	}
	if err:=os.Chmod(project,0o700);err!=nil {t.Fatal(err)}
	if _,err:=publishProtected(store,reg,"bad-digest");!errors.Is(err,ErrUntrustedStore) {
		t.Fatal("malformed previous hash accepted")
	}
	// No temporary registration files must remain after success/failure.
	entries,err:=os.ReadDir(store)
	if err!=nil || len(entries)!=1 || entries[0].Name()!=reg.RegistrationID+".json" {
		t.Fatalf("unexpected private store after publication: %v %#v",err,entries)
	}
}
