// Package executorcli implements the nonprivileged, Phase 1a CLI shell.
// It intentionally has no Docker, filesystem, subprocess, or network access.
package executorcli

import (
	"fmt"
	"io"
)

const (
	ExitOK          = 0
	ExitUsage       = 2
	ExitUnavailable = 3
)

// Run dispatches only implemented commands. Later phases must add their own
// authorization and trusted backends before making any deployment operation
// available. Version is supplied by the caller so tests need no global state.
func Run(args []string, stdout, stderr io.Writer, version string) int {
	if len(args) == 0 {
		usage(stderr)
		return ExitUsage
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, "docklantern-executor: unexpected arguments (no command options are supported)")
		return ExitUsage
	}

	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return ExitOK
	case "version", "-version", "--version":
		fmt.Fprintf(stdout, "DockLantern Executor %s\n", version)
		return ExitOK
	case "status", "register", "preview", "apply", "reconcile", "recover":
		// Explicitly fail closed: no host/Docker access, even for status.
		fmt.Fprintf(stderr, "docklantern-executor: %s unavailable (Phase 1a: executor backend not implemented)\n", args[0])
		return ExitUnavailable
	default:
		fmt.Fprintln(stderr, "docklantern-executor: unknown or unsupported command")
		return ExitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `DockLantern Executor (Phase 1a, no Docker write capability)
Usage: docklantern-executor <command>

Available:
  help       Show this help
  version    Show the executor build version

Reserved (unavailable until later reviewed phases):
  status, register, preview, apply, reconcile, recover

No deployment, Docker access, or unattended execution is available.`)
}
