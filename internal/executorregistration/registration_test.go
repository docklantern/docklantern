package executorregistration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/docklantern/docklantern/internal/executorhost"
)

func uid(x uint32) *uint32 { return &x }

func fixture() Registration {
	return Registration{
		SchemaVersion: SchemaVersion, RegistrationID: "hello-go-demo", ProjectName: "hello-go",
		ProjectDirectory: "/srv/deployments/hello-go",
		ComposeFiles: []string{"/srv/deployments/hello-go/docker-compose.yaml"},
		EnvFiles: []string{"/srv/deployments/hello-go/.env"}, Profiles: []string{},
		TrustedSourceUID: uid(1001),
		Services: []Service{{Name: "web", DeploymentEnabled: true,
			ApprovedRepository: "docker.io/example/hello-go",
			Platform: Platform{OS: "linux", Architecture: "amd64"}}},
		FeaturePolicy: FeaturePolicyPrebuilt,
	}
}

func TestParseAndExactAllowlist(t *testing.T) {
	data, err := json.Marshal(fixture())
	if err != nil { t.Fatal(err) }
	r, err := Parse(data)
	if err != nil || !r.AllowsTarget("web", "docker.io/example/hello-go", Platform{"linux", "amd64"}) {
		t.Fatalf("protected fixture invalid: %v", err)
	}
	for _, candidate := range []struct{ name, repo string; platform Platform }{
		{"worker", "docker.io/example/hello-go", Platform{"linux", "amd64"}},
		{"web", "docker.io/other/hello-go", Platform{"linux", "amd64"}},
		{"web", "docker.io/example/hello-go", Platform{"linux", "arm64"}},
	} {
		if r.AllowsTarget(candidate.name, candidate.repo, candidate.platform) {
			t.Fatalf("unregistered target accepted: %+v", candidate)
		}
	}
}

func TestInvalidRegistrationPolicies(t *testing.T) {
	cases := []struct{ name string; change func(*Registration) }{
		{"schema", func(r *Registration) { r.SchemaVersion = 2 }},
		{"missing-trusted-owner", func(r *Registration) { r.TrustedSourceUID = nil }},
		{"identifier-traversal", func(r *Registration) { r.RegistrationID = "../escape" }},
		{"project-not-canonical", func(r *Registration) { r.ProjectDirectory = "/srv/../etc" }},
		{"missing-compose", func(r *Registration) { r.ComposeFiles = nil }},
		{"sibling-file", func(r *Registration) { r.ComposeFiles[0] = "/srv/deployments/hello-go-old/compose.yaml" }},
		{"file-traversal", func(r *Registration) { r.EnvFiles[0] = "/srv/deployments/hello-go/../secret.env" }},
		{"duplicate-file", func(r *Registration) { r.ComposeFiles = append(r.ComposeFiles, r.ComposeFiles[0]) }},
		{"duplicate-across-file-groups", func(r *Registration) { r.EnvFiles[0] = r.ComposeFiles[0] }},
		{"duplicate-profile", func(r *Registration) { r.Profiles = []string{"prod", "prod"} }},
		{"profile-traversal", func(r *Registration) { r.Profiles = []string{"../prod"} }},
		{"duplicate-service", func(r *Registration) { r.Services = append(r.Services, r.Services[0]) }},
		{"nothing-enabled", func(r *Registration) { r.Services[0].DeploymentEnabled = false }},
		{"disabled-running-only", func(r *Registration) { r.Services[0].DeploymentEnabled = false; r.Services[0].AllowRunningOnly = true }},
		{"floating-tag", func(r *Registration) { r.Services[0].ApprovedRepository += ":latest" }},
		{"digest-in-repository", func(r *Registration) { r.Services[0].ApprovedRepository += "@sha256:abcd" }},
		{"unapproved-registry", func(r *Registration) { r.Services[0].ApprovedRepository = "evil.example/hello-go" }},
		{"uppercase-repository", func(r *Registration) { r.Services[0].ApprovedRepository = "docker.io/Example/hello-go" }},
		{"unsupported-platform", func(r *Registration) { r.Services[0].Platform.Architecture = "arm64" }},
		{"unsupported-policy", func(r *Registration) { r.FeaturePolicy = "build-everything" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := fixture()
			tc.change(&r)
			if err := r.Validate(); !errors.Is(err, ErrInvalidRegistration) {
				t.Fatalf("unsafe registration accepted: %v", err)
			}
		})
	}
}

func TestStrictJSONRejectsDuplicatesUnknownTrailingAndOversized(t *testing.T) {
	good, err := json.Marshal(fixture())
	if err != nil { t.Fatal(err) }
	cases := [][]byte{
		nil, []byte("null"), []byte("{}"), []byte("[]"),
		append(append([]byte{}, good...), []byte(" {}")...),
		bytes.Replace(good, []byte(`"feature_policy"`), []byte(`"unknown":"no","feature_policy"`), 1),
		bytes.Replace(good, []byte(`"project_name"`), []byte(`"project_name":"other","project_name"`), 1),
		bytes.Replace(good, []byte(`"schema_version"`), []byte(`"schema_version":1,"\u0073chema_version"`), 1),
		bytes.Replace(good, []byte(`"architecture"`), []byte(`"architecture":"arm64","architecture"`), 1),
		bytes.Repeat([]byte("X"), MaxRegistrationBytes+1),
	}
	for i, payload := range cases {
		if _, err := Parse(payload); !errors.Is(err, ErrInvalidRegistration) {
			t.Errorf("case %d unsafe JSON accepted: %v", i, err)
		}
	}
}

type fakeFS map[string]executorhost.Entry
func (f fakeFS) Lstat(path string) (executorhost.Entry, error) {
	v, ok := f[path]
	if !ok { return executorhost.Entry{}, fs.ErrNotExist }
	return v, nil
}
func trustedSource() fakeFS {
	return fakeFS{
		"/srv": {UID: 0, Mode: fs.ModeDir | 0o755},
		"/srv/deployments": {UID: 1001, Mode: fs.ModeDir | 0o750},
		"/srv/deployments/hello-go": {UID: 1001, Mode: fs.ModeDir | 0o700},
		"/srv/deployments/hello-go/docker-compose.yaml": {UID: 1001, Mode: 0o600},
		"/srv/deployments/hello-go/.env": {UID: 1001, Mode: 0o600},
	}
}
func TestRegistrationSourceTrustChecks(t *testing.T) {
	r := fixture()
	if err := r.ValidateTrustedSources(trustedSource()); err != nil { t.Fatal(err) }
	cases := []struct{ name, path string; entry executorhost.Entry }{
		{"group-writable-parent", "/srv/deployments", executorhost.Entry{UID: 1001, Mode: fs.ModeDir | 0o775}},
		{"foreign-owner", "/srv/deployments/hello-go/docker-compose.yaml", executorhost.Entry{UID: 9999, Mode: 0o600}},
		{"symlink-env", "/srv/deployments/hello-go/.env", executorhost.Entry{UID: 1001, Mode: fs.ModeSymlink | 0o777}},
		{"world-writable-root", "/srv", executorhost.Entry{UID: 0, Mode: fs.ModeDir | 0o777}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := trustedSource(); fs[tc.path] = tc.entry
			if err := r.ValidateTrustedSources(fs); !errors.Is(err, ErrInvalidRegistration) {
				t.Fatalf("untrusted source accepted: %v", err)
			}
		})
	}
}

func TestRegistrationLimits(t *testing.T) {
	r := fixture()
	r.Services[0].AllowRunningOnly = true
	if err := r.Validate(); err != nil { t.Fatalf("explicit running-only consent must be permitted: %v", err) }
	for i := 0; i < MaxComposeFiles; i++ {
		r.ComposeFiles = append(r.ComposeFiles, "/srv/deployments/hello-go/extra-"+strings.Repeat("q", i+1)+".yaml")
	}
	if err := r.Validate(); !errors.Is(err, ErrInvalidRegistration) { t.Fatal("overlarge file list accepted") }
}

func FuzzRegistrationParse(f *testing.F) {
	seed, _ := json.Marshal(fixture())
	f.Add(seed)
	f.Add([]byte(`{"schema_version":1,"schema_version":2}`))
	f.Add([]byte(`{"unknown":"\u0000"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MaxRegistrationBytes*2 { t.Skip() }
		r, err := Parse(b)
		if err == nil && r.Validate() != nil { t.Fatal("parser accepted invalid registration") }
	})
}
