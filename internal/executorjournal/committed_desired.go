package executorjournal

import (
    "context"
    "database/sql"
    "errors"

    "github.com/docklantern/docklantern/internal/executorregistration"
)

// LatestCommittedDesired reads the protected, last verified image digest for
// a project, excluding generation zero (the original source fingerprint).
// A missing positive generation means this is a new persistent enrollment,
// never that any existing Compose override may be adopted.
func (s *Store) LatestCommittedDesired(ctx context.Context, project string) (string, bool, error) {
    if s == nil || ctx == nil || ctx.Err() != nil ||
        !executorregistration.ValidID(project) {
        return "", false, ErrUnavailable
    }
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.db == nil { return "", false, ErrUnavailable }
    var digest string
    err := s.db.QueryRowContext(ctx,
        "SELECT committed_sha256 FROM override_records WHERE project_id=? AND generation>0 ORDER BY generation DESC LIMIT 1",
        project).Scan(&digest)
    if errors.Is(err, sql.ErrNoRows) { return "", false, nil }
    if err != nil || !validHash(digest) { return "", false, ErrUnavailable }
    return digest, true, nil
}
