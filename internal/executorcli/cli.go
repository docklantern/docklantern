// Package executorcli implements the host-native CLI. Phase 1c only adds
// opt-in read-only diagnostics; deployment and recovery remain unavailable.
package executorcli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docklantern/docklantern/internal/executorhost"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

const (
	ExitOK          = 0
	ExitUsage       = 2
	ExitUnavailable = 3
)

type DiagnoseFunc func(context.Context, executorhost.DiagnosticConfig) (executorhost.DiagnosticReport, error)

// RegisterFunc is injected for CLI unit tests; the production implementation
// reads a trusted, root-owned file and uses a controlling TTY before writing
// the executor-owned registration. It never contacts Docker.
type RegisterFunc func(path, previousSHA256 string) (executorregistration.Record, error)

// Run is invoked only by the host CLI and never by the monitoring web service.
// It does not provide apply/recover or any Docker mutation capability.
func Run(args []string, stdout, stderr io.Writer, version string) int {
	return RunWithDiagnostics(args, stdout, stderr, version, executorhost.DiagnoseLocal)
}

// RunWithDiagnostics supports deterministic, Docker-free unit tests.
func RunWithDiagnostics(args []string, stdout, stderr io.Writer, version string, diagnose DiagnoseFunc) int {
	return RunWithActions(args, stdout, stderr, version, diagnose, executorregistration.RegisterFromFile)
}

// RunWithActions isolates registration and diagnostics effects behind
// injectable dependencies, so rejected flags cannot trigger host changes.
func RunWithActions(args []string, stdout, stderr io.Writer, version string, diagnose DiagnoseFunc, register RegisterFunc) int {
	if len(args) == 0 {
		usage(stderr)
		return ExitUsage
	}
	if args[0] == "diagnose" {
		config, ok := parseDiagnosticArgs(args[1:])
		if !ok || diagnose == nil {
			fmt.Fprintln(stderr, "docklantern-executor: invalid diagnostics arguments")
			return ExitUsage
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		report, err := diagnose(ctx, config)
		if err != nil {
			// Avoid forwarding subprocess/host-specific error details to any
			// output that might later be copied into untrusted issue reports.
			fmt.Fprintln(stderr, "docklantern-executor: read-only diagnostics could not verify the trusted local environment")
			return ExitUnavailable
		}
		fmt.Fprintf(stdout, "Docker client: %s\nDocker server: %s\nDocker API: %s\nDocker context: %s\nDocker socket: %s\nCompose version: %s\nCompose plugin: %s\nHost: %s/%s\nDeployment support: UNQUALIFIED (read-only observations only)\n",
			report.ClientVersion, report.ServerVersion, report.ServerAPI, report.DockerContext,
			executorhost.LocalSocket, report.ComposeVersion, report.ComposePlugin, report.ServerOS, report.ServerArchitecture)
		return ExitOK
	}
	if args[0] == "register" {
		path, expected, ok := parseRegistrationArgs(args[1:])
		if !ok || register == nil {
			fmt.Fprintln(stderr, "docklantern-executor: invalid registration arguments")
			return ExitUsage
		}
		record, err := register(path, expected)
		if err != nil {
			// Never print the candidate JSON, untrusted path or raw TTY errors.
			fmt.Fprintln(stderr, "docklantern-executor: trusted host registration could not be completed")
			return ExitUnavailable
		}
		fmt.Fprintf(stdout, "Registered %s (%s). Deployment and recovery remain disabled.\n",
			record.Registration.RegistrationID, record.SHA256)
		return ExitOK
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, "docklantern-executor: unexpected arguments")
		return ExitUsage
	}

	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return ExitOK
	case "version", "-version", "--version":
		fmt.Fprintf(stdout, "DockLantern Executor %s\n", version)
		return ExitOK
	case "status", "preview", "apply", "reconcile", "recover":
		fmt.Fprintf(stderr, "docklantern-executor: %s unavailable (executor backend not implemented)\n", args[0])
		return ExitUnavailable
	default:
		fmt.Fprintln(stderr, "docklantern-executor: unknown or unsupported command")
		return ExitUsage
	}
}

// parseRegistrationArgs requires one root-controlled absolute JSON source
// filename and optionally the exact old registration hash for replacement.
// No stdin, no untrusted GUI/monitor path, no --yes and no host override.
func parseRegistrationArgs(args []string) (string, string, bool) {
	if len(args) != 2 && len(args) != 4 {
		return "", "", false
	}
	var path, expected string
	seen := make(map[string]bool, 2)
	for i := 0; i < len(args); i += 2 {
		flag, value := args[i], args[i+1]
		if seen[flag] || value == "" {
			return "", "", false
		}
		seen[flag] = true
		switch flag {
		case "--file":
			if !executorregistration.ValidInputPath(value) {
				return "", "", false
			}
			path = value
		case "--expect-sha256":
			if !validRegistrationSHA256(value) {
				return "", "", false
			}
			expected = value
		default:
			return "", "", false
		}
	}
	return path, expected, path != ""
}

func validRegistrationSHA256(s string) bool {
	if len(s) != 71 || s[:7] != "sha256:" {
		return false
	}
	for _, c := range s[7:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func parseDiagnosticArgs(args []string) (executorhost.DiagnosticConfig, bool) {
	c := executorhost.DefaultDiagnosticConfig()
	if len(args)%2 != 0 || len(args) > 4 {
		return c, false
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		key, value := args[i], args[i+1]
		if seen[key] || !executorhost.SafeDiagnosticToolPath(value) {
			return c, false
		}
		seen[key] = true
		switch key {
		case "--docker-binary":
			c.DockerBinary = value
		case "--compose-plugin":
			c.ComposePlugin = value
		default:
			return c, false
		}
	}
	if c.DockerBinary == c.ComposePlugin {
		return c, false
	}
	return c, true
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `DockLantern Executor (Phase 2 host registration, no Docker write capability)
Usage: docklantern-executor <command>

Available:
  help       Show this help
  version    Show the executor build version
  diagnose   Opt-in local Docker/Compose read-only diagnostics
             [--docker-binary ABSOLUTE_PATH] [--compose-plugin ABSOLUTE_PATH]
             Requires existing protected host config directories.
  register   Host-root + controlling-TTY protected registration publication
             --file ABSOLUTE_ROOT_OWNED_JSON [--expect-sha256 sha256:OLD_HASH]
             Creates a new registration when --expect-sha256 is omitted;
             a change to an existing record requires its exact previous hash.

Reserved (unavailable until later reviewed phases):
  status, preview, apply, reconcile, recover

No deployment, Docker writes, or unattended execution is available.`)
}
