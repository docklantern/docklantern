package executoroverride

import (
    "bytes"
    "strings"
    "testing"

    "github.com/docklantern/docklantern/internal/executorregistration"
)

func fixture() executorregistration.Registration {
    uid := uint32(1001)
    return executorregistration.Registration{
        SchemaVersion: executorregistration.SchemaVersion,
        RegistrationID: "hello-go-demo",
        ProjectName: "hello-go",
        ProjectDirectory: "/srv/hello-go",
        ComposeFiles: []string{"/srv/hello-go/compose.yaml"},
        EnvFiles: []string{"/srv/hello-go/.env"},
        TrustedSourceUID: &uid,
        FeaturePolicy: executorregistration.FeaturePolicyPrebuilt,
        Services: []executorregistration.Service{{
            Name: "web", DeploymentEnabled: true,
            ApprovedRepository: "docker.io/example/hello-go",
            Platform: executorregistration.Platform{OS: "linux", Architecture: "amd64"},
        }},
    }
}

func TestRenderIsScopedAndDeterministic(t *testing.T) {
    reg := fixture()
    digest := "sha256:" + strings.Repeat("a", 64)
    first, err := Render(reg, "web", "docker.io/example/hello-go", digest)
    if err != nil { t.Fatal(err) }
    want := "{\"services\":{\"web\":{\"image\":\"docker.io/example/hello-go@" + digest + "\"}}}"
    if string(first) != want { t.Fatalf("unexpected override: %s", first) }
    again, err := Render(reg, "web", "docker.io/example/hello-go", digest)
    if err != nil || !bytes.Equal(first, again) { t.Fatal("override is not deterministic") }
    if err := VerifyExact(first, reg, "web", "docker.io/example/hello-go", digest); err != nil { t.Fatal(err) }
    for _, altered := range [][]byte{
        append(append([]byte{}, first...), '\n'),
        []byte(strings.Replace(string(first), "web", "other", 1)),
        []byte(strings.Replace(string(first), "a", "b", 1)),
        []byte(strings.Replace(string(first), "}}}", ",\"privileged\":true}}}", 1)),
        []byte("{\"services\":{\"web\":{\"image\":\"invalid\"}}}"),
    } {
        if VerifyExact(altered, reg, "web", "docker.io/example/hello-go", digest) == nil {
            t.Fatalf("accepted unverified Compose data: %s", altered)
        }
    }
}

func TestRenderRefusesUnauthorizedIdentity(t *testing.T) {
    reg := fixture()
    digest := "sha256:" + strings.Repeat("1", 64)
    attempts := []struct{ service, repository, digest string }{
        {"worker", "docker.io/example/hello-go", digest},
        {"web", "docker.io/other/hello-go", digest},
        {"web", "docker.io/example/hello-go", "latest"},
        {"web", "docker.io/example/hello-go", "sha256:" + strings.Repeat("g", 64)},
    }
    for _, attempt := range attempts {
        if _, err := Render(reg, attempt.service, attempt.repository, attempt.digest); err == nil {
            t.Fatalf("accepted unauthorized image: %+v", attempt)
        }
    }
}

func TestDefaultFileContractRequiresOneConventionalBase(t *testing.T) {
    reg := fixture()
    if err := DefaultFileContract(reg); err != nil { t.Fatal(err) }
    reg.ComposeFiles = []string{"/srv/hello-go/docker-compose.yaml"}
    if DefaultFileContract(reg) == nil { t.Fatal("legacy filename not explicitly qualified") }
    reg.ComposeFiles = []string{"/srv/hello-go/compose.yaml", "/srv/hello-go/compose.prod.yaml"}
    if DefaultFileContract(reg) == nil { t.Fatal("layered project incorrectly qualified") }
}
