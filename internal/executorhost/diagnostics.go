package executorhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Diagnostics perform a fixed list of read-only Docker CLI queries. They do
// not authorize deployment or establish safe Compose mutation semantics.
const (
	LocalSocket    = "unix:///var/run/docker.sock"
	CommandTimeout = 5 * time.Second
	MaxOutputBytes = 4096
)

// DiagnosticConfig is supplied by the local operator, never by a GUI proposal.
// The socket is intentionally NOT configurable in Phase 1c.
type DiagnosticConfig struct {
	DockerBinary  string
	ComposePlugin string
}

func DefaultDiagnosticConfig() DiagnosticConfig {
	return DiagnosticConfig{
		DockerBinary:  "/usr/bin/docker",
		ComposePlugin: "/usr/libexec/docker/cli-plugins/docker-compose",
	}
}

// DiagnosticReport is safe for terminal printing: it contains parsed version
// tokens, fixed local-socket metadata and a trusted Compose executable path.
// QualifiedForDeployment must remain false until later security/compatibility
// phases explicitly grant and test a supported Engine/Compose pair.
type DiagnosticReport struct {
	ClientVersion          string
	ServerVersion          string
	ServerAPI              string
	ComposeVersion         string
	DockerContext          string
	ServerOS               string
	ServerArchitecture     string
	ComposePlugin          string
	QualifiedForDeployment bool
}

// Invocation specifies a read-only command. Values are constructed by this
// package, not by a proposal or user-supplied arbitrary command string.
type Invocation struct {
	Binary string
	Args   []string
	Env    []string
}

type ReadOnlyRunner interface {
	Run(context.Context, Invocation) (string, error)
}

// processRunner is the internal read-only subprocess runner. There are no
// shell invocations and it refuses every command not on the fixed allowlist.
// The executor must never expose this runner to future mutation commands.
type processRunner struct{}

func (processRunner) Run(ctx context.Context, inv Invocation) (string, error) {
	if !approvedReadOnlyArgs(inv.Args) || inv.Binary == "" {
		return "", errors.New("unsupported diagnostics command")
	}
	if ctx == nil {
		return "", errors.New("nil diagnostics context")
	}
	cctx, cancel := context.WithTimeout(ctx, CommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, inv.Binary, inv.Args...)
	cmd.Dir = "/"
	cmd.Env = inv.Env
	cmd.Stdin = strings.NewReader("")
	cmd.WaitDelay = time.Second
	stdout, stderr := &boundedBuffer{}, &boundedBuffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if stdout.exceeded || stderr.exceeded {
		return "", errors.New("diagnostics subprocess output limit exceeded")
	}
	if cctx.Err() != nil {
		return "", errors.New("diagnostics subprocess deadline exceeded")
	}
	if err != nil {
		// Do not expose arbitrary child error text, flags, env or stdout/stderr.
		return "", errors.New("diagnostics subprocess failed")
	}
	return strings.TrimSpace(stdout.String()), nil
}

type boundedBuffer struct {
	b        []byte
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := MaxOutputBytes - len(b.b)
	if n > remaining {
		b.exceeded = true
	}
	if remaining > 0 {
		if n < remaining {
			remaining = n
		}
		b.b = append(b.b, p[:remaining]...)
	}
	return n, nil
}

func (b *boundedBuffer) String() string { return string(b.b) }

var absoluteToolPath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

func SafeDiagnosticToolPath(path string) bool {
	return absoluteToolPath.MatchString(path) && filepath.Clean(path) == path && path != "/"
}

var versionToken = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[+.-][A-Za-z0-9][A-Za-z0-9.+-]*)?$`)
var apiToken = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

func validVersion(value string) bool {
	return len(value) <= 64 && versionToken.MatchString(value)
}

const (
	formatVersion = "{{.Client.Version}}|{{.Server.Version}}|{{.Server.APIVersion}}"
	formatInfo    = `{{.OSType}}|{{.Architecture}}|{{range .ClientInfo.Plugins}}{{if eq .Name "compose"}}{{.Path}}{{end}}{{end}}`
)

// approvedReadOnlyArgs ensures even a mistaken caller cannot reuse the real
// runner to execute docker pull/up/exec or arbitrary arguments.
func approvedReadOnlyArgs(args []string) bool {
	if len(args) == 2 && args[0] == "context" && args[1] == "show" {
		return true
	}
	if len(args) == 5 && args[0] == "--host" && args[1] == LocalSocket && args[2] == "version" && args[3] == "--format" && args[4] == formatVersion {
		return true
	}
	if len(args) == 5 && args[0] == "--host" && args[1] == LocalSocket && args[2] == "info" && args[3] == "--format" && args[4] == formatInfo {
		return true
	}
	if len(args) == 5 && args[0] == "--host" && args[1] == LocalSocket && args[2] == "compose" && args[3] == "version" && args[4] == "--short" {
		return true
	}
	return false
}

// DiagnoseLocal invokes the approved read-only checks through a restricted
// runner. It is only reachable from the explicitly invoked host CLI, not GUI.
func DiagnoseLocal(ctx context.Context, cfg DiagnosticConfig) (DiagnosticReport, error) {
	return Diagnose(ctx, cfg, RealFS{}, processRunner{})
}

// Diagnose reports observed facts without granting Docker mutation authority.
// Filesystem Lstat checks are preflight-only, not TOCTOU-safe handles.
func Diagnose(ctx context.Context, cfg DiagnosticConfig, reader Reader, runner ReadOnlyRunner) (DiagnosticReport, error) {
	var result DiagnosticReport
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return result, errors.New("diagnostics require Linux/amd64")
	}
	if ctx == nil || reader == nil || runner == nil {
		return result, errors.New("missing diagnostics dependency")
	}
	if !SafeDiagnosticToolPath(cfg.DockerBinary) || !SafeDiagnosticToolPath(cfg.ComposePlugin) || cfg.DockerBinary == cfg.ComposePlugin {
		return result, errors.New("invalid Docker/Compose executable paths")
	}
	if err := ValidateTrustedExecutable(reader, cfg.DockerBinary); err != nil {
		return result, fmt.Errorf("Docker executable not trusted: %w", err)
	}
	if err := ValidateTrustedExecutable(reader, cfg.ComposePlugin); err != nil {
		return result, fmt.Errorf("Compose plugin executable not trusted: %w", err)
	}
	for _, path := range []string{
		"/var/lib/docklantern/executor",
		"/var/lib/docklantern/executor/docker-config",
	} {
		if err := ValidatePrivateDirectory(reader, path); err != nil {
			return result, errors.New("trusted Docker CLI configuration directory is unavailable")
		}
	}
	if err := ctx.Err(); err != nil {
		return result, errors.New("diagnostics canceled")
	}
	env := MinimalChildEnvironment(os.Environ())
	call := func(args ...string) (string, error) {
		cctx, cancel := context.WithTimeout(ctx, CommandTimeout)
		defer cancel()
		got, err := runner.Run(cctx, Invocation{
			Binary: cfg.DockerBinary,
			Args:   append([]string(nil), args...),
			Env:    append([]string(nil), env...),
		})
		if err != nil {
			return "", err
		}
		if len(got) > MaxOutputBytes || strings.ContainsAny(got, "\x00\r\n") {
			return "", errors.New("invalid diagnostic output")
		}
		return strings.TrimSpace(got), nil
	}
	contextName, err := call("context", "show")
	if err != nil || contextName != "default" {
		return result, errors.New("Docker context is not verified default")
	}
	result.DockerContext = contextName
	versions, err := call("--host", LocalSocket, "version", "--format", formatVersion)
	if err != nil {
		return DiagnosticReport{}, errors.New("Docker version read failed")
	}
	v := strings.Split(versions, "|")
	if len(v) != 3 || !validVersion(v[0]) || !validVersion(v[1]) || !apiToken.MatchString(v[2]) {
		return DiagnosticReport{}, errors.New("Docker versions malformed")
	}
	result.ClientVersion, result.ServerVersion, result.ServerAPI = v[0], v[1], v[2]
	info, err := call("--host", LocalSocket, "info", "--format", formatInfo)
	if err != nil {
		return DiagnosticReport{}, errors.New("Docker info read failed")
	}
	fields := strings.Split(info, "|")
	if len(fields) != 3 || fields[0] != "linux" || (fields[1] != "amd64" && fields[1] != "x86_64") || fields[2] != cfg.ComposePlugin {
		return DiagnosticReport{}, errors.New("unsupported host or Compose plugin path mismatch")
	}
	result.ServerOS, result.ServerArchitecture, result.ComposePlugin = fields[0], fields[1], fields[2]
	// Recheck plugin paths before invoking it through Docker CLI. The check
	// is not a stable-handle guarantee and cannot authorize any update.
	if err := ValidateTrustedExecutable(reader, cfg.ComposePlugin); err != nil {
		return DiagnosticReport{}, errors.New("Compose plugin changed during diagnostics")
	}
	compose, err := call("--host", LocalSocket, "compose", "version", "--short")
	if err != nil || !validVersion(compose) || !(strings.HasPrefix(compose, "v2.") || strings.HasPrefix(compose, "2.") || strings.HasPrefix(compose, "v5.") || strings.HasPrefix(compose, "5.")) {
		return DiagnosticReport{}, errors.New("unsupported Compose version output")
	}
	result.ComposeVersion = compose
	// Deliberately no supported-version allowlist until exact-version testing.
	result.QualifiedForDeployment = false
	return result, nil
}
