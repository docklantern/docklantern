package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/docklantern/docklantern/internal/registry"
)

// explicitWatchReference requires an explicitly tagged, normalized OCI
// reference. registry.Parse supplies Docker Hub canonicalization; checking
// the final path segment prevents accidentally treating a missing tag as
// operator intent to monitor :latest.
func explicitWatchReference(raw string) (registry.Reference, error) {
    if raw == "" || strings.TrimSpace(raw) != raw {
        return registry.Reference{}, errors.New("watch target must be a tagged image reference")
    }
    last := raw[strings.LastIndex(raw, "/")+1:]
    if !strings.Contains(last, ":") {
        return registry.Reference{}, errors.New("watch target must include an explicit tag")
    }
    ref, err := registry.Parse(raw)
    if err != nil {
        return registry.Reference{}, errors.New("invalid tagged watch image reference")
    }
    return ref, nil
}

const watchRepositoryConflict = "watched image no longer matches Docker image repository"

func sameImageRepository(base string, requested registry.Reference) bool {
    source, err := registry.Parse(base)
    return err == nil && source.Registry == requested.Registry &&
        source.Repository == requested.Repository
}

// SetServiceWatchOverride changes only the *registry watch target* for an
// already-enrolled, validated, non-legacy Compose service. nil restores the
// Docker-observed image reference; an empty string is invalid. This operation
// never changes a Compose file, image pull, container, platform or enrollment.
//
// The watch must stay in the discovered image's canonical registry/repository
// until explicit cross-repository credential and security semantics exist.
// Reconciliation preserves this preference across a same-repository Compose
// tag change and disables checks if the discovered repository changes.
func (s *Store) SetServiceWatchOverride(ctx context.Context, project, service string, image *string) error {
    if project == "" || service == "" {
        return errors.New("service identity required")
    }
    var requested registry.Reference
    var err error
    if image != nil {
        requested, err = explicitWatchReference(*image)
        if err != nil {
            return err
        }
    }

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer tx.Rollback()

    var legacy, gui int
    var optIn sql.NullInt64
    var state, validationError string
    var oldOverride sql.NullString
    err = tx.QueryRowContext(ctx, `SELECT legacy_enrolled,gui_enrolled,label_opt_in,discovery_state,validation_error,watch_image_override
        FROM monitor_service_settings WHERE project=? AND service=?`,project,service).
        Scan(&legacy,&gui,&optIn,&state,&validationError,&oldOverride)
    if errors.Is(err,sql.ErrNoRows) {
        return errors.New("service is not discovered or registered")
    }
    if err != nil { return err }
    if legacy != 0 {
        return errors.New("legacy manual watches must be changed through registered monitoring policies")
    }
    // The only recoverable conflict is one created by our own watch-tag
    // repository guard after otherwise-valid Docker discovery. Do not
    // override replica/label/platform validation errors. A reset is also
    // allowed while a previously GUI-enrolled service is disabled: it only
    // clears a preference and cannot enable any network checks.
    recovering := state == "conflict" && validationError == watchRepositoryConflict && oldOverride.Valid
    enrolled := gui != 0 || (optIn.Valid && optIn.Int64 == 1)
    if (state != "present" && !recovering) ||
        (optIn.Valid && optIn.Int64 == 0) ||
        (!enrolled && (image != nil || !oldOverride.Valid)) {
        return errors.New("service is not eligible to change watched image")
    }

    var previous, platform string
    err = tx.QueryRowContext(ctx,`SELECT image,platform FROM monitor_policies
        WHERE project=? AND service=?`,project,service).Scan(&previous,&platform)
    if errors.Is(err,sql.ErrNoRows) { return errors.New("monitoring policy missing") }
    if err != nil { return err }
    if !validPlatform(platform) { return errors.New("unverified monitoring platform") }

    // Only the last successful discovery snapshot is consulted. Require a
    // running image with verified metadata and matching platform.
    var dockerImage, osName, architecture, variant, runState string
    err = tx.QueryRowContext(ctx,`SELECT image,image_os,image_architecture,image_variant,state
        FROM inventory WHERE project=? AND service=? AND state='running'
        ORDER BY container_id LIMIT 1`,project,service).
        Scan(&dockerImage,&osName,&architecture,&variant,&runState)
    if err != nil { return errors.New("verified running Docker image unavailable") }
    source := parseMonitorCandidate(Container{
        Project:project, Service:service, Image:dockerImage,
        ImageOS:osName, ImageArchitecture:architecture, ImageVariant:variant,
        State:runState,
    })
    if source.issue != "" || !source.running ||
        (source.platform != platform && !recovering) {
        return errors.New("unverified Docker image or platform")
    }

    selected := source.image
    var stored any // NULL means "inherit Docker image tag"
    if image != nil {
        if !sameImageRepository(source.image, requested) {
            return errors.New("watch target must use the deployed image repository")
        }
        selected = requested.Key()
        if selected != source.image { stored = selected }
    }
    // Avoid invalidating scheduled/in-flight work for an idempotent request.
    unchanged := (stored == nil && !oldOverride.Valid) ||
        (stored != nil && oldOverride.Valid && stored == oldOverride.String)
    if unchanged && previous == selected && platform == source.platform && !recovering {
        return tx.Commit()
    }

    if recovering {
        // The prior successful snapshot already verified this service's
        // image/platform and the sole conflict was the obsolete watch.
        // Resolve it atomically with the replacement target; no Docker call.
        _,err=tx.ExecContext(ctx,`UPDATE monitor_service_settings SET
            watch_image_override=?,discovery_state='present',validation_error=''
            WHERE project=? AND service=?`,stored,project,service)
    } else {
        _,err=tx.ExecContext(ctx,`UPDATE monitor_service_settings SET watch_image_override=?
            WHERE project=? AND service=?`,stored,project,service)
    }
    if err!=nil{return err}
    stamp:=time.Now().UTC().Format(time.RFC3339)
    // Changing image identity invalidates in-flight generation/CAS, resets
    // only *per-service* attempts/backoff and schedules a new observation.
    // Never delete image-scoped registry_checks or monitor_events: other
    // services may watch the same reference/platform.
    _,err=tx.ExecContext(ctx,`UPDATE monitor_policies SET image=?,platform=?,
        next_check_at=CASE WHEN image!=? OR platform!=? THEN ? ELSE next_check_at END,
        last_checked_at=CASE WHEN image!=? OR platform!=? THEN '' ELSE last_checked_at END,
        last_error=CASE WHEN image!=? OR platform!=? THEN '' ELSE last_error END,
        consecutive_failures=CASE WHEN image!=? OR platform!=? THEN 0 ELSE consecutive_failures END,
        generation=generation+1
        WHERE project=? AND service=?`,
        selected,source.platform,selected,source.platform,stamp,
        selected,source.platform,selected,source.platform,
        selected,source.platform,project,service)
    if err!=nil{return fmt.Errorf("change watched image: %w",err)}
    return tx.Commit()
}
