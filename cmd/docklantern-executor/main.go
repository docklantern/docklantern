// docklantern-executor is a host-native, operator-invoked CLI.
// Phase 1c provides only read-only diagnostics, help and version reporting.
package main

import (
	"os"

	"github.com/docklantern/docklantern/internal/buildinfo"
	"github.com/docklantern/docklantern/internal/executorcli"
)

func main() {
	os.Exit(executorcli.Run(os.Args[1:], os.Stdout, os.Stderr, buildinfo.Version))
}
