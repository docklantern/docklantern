package executorcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/docklantern/docklantern/internal/executorhost"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

func disabledDiagnostic(context.Context, executorhost.DiagnosticConfig) (executorhost.DiagnosticReport, error) {
	return executorhost.DiagnosticReport{}, errors.New("diagnose must not run")
}

func TestRegisterAcceptsOnlyExplicitRootFileAndPreviousHash(t *testing.T) {
	const validFile = "/root/docklantern/registration.json"
	const validHash = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, args := range [][]string{
		{"register", "--file", validFile},
		{"register", "--file", validFile, "--expect-sha256", validHash},
		{"register", "--expect-sha256", validHash, "--file", validFile},
	} {
		calls := 0
		var output, stderr bytes.Buffer
		action := func(path, old string) (executorregistration.Record, error) {
			calls++
			if path != validFile {
				t.Fatalf("unexpected file path %q", path)
			}
			if len(args) == 3 && old != "" {
				t.Fatal("create received unexpected previous hash")
			}
			if len(args) == 5 && old != validHash {
				t.Fatalf("update not bound to previous record: %q", old)
			}
			return executorregistration.Record{
				Registration: executorregistration.Registration{RegistrationID: "hello-demo"},
				SHA256:       validHash,
			}, nil
		}
		code := RunWithActions(args, &output, &stderr, "dev", disabledDiagnostic, action)
		if code != ExitOK || calls != 1 || stderr.Len() != 0 ||
			!strings.Contains(output.String(), "Registered hello-demo") ||
			!strings.Contains(output.String(), "Deployment and recovery remain disabled") {
			t.Fatalf("args %q: code=%d calls=%d out=%q err=%q", args, code, calls, output.String(), stderr.String())
		}
	}
}

func TestRegisterRejectsUnsafeFlagsBeforeHostAction(t *testing.T) {
	bad := [][]string{
		{"register"},
		{"register", "--yes"},
		{"register", "--file"},
		{"register", "--file", "-"},
		{"register", "--file", "registration.json"},
		{"register", "--file", "../registration.json"},
		{"register", "--file", "/root/../tmp/registration.json"},
		{"register", "--file", "/root//registration.json"},
		{"register", "--file", "/root/registration.json/"},
		{"register", "--file", "/root/registration.json", "--yes"},
		{"register", "--file", "/root/registration.json", "--expect-sha256", "latest"},
		{"register", "--file", "/root/registration.json", "--expect-sha256", "sha256:1234"},
		{"register", "--file", "/root/registration.json", "--expect-sha256", "sha256:" + strings.Repeat("A", 64)},
		{"register", "--file", "/root/registration.json", "--expect-sha256", "sha256:" + strings.Repeat("a", 63)},
		{"register", "--file", "/root/registration.json", "--expect-sha256", ""},
		{"register", "--expect-sha256", "sha256:" + strings.Repeat("a", 64)},
		{"register", "--file", "/root/registration.json", "--file", "/etc/shadow"},
		{"register", "--file", "/root/registration.json", "--docker-host", "tcp://evil:2375"},
		{"register", "--file", "/root/registration.json", "--input-from-stdin", "true"},
		{"register", "--file", "/root/registration.json", "--expect-sha256", "sha256:" + strings.Repeat("a", 64), "--yes", "true"},
		{"register", "--file", "/root/registration.json", "apply"},
		{"register", "--file", "/root/x\nsecret.json"},
	}
	for _, args := range bad {
		calls := 0
		var output, stderr bytes.Buffer
		code := RunWithActions(args, &output, &stderr, "dev", disabledDiagnostic, func(string, string) (executorregistration.Record, error) {
			calls++
			return executorregistration.Record{}, nil
		})
		if code != ExitUsage || calls != 0 || output.Len() != 0 || stderr.Len() == 0 ||
			strings.Contains(stderr.String(), "/root/") {
			t.Errorf("args %q: invalid request reached privileged action: code=%d calls=%d out=%q err=%q",
				args, code, calls, output.String(), stderr.String())
		}
	}
}

func TestRegisterErrorsAreSanitizedAndFailClosed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	n := 0
	code := RunWithActions(
		[]string{"register", "--file", "/root/registration.json"},
		&stdout, &stderr, "dev", disabledDiagnostic,
		func(string, string) (executorregistration.Record, error) {
			n++
			return executorregistration.Record{}, errors.New("secret=private docker credentials /root/input.json")
		},
	)
	if n != 1 || code != ExitUnavailable || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "could not be completed") ||
		strings.Contains(stderr.String(), "secret") || strings.Contains(stderr.String(), "/root/") {
		t.Fatalf("registration error leaked: calls=%d code=%d stdout=%q stderr=%q",
			n, code, stdout.String(), stderr.String())
	}
}

func TestRegisterNeverEnablesDeploymentCommands(t *testing.T) {
	calls := 0
	action := func(string, string) (executorregistration.Record, error) {
		calls++
		return executorregistration.Record{}, errors.New("should not be called")
	}
	for _, args := range [][]string{
		{"apply", "hello-demo"}, {"recover", "hello-demo"},
		{"apply", "--yes"}, {"reconcile", "hello-demo"}, {"status", "hello-demo"},
	} {
		var out, stderr bytes.Buffer
		code := RunWithActions(args, &out, &stderr, "dev", disabledDiagnostic, action)
		if code == ExitOK || calls != 0 || out.Len() != 0 {
			t.Errorf("deployment operation %q inadvertently enabled: code=%d called=%d", args, code, calls)
		}
	}
}

func TestRegisterHelpExplainsRootTTYAndNoDockerWrites(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := RunWithActions([]string{"help"}, &out, &stderr, "dev", disabledDiagnostic, nil); code != ExitOK {
		t.Fatal("help failed")
	}
	for _, s := range []string{"register", "--file", "--expect-sha256", "root", "TTY", "no Docker write capability"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("missing help disclosure: %q", s)
		}
	}
}
