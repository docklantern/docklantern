package store

import (
    "context"
    "database/sql"
    "errors"
)

// MonitorServiceSettings tracks *monitoring* provenance and preferences,
// independent of registration for any future Compose deployment authority.
// NULL label settings mean the container did not supply that label.
type MonitorServiceSettings struct {
    Project string
    Service string
    LegacyEnrolled bool
    LegacyIntervalSeconds *int
    GUIEnrolled bool
    LabelOptIn *bool
    LabelIntervalSeconds *int
    GUIIntervalSeconds *int
    DiscoveryState string
    LastSeenAt string
    ValidationError string
}

func (s *Store) DefaultMonitorInterval(ctx context.Context) (int, error) {
    var seconds int
    err := s.db.QueryRowContext(ctx,
        "SELECT interval_seconds FROM monitoring_defaults WHERE id=1").Scan(&seconds)
    return seconds, err
}

// SetDefaultMonitorInterval is used by the future GUI; it does not itself
// enroll any service or change running Docker containers.
func (s *Store) SetDefaultMonitorInterval(ctx context.Context,seconds int)error {
    if err:=intervalRange(seconds);err!=nil{return err}
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
    result,err:=tx.ExecContext(ctx,
        "UPDATE monitoring_defaults SET interval_seconds=? WHERE id=1",seconds)
    if err!=nil{return err}
    affected,err:=result.RowsAffected();if err!=nil{return err}
    if affected!=1{return errors.New("monitoring default row missing")}
    // Only label/GUI-managed watches inheriting the global default change.
    // A legacy/manual policy and explicit label/GUI intervals are untouched.
    stamp:=time.Now().UTC().Format(time.RFC3339)
    _,err=tx.ExecContext(ctx,`UPDATE monitor_policies SET interval_seconds=?,
       next_check_at=CASE WHEN paused=0 AND consecutive_failures=0 THEN ? ELSE next_check_at END,
       generation=generation+1
       WHERE interval_seconds!=? AND EXISTS(
         SELECT 1 FROM monitor_service_settings m
         WHERE m.project=monitor_policies.project AND m.service=monitor_policies.service
           AND m.legacy_enrolled=0 AND m.gui_interval_seconds IS NULL AND m.label_interval_seconds IS NULL)`,
       seconds,stamp,seconds)
    if err!=nil{return err}
    return tx.Commit()
}

func (s *Store) MonitoringServiceSettings(ctx context.Context) ([]MonitorServiceSettings, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT project,service,legacy_enrolled,gui_enrolled,label_opt_in,
            label_interval_seconds,gui_interval_seconds,discovery_state,last_seen_at,validation_error,legacy_interval_seconds
            FROM monitor_service_settings ORDER BY project,service`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    out := []MonitorServiceSettings{}
    for rows.Next() {
        var p MonitorServiceSettings
        var legacy, gui int
        var labelOptIn, labelInterval, guiInterval, legacyInterval sql.NullInt64
        err := rows.Scan(&p.Project,&p.Service,&legacy,&gui,&labelOptIn,
            &labelInterval,&guiInterval,&p.DiscoveryState,&p.LastSeenAt,&p.ValidationError,&legacyInterval)
        if err != nil {
            return nil, err
        }
        p.LegacyEnrolled = legacy == 1
        p.GUIEnrolled = gui == 1
        if labelOptIn.Valid {
            value := labelOptIn.Int64 == 1
            p.LabelOptIn = &value
        }
        if labelInterval.Valid {
            value := int(labelInterval.Int64)
            p.LabelIntervalSeconds = &value
        }
        if guiInterval.Valid {
            value := int(guiInterval.Int64)
            p.GUIIntervalSeconds = &value
        }
        if legacyInterval.Valid {v:=int(legacyInterval.Int64);p.LegacyIntervalSeconds=&v}
        out = append(out,p)
    }
    return out,rows.Err()
}
