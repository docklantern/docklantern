// Package executoroverride renders a narrowly scoped, immutable Compose image
// override. It provides no filesystem, Docker, or authorization capability.
package executoroverride

import (
    "bytes"
    "encoding/json"
    "errors"
    "path/filepath"

    "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrInvalidOverride = errors.New("unsupported persistent Compose image override")

// Render returns only a selected, independently registered service image.
// The exact same bytes may be supplied to Compose via stdin or verified
// against an eventual durable override file. Neither operation authorizes
// applying the image.
func Render(reg executorregistration.Registration, service, repository, digest string) ([]byte, error) {
    platform := executorregistration.Platform{OS: "linux", Architecture: "amd64"}
    if !reg.AllowsTarget(service, repository, platform) || !validDigest(digest) {
        return nil, ErrInvalidOverride
    }
    data, err := json.Marshal(struct {
        Services map[string]struct {
            Image string `json:"image"`
        } `json:"services"`
    }{
        Services: map[string]struct { Image string `json:"image"` }{
            service: {Image: repository + "@" + digest},
        },
    })
    if err != nil || len(data) == 0 || len(data) > 1024 {
        return nil, ErrInvalidOverride
    }
    return data, nil
}

// VerifyExact rejects added Compose keys, another service, altered digests,
// syntactic variants and trailing data. It is deliberately byte-exact so
// persistent state cannot silently diverge from the executor's expectation.
func VerifyExact(data []byte, reg executorregistration.Registration, service, repository, digest string) error {
    expected, err := Render(reg, service, repository, digest)
    if err != nil || !bytes.Equal(data, expected) {
        return ErrInvalidOverride
    }
    return nil
}

// DefaultFileContract defines the first opt-in persistence compatibility
// subset: exactly one standard base file, no explicit Compose file layering.
// Default docker compose commands would read compose.override.yaml after
// compose.yaml. Explicit -f invocations MUST append the managed override,
// and are not covered by this check. This predicate does not examine
// filesystem trust, an existing override, or operator authority.
func DefaultFileContract(reg executorregistration.Registration) error {
    if reg.Validate() != nil || len(reg.ComposeFiles) != 1 ||
        reg.ComposeFiles[0] != filepath.Join(reg.ProjectDirectory, "compose.yaml") {
        return ErrInvalidOverride
    }
    return nil
}

func validDigest(d string) bool {
    if len(d) != 71 || d[:7] != "sha256:" { return false }
    for _, ch := range d[7:] {
        if ch < '0' || ch > '9' && ch < 'a' || ch > 'f' {
            return false
        }
    }
    return true
}
