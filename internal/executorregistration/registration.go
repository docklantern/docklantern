// Package executorregistration models protected, host-administrator-approved
// deployment registrations. Monitoring enrollments and proposal data are never
// registration authority. Phase 2a adds no Docker operations or CLI writes.
package executorregistration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/docklantern/docklantern/internal/executorhost"
)

const (
	SchemaVersion         = 1
	MaxRegistrationBytes  = 16 * 1024
	MaxComposeFiles       = 8
	MaxEnvironmentFiles   = 8
	MaxProfiles           = 8
	MaxServices           = 8
	FeaturePolicyPrebuilt = "single-service-prebuilt-v1"
)

var ErrInvalidRegistration = errors.New("invalid executor registration")

// Registration is independent executor-owned authority, NOT a monitoring
// registration. The trusted UID is an explicit host administrator decision.
// Only one service may be selected for a future operation at a time.
type Registration struct {
	SchemaVersion    int       `json:"schema_version"`
	RegistrationID   string    `json:"registration_id"`
	ProjectName      string    `json:"project_name"`
	ProjectDirectory string    `json:"project_directory"`
	ComposeFiles     []string  `json:"compose_files"`
	EnvFiles         []string  `json:"env_files"`
	Profiles         []string  `json:"profiles"`
	TrustedSourceUID *uint32   `json:"trusted_source_uid"`
	Services         []Service `json:"services"`
	FeaturePolicy    string    `json:"feature_policy"`
}

type Service struct {
	Name               string   `json:"name"`
	DeploymentEnabled  bool     `json:"deployment_enabled"`
	ApprovedRepository string   `json:"approved_repository"`
	Platform           Platform `json:"platform"`
	AllowRunningOnly   bool     `json:"allow_running_only"`
}

type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var repositoryComponentRE = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

// ValidID constrains an opaque identifier that can later be used for a
// descriptor-relative filename. Never accept a filename or relative path.
func ValidID(id string) bool { return nameRE.MatchString(id) }

// ValidRepository exposes the strict initial public Docker Hub repository
// policy to the untrusted proposal parser. It does not grant deployment rights.
func ValidRepository(repository string) bool { return validRepository(repository) }

func validRepository(repository string) bool {
	// First beta.6 host executor supports only public Docker Hub repositories
	// explicitly approved in an executor-owned record. Other registries can be
	// enabled by a separately reviewed policy change; no credentials here.
	const prefix = "docker.io/"
	if !strings.HasPrefix(repository, prefix) || len(repository) > 230 {
		return false
	}
	components := strings.Split(strings.TrimPrefix(repository, prefix), "/")
	if len(components) != 2 { // namespace/repository, never implicit library aliases
		return false
	}
	for _, component := range components {
		if !repositoryComponentRE.MatchString(component) {
			return false
		}
	}
	return true
}

func safeAbsolutePath(path string) bool {
	return len(path) <= 512 && filepath.IsAbs(path) && path != "/" && filepath.Clean(path) == path && !strings.ContainsRune(path, rune(0))
}

func withinProject(project, file string) bool {
	if !safeAbsolutePath(file) {
		return false
	}
	rel, err := filepath.Rel(project, file)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, "../")
}

func distinctPaths(files []string, project string, max int) bool {
	if len(files) > max {
		return false
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if !withinProject(project, file) {
			return false
		}
		if _, exists := seen[file]; exists {
			return false
		}
		seen[file] = struct{}{}
	}
	return true
}

// Validate checks schema and allowlisted field values but does NOT validate
// host filesystem ownership, Compose semantics, platform OCI digests, or the
// running Docker state. All are mandatory independent future gates.
func (r Registration) Validate() error {
	if r.SchemaVersion != SchemaVersion || !ValidID(r.RegistrationID) || !ValidID(r.ProjectName) ||
		!safeAbsolutePath(r.ProjectDirectory) || r.TrustedSourceUID == nil || r.FeaturePolicy != FeaturePolicyPrebuilt {
		return ErrInvalidRegistration
	}
	if len(r.ComposeFiles) == 0 || !distinctPaths(r.ComposeFiles, r.ProjectDirectory, MaxComposeFiles) ||
		!distinctPaths(r.EnvFiles, r.ProjectDirectory, MaxEnvironmentFiles) ||
		len(r.Profiles) > MaxProfiles || len(r.Services) == 0 || len(r.Services) > MaxServices {
		return ErrInvalidRegistration
	}
	allPaths := make(map[string]bool)
	for _, p := range append(append([]string(nil), r.ComposeFiles...), r.EnvFiles...) {
		if allPaths[p] {
			return ErrInvalidRegistration
		}
		allPaths[p] = true
	}
	seenProfiles := make(map[string]bool)
	for _, name := range r.Profiles {
		if !ValidID(name) || seenProfiles[name] {
			return ErrInvalidRegistration
		}
		seenProfiles[name] = true
	}
	seenServices := make(map[string]bool)
	enabled := 0
	for _, service := range r.Services {
		if !ValidID(service.Name) || seenServices[service.Name] ||
			!validRepository(service.ApprovedRepository) || service.Platform.OS != "linux" ||
			service.Platform.Architecture != "amd64" ||
			(!service.DeploymentEnabled && service.AllowRunningOnly) {
			return ErrInvalidRegistration
		}
		seenServices[service.Name] = true
		if service.DeploymentEnabled {
			enabled++
		}
	}
	if enabled == 0 {
		return ErrInvalidRegistration
	}
	return nil
}

// AllowsTarget checks exactly one explicit service/repository/platform match.
// This helper is NOT enough to authorize execution; source, OCI, Compose,
// interactive approval, and fresh Docker state must also pass later gates.
func (r Registration) AllowsTarget(service, repository string, platform Platform) bool {
	if r.Validate() != nil {
		return false
	}
	for _, allowed := range r.Services {
		if allowed.Name == service && allowed.DeploymentEnabled && allowed.ApprovedRepository == repository && allowed.Platform == platform {
			return true
		}
	}
	return false
}

// ValidateTrustedSources rechecks every project/source path against the
// explicitly recorded source-owner UID. It is a preflight check, not a stable
// Compose snapshot or defense against a trusted operator changing the files.
func (r Registration) ValidateTrustedSources(reader executorhost.Reader) error {
	if r.Validate() != nil || reader == nil {
		return ErrInvalidRegistration
	}
	if err := executorhost.ValidateTrustedSourceDirectory(reader, r.ProjectDirectory, *r.TrustedSourceUID); err != nil {
		return fmt.Errorf("%w: untrusted source directory", ErrInvalidRegistration)
	}
	for _, source := range append(append([]string(nil), r.ComposeFiles...), r.EnvFiles...) {
		if err := executorhost.ValidateTrustedSource(reader, source, *r.TrustedSourceUID); err != nil {
			return fmt.Errorf("%w: untrusted source file", ErrInvalidRegistration)
		}
	}
	return nil
}

// Parse accepts bounded JSON only, rejects all duplicate keys including
// nested keys (the standard decoder otherwise accepts last-key-wins),
// disallows unknown members, and refuses trailing JSON. It never executes.
func Parse(data []byte) (Registration, error) {
	var r Registration
	if len(data) == 0 || len(data) > MaxRegistrationBytes {
		return r, ErrInvalidRegistration
	}
	if err := rejectDuplicates(data); err != nil {
		return r, ErrInvalidRegistration
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r); err != nil {
		return Registration{}, ErrInvalidRegistration
	}
	var other any
	if err := decoder.Decode(&other); err != io.EOF {
		return Registration{}, ErrInvalidRegistration
	}
	if err := r.Validate(); err != nil {
		return Registration{}, ErrInvalidRegistration
	}
	return r, nil
}

// Parse's preliminary token walk catches duplicate keys in objects at any
// depth, including escaped spellings of the same JSON name. Depth and item
// limits are defensive even though the full input is already capped at 16KiB.
func rejectDuplicates(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if err := walkJSON(decoder, 0, &count); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalidRegistration
	}
	return nil
}

func walkJSON(dec *json.Decoder, depth int, count *int) error {
	if depth > 16 || *count > 1024 {
		return ErrInvalidRegistration
	}
	(*count)++
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return ErrInvalidRegistration
			}
			if _, exists := keys[key]; exists {
				return ErrInvalidRegistration
			}
			keys[key] = struct{}{}
			if err := walkJSON(dec, depth+1, count); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := walkJSON(dec, depth+1, count); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidRegistration
	}
	closing, err := dec.Token()
	if err != nil || closing != json.Delim('}') && delim == '{' || closing != json.Delim(']') && delim == '[' {
		return ErrInvalidRegistration
	}
	return nil
}
