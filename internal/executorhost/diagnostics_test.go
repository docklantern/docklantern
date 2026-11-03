package executorhost

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeDiagnosticRunner struct {
	calls   []Invocation
	outputs map[string]string
	failAt  string
	onCall  func(Invocation)
}

func (f *fakeDiagnosticRunner) Run(ctx context.Context, inv Invocation) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if deadline, has := ctx.Deadline(); !has || time.Until(deadline) > CommandTimeout+time.Second {
		return "", errors.New("missing bounded deadline")
	}
	f.calls = append(f.calls, inv)
	if f.onCall != nil {
		f.onCall(inv)
	}
	key := strings.Join(inv.Args, "\x00")
	if key == f.failAt {
		return "", errors.New("fake command error with secret")
	}
	s, ok := f.outputs[key]
	if !ok {
		return "", errors.New("unexpected fake command")
	}
	return s, nil
}

func invocationKey(args ...string) string { return strings.Join(args, "\x00") }

func trustedDiagnosticsFS() fakeFS {
	f := hostFixture()
	f["/var"] = Entry{Mode: fs.ModeDir | 0o755}
	f["/var/lib"] = Entry{Mode: fs.ModeDir | 0o755}
	f["/var/lib/docklantern"] = Entry{Mode: fs.ModeDir | 0o700}
	f["/var/lib/docklantern/executor"] = Entry{Mode: fs.ModeDir | 0o700}
	f["/var/lib/docklantern/executor/docker-config"] = Entry{Mode: fs.ModeDir | 0o700}
	return f
}

func trustedDiagnosticRunner() *fakeDiagnosticRunner {
	return &fakeDiagnosticRunner{outputs: map[string]string{
		invocationKey("context", "show"):                                           "default",
		invocationKey("--host", LocalSocket, "version", "--format", formatVersion): "29.8.1|29.8.1|1.56",
		invocationKey("--host", LocalSocket, "info", "--format", formatInfo):       "linux|x86_64|/usr/libexec/docker/cli-plugins/docker-compose",
		invocationKey("--host", LocalSocket, "compose", "version", "--short"):      "v5.5.1",
	}}
}

func TestDiagnosticsReadOnlyAndUnqualified(t *testing.T) {
	r := trustedDiagnosticRunner()
	result, err := Diagnose(context.Background(), DefaultDiagnosticConfig(), trustedDiagnosticsFS(), r)
	if err != nil {
		t.Fatal(err)
	}
	if result.ClientVersion != "29.8.1" || result.ServerAPI != "1.56" || result.ComposeVersion != "v5.5.1" || result.QualifiedForDeployment {
		t.Fatalf("unexpected diagnostic result: %#v", result)
	}
	if len(r.calls) != 4 {
		t.Fatalf("want 4 fixed read-only queries, got %d", len(r.calls))
	}
	for _, inv := range r.calls {
		if inv.Binary != "/usr/bin/docker" || !approvedReadOnlyArgs(inv.Args) || !reflect.DeepEqual(inv.Env, MinimalChildEnvironment(nil)) {
			t.Errorf("untrusted process invocation: %#v", inv)
		}
		for _, token := range inv.Args {
			if token == "pull" || token == "up" || token == "apply" || token == "exec" || token == "down" || token == "--yes" {
				t.Errorf("unexpected mutating flag/command in %q", inv.Args)
			}
		}
	}
}

func TestDiagnosticsRejectUnsafeSourceBeforeAnyCLI(t *testing.T) {
	cfg := DefaultDiagnosticConfig()
	scenarios := []struct {
		name   string
		mutate func(fakeFS, *DiagnosticConfig)
	}{
		{"group-writable-docker-dir", func(f fakeFS, _ *DiagnosticConfig) {
			f["/usr/bin"] = Entry{Mode: fs.ModeDir | 0o775}
		}},
		{"symlink-plugin", func(f fakeFS, _ *DiagnosticConfig) {
			f["/usr/libexec/docker/cli-plugins/docker-compose"] = Entry{Mode: fs.ModeSymlink | 0o777}
		}},
		{"writable-config", func(f fakeFS, _ *DiagnosticConfig) {
			f["/var/lib/docklantern/executor/docker-config"] = Entry{Mode: fs.ModeDir | 0o770}
		}},
		{"missing-config", func(f fakeFS, _ *DiagnosticConfig) {
			delete(f, "/var/lib/docklantern/executor/docker-config")
		}},
		{"relative-binary", func(_ fakeFS, cfg *DiagnosticConfig) { cfg.DockerBinary = "docker" }},
		{"same-binaries", func(_ fakeFS, cfg *DiagnosticConfig) { cfg.ComposePlugin = cfg.DockerBinary }},
	}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			f := trustedDiagnosticsFS()
			candidate := cfg
			tc.mutate(f, &candidate)
			r := trustedDiagnosticRunner()
			if _, err := Diagnose(context.Background(), candidate, f, r); err == nil {
				t.Fatal("unsafe source accepted")
			}
			if len(r.calls) != 0 {
				t.Fatalf("executed command before trust validation: %#v", r.calls)
			}
		})
	}
}

func TestDiagnosticsRejectUnexpectedCLIOutputs(t *testing.T) {
	scenarios := []struct{ name, key, response string }{
		{"context-remote", invocationKey("context", "show"), "production"},
		{"unstructured-docker-version", invocationKey("--host", LocalSocket, "version", "--format", formatVersion), "29.8.1 secret"},
		{"missing-plugin", invocationKey("--host", LocalSocket, "info", "--format", formatInfo), "linux|amd64|"},
		{"unexpected-plugin", invocationKey("--host", LocalSocket, "info", "--format", formatInfo), "linux|amd64|/tmp/plugin"},
		{"not-linux", invocationKey("--host", LocalSocket, "info", "--format", formatInfo), "windows|amd64|/usr/libexec/docker/cli-plugins/docker-compose"},
		{"unsupported-compose", invocationKey("--host", LocalSocket, "compose", "version", "--short"), "v1.29.2"},
		{"raw-extraneous-output", invocationKey("--host", LocalSocket, "compose", "version", "--short"), "v5.5.1\nSECRET"},
		{"huge-output", invocationKey("--host", LocalSocket, "compose", "version", "--short"), strings.Repeat("X", MaxOutputBytes+1)},
	}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			r := trustedDiagnosticRunner()
			r.outputs[tc.key] = tc.response
			result, err := Diagnose(context.Background(), DefaultDiagnosticConfig(), trustedDiagnosticsFS(), r)
			if err == nil || result.QualifiedForDeployment {
				t.Fatalf("unsafe output accepted: %#v", result)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("leaked fake output: %v", err)
			}
		})
	}
}

func TestDiagnosticsNoVersionClaimOnRunnerErrorOrCancel(t *testing.T) {
	r := trustedDiagnosticRunner()
	r.failAt = invocationKey("--host", LocalSocket, "info", "--format", formatInfo)
	out, err := Diagnose(context.Background(), DefaultDiagnosticConfig(), trustedDiagnosticsFS(), r)
	if err == nil || out.QualifiedForDeployment || strings.Contains(err.Error(), "secret") {
		t.Fatalf("runner error should fail closed without leaking: out=%#v err=%v", out, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = trustedDiagnosticRunner()
	if _, err := Diagnose(ctx, DefaultDiagnosticConfig(), trustedDiagnosticsFS(), r); err == nil || len(r.calls) > 0 {
		t.Fatalf("canceled context invoked Docker commands: %v %#v", err, r.calls)
	}
}

func TestDiagnosticsPluginMustRemainTrustedAfterDockerInfo(t *testing.T) {
	f := trustedDiagnosticsFS()
	r := trustedDiagnosticRunner()
	r.onCall = func(inv Invocation) {
		if reflect.DeepEqual(inv.Args, []string{"--host", LocalSocket, "info", "--format", formatInfo}) {
			f["/usr/libexec/docker/cli-plugins/docker-compose"] = Entry{Mode: fs.ModeSymlink | 0o777}
		}
	}
	if _, err := Diagnose(context.Background(), DefaultDiagnosticConfig(), f, r); err == nil {
		t.Fatal("plugin changed after info was not rejected")
	}
	if len(r.calls) != 3 {
		t.Fatalf("compose executed after plugin changed: %d calls", len(r.calls))
	}
}

func TestProcessRunnerRejectsUnexpectedCommandsWithoutExec(t *testing.T) {
	for _, args := range [][]string{
		{"pull", "nginx"}, {"compose", "up", "-d"}, {"--host", "tcp://evil:2375", "version", "--format", formatVersion},
		{"--host", LocalSocket, "container", "rm", "name"}, {"exec", "sh"}, {"context", "use", "evil"},
	} {
		if approvedReadOnlyArgs(args) {
			t.Fatalf("accepted unsafe command %q", args)
		}
		if _, err := (processRunner{}).Run(context.Background(), Invocation{Binary: "/bin/false", Args: args}); err == nil {
			t.Fatalf("runner accepted unsafe command %q", args)
		}
	}
}

func TestBoundedCaptureDoesNotGrowUnbounded(t *testing.T) {
	b := &boundedBuffer{}
	input := strings.Repeat("x", MaxOutputBytes*4)
	if n, err := b.Write([]byte(input)); err != nil || n != len(input) {
		t.Fatalf("write n=%d err=%v", n, err)
	}
	if !b.exceeded || len(b.String()) != MaxOutputBytes {
		t.Fatalf("limit failed: %d %v", len(b.String()), b.exceeded)
	}
	exact := &boundedBuffer{}
	if _, err := exact.Write([]byte(strings.Repeat("X", MaxOutputBytes))); err != nil || exact.exceeded {
		t.Fatalf("exact limit rejected: %v", err)
	}
}
