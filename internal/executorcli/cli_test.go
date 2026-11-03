package executorcli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommands(t *testing.T) {
	for _, command := range []string{"version", "-version", "--version"} {
		t.Run(command, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run([]string{command}, &out, &errOut, "v0.1.0-beta.6-test")
			if code != ExitOK || out.String() != "DockLantern Executor v0.1.0-beta.6-test\n" || errOut.Len() != 0 {
				t.Fatalf("code=%d, stdout=%q, stderr=%q", code, out.String(), errOut.String())
			}
		})
	}
}

func TestHelpCommands(t *testing.T) {
	for _, command := range []string{"help", "-h", "--help"} {
		t.Run(command, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run([]string{command}, &out, &errOut, "dev")
			if code != ExitOK || !strings.Contains(out.String(), "no Docker write capability") || errOut.Len() != 0 {
				t.Fatalf("code=%d, stdout=%q, stderr=%q", code, out.String(), errOut.String())
			}
		})
	}
}

func TestUnavailableCommandsFailClosed(t *testing.T) {
	for _, command := range []string{"status", "register", "preview", "apply", "reconcile", "recover"} {
		t.Run(command, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run([]string{command}, &out, &errOut, "dev")
			if code != ExitUnavailable || out.Len() != 0 || !strings.Contains(errOut.String(), command+" unavailable") {
				t.Fatalf("code=%d, stdout=%q, stderr=%q", code, out.String(), errOut.String())
			}
		})
	}
}

func TestUnexpectedInputsNeverExecute(t *testing.T) {
	cases := [][]string{
		nil,
		{},
		{"--yes"},
		{"apply", "--yes"},
		{"recover", "--yes"},
		{"apply", "--confirm-unsafe-poc-deploy"},
		{"version", "apply"},
		{"status", "--docker-host", "unix:///var/run/docker.sock"},
		{"docker", "compose", "up"},
		{"-mode", "apply"},
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		code := Run(args, &out, &errOut, "dev")
		if code != ExitUsage || out.Len() != 0 || errOut.Len() == 0 {
			t.Errorf("args=%q: code=%d stdout=%q stderr=%q", args, code, out.String(), errOut.String())
		}
	}
}
