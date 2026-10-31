package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
)

// schemaVersion is deliberately separate from the SQLite application_id.
// beta.2 databases had user_version=0 and previously used ad-hoc column checks.
const schemaVersion = 4

// initializeSchema performs the entire schema upgrade within one SQLite
// transaction. A failed upgrade leaves the old tables/version untouched.
// PRAGMA journal_mode cannot be changed inside a transaction.
func initializeSchema(ctx context.Context, db *sql.DB) error {
    if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
        return err
    }
    if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
        return err
    }
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    var version int
    if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
        return err
    }
    if version > schemaVersion {
        return fmt.Errorf("database schema version %d is newer than supported version %d; use a compatible binary", version, schemaVersion)
    }
    if version < 0 {
        return errors.New("invalid database schema version")
    }

    // Keep the beta.2 schema as-is, including legacy Compose registration
    // records. Label-based monitoring never manufactures registration records.
    statements := []string{
        "CREATE TABLE IF NOT EXISTS registrations(name TEXT PRIMARY KEY,project_dir TEXT NOT NULL,files_json TEXT NOT NULL,env_files_json TEXT NOT NULL,services_json TEXT NOT NULL,profiles_json TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL)",
        "CREATE TABLE IF NOT EXISTS inventory(container_id TEXT PRIMARY KEY,name TEXT NOT NULL,project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,image_id TEXT NOT NULL,state TEXT NOT NULL,health TEXT NOT NULL,observed_at TEXT NOT NULL)",
        "CREATE TABLE IF NOT EXISTS registry_checks(image TEXT NOT NULL,platform TEXT NOT NULL,index_digest TEXT NOT NULL,platform_digest TEXT NOT NULL,previous_digest TEXT NOT NULL,status TEXT NOT NULL,checked_at TEXT NOT NULL,PRIMARY KEY(image,platform))",
        "CREATE TABLE IF NOT EXISTS monitor_events(id INTEGER PRIMARY KEY AUTOINCREMENT,kind TEXT NOT NULL,project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,platform TEXT NOT NULL,previous_digest TEXT NOT NULL,digest TEXT NOT NULL,occurred_at TEXT NOT NULL,delivery_state TEXT NOT NULL,delivery_attempts INTEGER NOT NULL,next_delivery_at TEXT NOT NULL,last_delivery_error TEXT NOT NULL)",
        "CREATE INDEX IF NOT EXISTS idx_monitor_events_due ON monitor_events(delivery_state,next_delivery_at,id)",
        "CREATE TABLE IF NOT EXISTS worker_status(name TEXT PRIMARY KEY,state TEXT NOT NULL,last_started_at TEXT NOT NULL,last_success_at TEXT NOT NULL,last_error_at TEXT NOT NULL,last_error TEXT NOT NULL,last_heartbeat_at TEXT NOT NULL,last_items INTEGER NOT NULL)",
        "CREATE TABLE IF NOT EXISTS notification_settings(id INTEGER PRIMARY KEY CHECK(id=1),digest_changes INTEGER NOT NULL,registry_failures INTEGER NOT NULL,registry_recovery INTEGER NOT NULL,prolonged_outage INTEGER NOT NULL,outage_failures INTEGER NOT NULL,retention_days INTEGER NOT NULL)",
        "INSERT OR IGNORE INTO notification_settings(id,digest_changes,registry_failures,registry_recovery,prolonged_outage,outage_failures,retention_days) VALUES(1,1,1,1,1,3,30)",
        "CREATE TABLE IF NOT EXISTS monitor_policies(project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,platform TEXT NOT NULL,interval_seconds INTEGER NOT NULL,paused INTEGER NOT NULL DEFAULT 0,next_check_at TEXT NOT NULL,last_checked_at TEXT NOT NULL DEFAULT '',last_error TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,service),FOREIGN KEY(project) REFERENCES registrations(name))",
    }
    for _, statement := range statements {
        if _, err := tx.ExecContext(ctx, statement); err != nil {
            return fmt.Errorf("initialize base schema: %w", err)
        }
    }

    // Both pre-beta.2 and beta.2 databases must still upgrade without loss.
    for _, column := range []string{"repo_digests_json", "image_os", "image_architecture", "image_variant"} {
        if err := addColumnIfMissing(ctx, tx, "inventory", column, "TEXT NOT NULL DEFAULT ''"); err != nil {
            return err
        }
    }
    if err := addColumnIfMissing(ctx, tx, "monitor_policies", "consecutive_failures", "INTEGER NOT NULL DEFAULT 0"); err != nil {
        return err
    }

    if version == 0 {
        // No FK to 'registrations': label/GUI monitored services do not need
        // Compose source paths, and monitoring never authorizes deployment.
        // Legacy rows are marked only once, during migration, not on each
        // startup (label-managed rows must never be silently made legacy).
        for _, statement := range []string{
            "CREATE TABLE IF NOT EXISTS monitoring_defaults(id INTEGER PRIMARY KEY CHECK(id=1),interval_seconds INTEGER NOT NULL DEFAULT 300 CHECK(interval_seconds BETWEEN 60 AND 86400))",
            "INSERT OR IGNORE INTO monitoring_defaults(id,interval_seconds) VALUES(1,300)",
            "CREATE TABLE IF NOT EXISTS monitor_service_settings(project TEXT NOT NULL,service TEXT NOT NULL,legacy_enrolled INTEGER NOT NULL DEFAULT 0 CHECK(legacy_enrolled IN (0,1)),gui_enrolled INTEGER NOT NULL DEFAULT 0 CHECK(gui_enrolled IN (0,1)),label_opt_in INTEGER CHECK(label_opt_in IN (0,1)),label_interval_seconds INTEGER CHECK(label_interval_seconds BETWEEN 60 AND 86400),gui_interval_seconds INTEGER CHECK(gui_interval_seconds BETWEEN 60 AND 86400),discovery_state TEXT NOT NULL DEFAULT 'unknown' CHECK(discovery_state IN ('unknown','present','missing','conflict')),last_seen_at TEXT NOT NULL DEFAULT '',validation_error TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,service))",
            "INSERT OR IGNORE INTO monitor_service_settings(project,service,legacy_enrolled) SELECT project,service,1 FROM monitor_policies",
            "PRAGMA user_version=1",
        } {
            if _, err := tx.ExecContext(ctx, statement); err != nil {
                return fmt.Errorf("upgrade monitoring schema: %w", err)
            }
        }
    }

    if version < 2 {
        // Legacy policies reference Compose registrations, which do not exist
        // for label-only monitoring. Preserve every policy row when removing
        // that FK; managed deployments still require independent registration.
        for _, statement := range []string{
            "CREATE TABLE monitor_policies_v2(project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,platform TEXT NOT NULL,interval_seconds INTEGER NOT NULL,paused INTEGER NOT NULL DEFAULT 0,next_check_at TEXT NOT NULL,last_checked_at TEXT NOT NULL DEFAULT '',last_error TEXT NOT NULL DEFAULT '',consecutive_failures INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(project,service))",
            "INSERT INTO monitor_policies_v2 SELECT project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error,consecutive_failures FROM monitor_policies",
            "DROP TABLE monitor_policies",
            "ALTER TABLE monitor_policies_v2 RENAME TO monitor_policies",
            "PRAGMA user_version=2",
        } {
            if _, err := tx.ExecContext(ctx,statement); err != nil {
                return fmt.Errorf("upgrade label monitoring schema: %w",err)
            }
        }
    }

    if version < 3 {
        if err:=addColumnIfMissing(ctx,tx,"monitor_policies","generation",
            "INTEGER NOT NULL DEFAULT 0");err!=nil{return err}
        // Preserve the original interval of manually registered beta.2
        // policies so an administrator can reset a temporary GUI override.
        if err := addColumnIfMissing(ctx,tx,"monitor_service_settings","legacy_interval_seconds",
            "INTEGER CHECK(legacy_interval_seconds BETWEEN 60 AND 86400)"); err!=nil{return err}
        if _,err:=tx.ExecContext(ctx,`UPDATE monitor_service_settings
            SET legacy_interval_seconds=(SELECT interval_seconds FROM monitor_policies p
                WHERE p.project=monitor_service_settings.project AND p.service=monitor_service_settings.service)
            WHERE legacy_enrolled=1 AND legacy_interval_seconds IS NULL`);err!=nil{return err}
        if _,err:=tx.ExecContext(ctx,"PRAGMA user_version=3");err!=nil{return err}
    }

    if version < 4 {
        // A watch-tag override belongs to monitoring preferences, never
        // Compose desired state or the Docker-observed image. NULL inherits
        // the currently discovered image. Pre-existing rows are unchanged.
        if err := addColumnIfMissing(ctx, tx, "monitor_service_settings",
            "watch_image_override", "TEXT"); err != nil {
            return err
        }
        if _, err := tx.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
            return err
        }
    }

    // user_version alone does not prove a complete/correct migration if
    // someone copied or modified a database externally. Fail closed.
    for table, expected := range map[string][]string{
        "monitoring_defaults": {"id", "interval_seconds"},
        "monitor_service_settings": {"project", "service", "legacy_enrolled", "gui_enrolled", "label_opt_in", "label_interval_seconds", "gui_interval_seconds", "discovery_state", "last_seen_at", "validation_error", "legacy_interval_seconds", "watch_image_override"},
    } {
        actual, err := tableColumns(ctx, tx, table)
        if err != nil {
            return err
        }
        for _, name := range expected {
            if !actual[name] {
                return fmt.Errorf("database schema version %d is incomplete: %s.%s missing", schemaVersion, table, name)
            }
        }
    }
    return tx.Commit()
}

// All identifiers below come from constant, internal allowlists, never user
// input. PRAGMA table_info also handles partially migrated older databases.
func addColumnIfMissing(ctx context.Context, tx *sql.Tx, table, column, columnDefinition string) error {
    columns, err := tableColumns(ctx, tx, table)
    if err != nil {
        return err
    }
    if columns[column] {
        return nil
    }
    _, err = tx.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+columnDefinition)
    if err != nil {
        return fmt.Errorf("add column %s.%s: %w", table, column, err)
    }
    return nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
    rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+table+")")
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    out := make(map[string]bool)
    for rows.Next() {
        var id, notNull, primary int
        var name, kind string
        var defaultValue sql.NullString
        if err := rows.Scan(&id, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
            return nil, err
        }
        out[name] = true
    }
    return out, rows.Err()
}
