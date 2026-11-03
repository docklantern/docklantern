// docklantern-executor is a host-native, operator-invoked CLI.
// The Phase 2 host-only registration and read-only preview commands are available;
// Docker deployment and recovery remain disabled.
package main

import (
	"os"

	"github.com/docklantern/docklantern/internal/buildinfo"
	"github.com/docklantern/docklantern/internal/executorcli"
)

func main() {
	os.Exit(executorcli.Run(os.Args[1:], os.Stdout, os.Stderr, buildinfo.Version))
}
