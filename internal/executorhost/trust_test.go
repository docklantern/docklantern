package executorhost

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

type fakeFS map[string]Entry

func (f fakeFS) Lstat(p string) (Entry, error) {
	v, ok := f[p]
	if !ok {
		return Entry{}, fs.ErrNotExist
	}
	return v, nil
}

func hostFixture() fakeFS {
	return fakeFS{
		"/usr":                            {Mode: fs.ModeDir | 0o755},
		"/usr/bin":                        {Mode: fs.ModeDir | 0o755},
		"/usr/bin/docker":                 {Mode: 0o755},
		"/usr/libexec":                    {Mode: fs.ModeDir | 0o755},
		"/usr/libexec/docker":             {Mode: fs.ModeDir | 0o755},
		"/usr/libexec/docker/cli-plugins": {Mode: fs.ModeDir | 0o755},
		"/usr/libexec/docker/cli-plugins/docker-compose": {Mode: 0o755},
		"/home":                          {Mode: fs.ModeDir | 0o755},
		"/home/operator":                 {Mode: fs.ModeDir | 0o750, UID: 1001},
		"/home/operator/deployments":     {Mode: fs.ModeDir | 0o750, UID: 1001},
		"/home/operator/deployments/app": {Mode: fs.ModeDir | 0o700, UID: 1001},
		"/home/operator/deployments/app/compose.yaml": {Mode: 0o600, UID: 1001},
	}
}

func TestTrustedExecutable(t *testing.T) {
	paths := []string{"/usr/bin/docker", "/usr/libexec/docker/cli-plugins/docker-compose"}
	for _, p := range paths {
		if err := ValidateTrustedExecutable(hostFixture(), p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	for _, test := range []struct {
		name   string
		path   string
		mutate func(fakeFS)
	}{
		{"symlink-binary", "/usr/bin/docker", func(f fakeFS) { f["/usr/bin/docker"] = Entry{Mode: fs.ModeSymlink | 0o777} }},
		{"non-root-binary", "/usr/bin/docker", func(f fakeFS) { f["/usr/bin/docker"] = Entry{Mode: 0o755, UID: 1001} }},
		{"group-writable-binary", "/usr/bin/docker", func(f fakeFS) { f["/usr/bin/docker"] = Entry{Mode: 0o775} }},
		{"world-writable-ancestor", "/usr/bin/docker", func(f fakeFS) { f["/usr/bin"] = Entry{Mode: fs.ModeDir | 0o777} }},
		{"setuid-binary", "/usr/bin/docker", func(f fakeFS) { f["/usr/bin/docker"] = Entry{Mode: fs.ModeSetuid | 0o755} }},
		{"not-executable", "/usr/bin/docker", func(f fakeFS) { f["/usr/bin/docker"] = Entry{Mode: 0o644} }},
		{"plugin-symlink-parent", "/usr/libexec/docker/cli-plugins/docker-compose", func(f fakeFS) { f["/usr/libexec/docker"] = Entry{Mode: fs.ModeSymlink | 0o755} }},
		{"plugin-missing", "/usr/libexec/docker/cli-plugins/docker-compose", func(f fakeFS) { delete(f, "/usr/libexec/docker/cli-plugins/docker-compose") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := hostFixture()
			test.mutate(f)
			if err := ValidateTrustedExecutable(f, test.path); !errors.Is(err, ErrUntrusted) {
				t.Fatalf("expected fail-closed: %v", err)
			}
		})
	}
	for _, path := range []string{"docker", "usr/bin/docker", "/usr/../bin/docker", "/usr//bin/docker", "/usr/bin/docker/"} {
		if err := ValidateTrustedExecutable(hostFixture(), path); !errors.Is(err, ErrUntrusted) {
			t.Errorf("invalid path %q: %v", path, err)
		}
	}
}

func TestTrustedComposeSource(t *testing.T) {
	p := "/home/operator/deployments/app/compose.yaml"
	if err := ValidateTrustedSource(hostFixture(), p, 1001); err != nil {
		t.Fatal(err)
	}
	rootOwned := hostFixture()
	rootOwned[p] = Entry{Mode: 0o600, UID: 0}
	if err := ValidateTrustedSource(rootOwned, p, 1001); err != nil {
		t.Fatalf("trusted root-owned source: %v", err)
	}
	checks := []struct {
		name   string
		change func(fakeFS)
	}{
		{"group-writable-parent", func(f fakeFS) { f["/home/operator/deployments"] = Entry{Mode: fs.ModeDir | 0o775, UID: 1001} }},
		{"other-writable-parent", func(f fakeFS) { f["/home/operator"] = Entry{Mode: fs.ModeDir | 0o757, UID: 1001} }},
		{"wrong-file-owner", func(f fakeFS) { f[p] = Entry{Mode: 0o600, UID: 2000} }},
		{"symlink-project", func(f fakeFS) { f["/home/operator/deployments/app"] = Entry{Mode: fs.ModeSymlink | 0o700, UID: 1001} }},
		{"untrusted-parent-owner", func(f fakeFS) { f["/home"] = Entry{Mode: fs.ModeDir | 0o755, UID: 2000} }},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			f := hostFixture()
			tc.change(f)
			if err := ValidateTrustedSource(f, p, 1001); !errors.Is(err, ErrUntrusted) {
				t.Fatalf("expected rejection, got %v", err)
			}
		})
	}
}

func layoutFixture() fakeFS {
	f := hostFixture()
	f["/var"] = Entry{Mode: fs.ModeDir | 0o755}
	f["/var/lib"] = Entry{Mode: fs.ModeDir | 0o755}
	f["/var/lib/docklantern"] = Entry{Mode: fs.ModeDir | 0o700}
	f["/var/lib/docklantern/handoff"] = Entry{Mode: fs.ModeDir | 0o700}
	f["/var/lib/docklantern/handoff/receipts"] = Entry{Mode: fs.ModeDir | 0o700}
	f["/var/lib/docklantern/handoff/proposals"] = Entry{Mode: fs.ModeDir | 0o700, UID: 1002}
	f["/var/lib/docklantern/executor"] = Entry{Mode: fs.ModeDir | 0o700}
	for _, d := range []string{"registrations", "journal", "locks", "overrides"} {
		f["/var/lib/docklantern/executor/"+d] = Entry{Mode: fs.ModeDir | 0o700}
	}
	return f
}

func TestLayoutReadOnlyPolicy(t *testing.T) {
	layout := Layout{Root: "/var/lib/docklantern", MonitorUID: 1002}
	if err := layout.Validate(layoutFixture()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(fakeFS)
	}{
		{"root-writable", func(f fakeFS) { f["/var/lib/docklantern"] = Entry{Mode: fs.ModeDir | 0o777} }},
		{"monitor-root", func(f fakeFS) { f["/var/lib/docklantern/handoff/proposals"] = Entry{Mode: fs.ModeDir | 0o700} }},
		{"monitor-private-owner", func(f fakeFS) {
			f["/var/lib/docklantern/executor/journal"] = Entry{Mode: fs.ModeDir | 0o700, UID: 1002}
		}},
		{"private-readable", func(f fakeFS) {
			f["/var/lib/docklantern/executor/journal"] = Entry{Mode: fs.ModeDir | 0o755}
		}},
		{"receipt-monitor-writable", func(f fakeFS) { f["/var/lib/docklantern/handoff/receipts"] = Entry{Mode: fs.ModeDir | 0o770} }},
		{"proposal-symlink", func(f fakeFS) {
			f["/var/lib/docklantern/handoff/proposals"] = Entry{Mode: fs.ModeSymlink | 0o700, UID: 1002}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := layoutFixture()
			tc.mutate(f)
			if err := layout.Validate(f); !errors.Is(err, ErrUntrusted) {
				t.Fatalf("expected rejection: %v", err)
			}
		})
	}
	if err := (Layout{Root: "/var/lib/docklantern", MonitorUID: 0}).Validate(layoutFixture()); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("root monitor should be rejected: %v", err)
	}
}

func TestEnvironmentNeverUsesInheritedDockerConfig(t *testing.T) {
	poisoned := []string{
		"PATH=/tmp/attacker", "HOME=/tmp/attacker", "DOCKER_HOST=tcp://evil:2375",
		"DOCKER_CONTEXT=remote", "DOCKER_CONFIG=/tmp/leak", "COMPOSE_FILE=/tmp/evil.yaml",
		"COMPOSE_PROFILES=admin", "COMPOSE_PROJECT_NAME=other", "COMPOSE_ENV_FILES=/tmp/evil.env",
		"DOCKER_TLS_VERIFY=1", "LD_PRELOAD=/tmp/libevil.so", "DOCKER_CERT_PATH=/tmp/cert",
	}
	got := MinimalChildEnvironment(poisoned)
	if !reflect.DeepEqual(got, MinimalChildEnvironment(nil)) {
		t.Fatal("inherited variables affected allowlist")
	}
	joined := "\n" + strings.Join(got, "\n") + "\n"
	for _, forbidden := range []string{"/tmp", "DOCKER_CONTEXT=", "COMPOSE_FILE=", "LD_PRELOAD="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsanitized env: %s", forbidden)
		}
	}
	if !strings.Contains(joined, "DOCKER_HOST=unix:///var/run/docker.sock") {
		t.Fatal("missing fixed local socket")
	}
}
