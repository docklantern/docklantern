package executorregistration

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestTrustedRegistrationInputRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_REGISTRATION_ROOT_FIXTURE") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicitly enabled disposable root-owned fixture")
	}
	root, err := os.MkdirTemp("/root", "docklantern-input-fixture-")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	src := filepath.Join(root, "registration.json")
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil { t.Fatal(err) }
	compose, env := filepath.Join(project, "compose.yaml"), filepath.Join(project, ".env")
	for _, file := range []string{compose, env} {
		if err := os.WriteFile(file, []byte("# inert Compose fixture\n"), 0o600); err != nil { t.Fatal(err) }
	}
	reg := fixture()
	reg.ProjectDirectory = project
	reg.ComposeFiles = []string{compose}
	reg.EnvFiles = []string{env}
	reg.TrustedSourceUID = uid(0)
	valid, err := json.Marshal(reg)
	if err != nil { t.Fatal(err) }
	write := func(data []byte) {
		t.Helper()
		if err := os.WriteFile(src, data, 0o600); err != nil { t.Fatal(err) }
		if err := os.Chmod(src, 0o600); err != nil { t.Fatal(err) }
	}
	expectReject := func(label string) {
		t.Helper()
		if _, err := readTrustedRegistrationInput(src); !errors.Is(err, ErrUntrustedRegistrationInput) {
			t.Fatalf("%s: accepted unsafe input: %v", label, err)
		}
	}

	write(valid)
	found, err := readTrustedRegistrationInput(src)
	if err != nil || found.Registration.RegistrationID != reg.RegistrationID ||
		len(found.SHA256) != 71 {
		t.Fatalf("trusted host source rejected: %+v %v", found, err)
	}

	// A successful input read is not the same as registration publication;
	// the latter requires a root-owned protected store AND a controlling TTY.
	if tty, e := os.OpenFile("/dev/tty", os.O_RDWR, 0); e == nil {
		_ = tty.Close()
	} else {
		if _, err := RegisterFromFile(src, ""); err == nil {
			t.Fatal("headless host unexpectedly published registration")
		}
	}

	withSpace := append([]byte(" \n"), valid...)
	write(withSpace)
	replaced, err := readTrustedRegistrationInput(src)
	if err != nil || replaced.SHA256 == found.SHA256 {
		t.Fatalf("changed exact bytes must yield new hash: %v", err)
	}
	write(valid)

	for _, mode := range []os.FileMode{0o640, 0o400, 0o700, 0o664, 0o666} {
		if err := os.Chmod(src, mode); err != nil { t.Fatal(err) }
		expectReject("non-0600 file mode")
		write(valid)
	}

	if err := os.Chown(src, 65534, -1); err != nil { t.Fatal(err) }
	expectReject("untrusted nonroot owner")
	if err := os.Chown(src, 0, -1); err != nil { t.Fatal(err) }
	write(valid)

	hard := filepath.Join(root, "second-registration-link")
	if err := os.Link(src, hard); err != nil { t.Fatal(err) }
	expectReject("multiply linked source")
	if err := os.Remove(hard); err != nil { t.Fatal(err) }

	if err := os.Remove(src); err != nil { t.Fatal(err) }
	if err := os.Symlink("/etc/shadow", src); err != nil { t.Fatal(err) }
	expectReject("symlinked input")
	if err := os.Remove(src); err != nil { t.Fatal(err) }

	if err := syscall.Mkfifo(src, 0o600); err != nil { t.Fatal(err) }
	expectReject("FIFO input")
	if err := os.Remove(src); err != nil { t.Fatal(err) }

	write(bytes.Repeat([]byte("x"), MaxRegistrationBytes+1))
	expectReject("oversized input")
	write([]byte("{}"))
	expectReject("invalid schema")
	write(bytes.Replace(valid, []byte(`"registration_id"`), []byte(`"registration_id":"forged","registration_id"`), 1))
	expectReject("duplicate JSON key")
	write(valid)

	if err := os.Chmod(root, 0o775); err != nil { t.Fatal(err) }
	expectReject("group-writable parent")
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }

	real := root + "-original"
	if err := os.Rename(root, real); err != nil { t.Fatal(err) }
	if err := os.Symlink(real, root); err != nil { t.Fatal(err) }
	expectReject("symlinked ancestor")
	if err := os.Remove(root); err != nil { t.Fatal(err) }
	if err := os.Rename(real, root); err != nil { t.Fatal(err) }
	write(valid)
	if _, err := readTrustedRegistrationInput(src); err != nil { t.Fatalf("fixture not restored: %v", err) }

	for _, candidate := range []string{
		"", "-", "registration.json", "../registration.json", "/root/../etc/shadow",
		"/root//registration.json", "/root/registration.json/", "/root/bad\nfile",
		"/root/bad\rfile", "/root/bad"+string(rune(0))+"file",
		"/root"+strings.Repeat("/ab", 260)+"/input.json",
	} {
		if trustedInputPath(candidate) {
			t.Fatalf("untrusted filename accepted: %q", candidate)
		}
	}
}
