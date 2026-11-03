package executorcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/docklantern/docklantern/internal/executorhost"
)

func TestDiagnosePrintsSafeFactsWithoutGrantingSupport(t *testing.T) {
	var out, errOut bytes.Buffer
	called := 0
	fn := func(_ context.Context, cfg executorhost.DiagnosticConfig) (executorhost.DiagnosticReport, error) {
		called++
		if cfg.DockerBinary != "/usr/bin/docker" || cfg.ComposePlugin != "/usr/libexec/docker/cli-plugins/docker-compose" {
			t.Fatalf("unexpected defaults: %#v", cfg)
		}
		return executorhost.DiagnosticReport{
			ClientVersion: "29.8.1", ServerVersion: "29.8.1", ServerAPI: "1.56", ComposeVersion: "v5.5.1",
			DockerContext: "default", ServerOS: "linux", ServerArchitecture: "x86_64", ComposePlugin: cfg.ComposePlugin,
		}, nil
	}
	code := RunWithDiagnostics([]string{"diagnose"}, &out, &errOut, "dev", fn)
	if code != ExitOK || called != 1 || errOut.Len() != 0 {
		t.Fatalf("code=%d called=%d err=%s", code, called, errOut.String())
	}
	if !strings.Contains(out.String(), "Compose version: v5.5.1") || !strings.Contains(out.String(), "Deployment support: UNQUALIFIED") {
		t.Fatalf("unexpected diagnostics stdout: %s", out.String())
	}
}

func TestDiagnosticFlagsStrictAndNoBypass(t *testing.T) {
	good := []string{"diagnose", "--compose-plugin", "/opt/docker/cli-plugins/docker-compose", "--docker-binary", "/usr/bin/docker"}
	called := false
	mock := func(_ context.Context, cfg executorhost.DiagnosticConfig) (executorhost.DiagnosticReport, error) {
		called = true
		if cfg.ComposePlugin != "/opt/docker/cli-plugins/docker-compose" {
			t.Fatalf("plugin not passed: %#v", cfg)
		}
		return executorhost.DiagnosticReport{}, nil
	}
	var out, errOut bytes.Buffer
	if code := RunWithDiagnostics(good, &out, &errOut, "dev", mock); code != ExitOK || !called {
		t.Fatalf("flags rejected: code=%d", code)
	}
	invalid := [][]string{
		{"diagnose", "--yes"},
		{"diagnose", "--socket", "tcp://evil:2375"},
		{"diagnose", "--docker-binary"},
		{"diagnose", "--docker-binary", "docker"},
		{"diagnose", "--compose-plugin", "/tmp/../usr/bin/foo"},
		{"diagnose", "--docker-binary", "/usr/bin/docker", "--docker-binary", "/usr/bin/docker"},
		{"diagnose", "--compose-plugin", "/usr/bin/docker", "--docker-binary", "/usr/bin/docker"},
		{"diagnose", "--docker-binary", "/usr/bin/docker", "--extra", "/bin/bash"},
		{"diagnose", "--compose-plugin", "/usr/bin/docker\nsecret"},
		{"diagnose", "--docker-binary", "/bin/sh", "apply"},
	}
	for _, args := range invalid {
		called = false
		var stdout, stderr bytes.Buffer
		if code := RunWithDiagnostics(args, &stdout, &stderr, "dev", mock); code != ExitUsage || called || stdout.Len() != 0 {
			t.Errorf("args=%q: code=%d called=%v stdout=%s", args, code, called, stdout.String())
		}
	}
}

func TestDiagnosticErrorIsSanitizedAndFailClosed(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithDiagnostics([]string{"diagnose"}, &out, &errOut, "dev", func(context.Context, executorhost.DiagnosticConfig) (executorhost.DiagnosticReport, error) {
		return executorhost.DiagnosticReport{}, errors.New("secret=not-for-logs /home/private/config")
	})
	if code != ExitUnavailable || out.Len() != 0 || strings.Contains(errOut.String(), "secret") || strings.Contains(errOut.String(), "/home/") {
		t.Fatalf("diagnose failure leaked data: code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}
