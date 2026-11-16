package management

import (
    "context"
    "strings"
    "testing"

    "github.com/docklantern/docklantern/internal/store"
)

// The local Docker image configuration ID must never be labeled as the
// remote OCI manifest digest or inferred from a monitoring watch tag.
func TestServiceDetailsSeparatesLocalImageIDAndManifestEvidence(t *testing.T) {
    h, db, _, _ := setup(t)
    containerID := strings.Repeat("a", 64)
    imageID := "sha256:" + strings.Repeat("b", 64)
    err := db.Snapshot(context.Background(), []store.Container{{
        ID: containerID, Name: "demo-web-1",
        Project: "demo", Service: "web",
        Image: "docker.io/example/hello-go@sha256:" + strings.Repeat("c", 64),
        ImageID: imageID,
        State: "running", ImageOS: "linux", ImageArchitecture: "amd64",
    }})
    if err != nil { t.Fatal(err) }
    page := request(h, "GET", "/services/demo/web", "", true)
    if page.Code != 200 { t.Fatalf("status %d", page.Code) }
    html := page.Body.String()
    for _, evidence := range []string{
        "Observed container ID", containerID,
        "Local Docker image configuration ID", imageID,
        "Local configuration digest, not the OCI manifest/index digest.",
        "Deployed digest",
    } {
        if !strings.Contains(html, evidence) {
            t.Errorf("missing independently labeled image evidence %q", evidence)
        }
    }
}

func TestMultipleContainerServiceDoesNotSelectSingleImageID(t *testing.T) {
    h, db, _, _ := setup(t)
    err := db.Snapshot(context.Background(), []store.Container{
        {ID: "one", Project: "demo", Service: "web", Image: "nginx:1", ImageID: "sha256:"+strings.Repeat("a",64), State: "running"},
        {ID: "two", Project: "demo", Service: "web", Image: "nginx:2", ImageID: "sha256:"+strings.Repeat("b",64), State: "running"},
    })
    if err != nil { t.Fatal(err) }
    page := request(h, "GET", "/services/demo/web", "", true)
    if page.Code != 200 { t.Fatalf("status %d", page.Code) }
    if !strings.Contains(page.Body.String(), "Unknown or multiple containers") {
        t.Fatal("multi-container service falsely displayed one actual image")
    }
}
