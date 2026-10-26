package store

import (
    "context"
    "database/sql"
    "path/filepath"
    "strings"
    "testing"
    "time"

    _ "modernc.org/sqlite"
)

// makeBeta2Fixture creates a version-zero database with realistic data,
// without invoking the current Open/schema creation path. It deliberately
// omits older columns to also exercise pre-existing additive migrations.
func makeBeta2Fixture(t *testing.T, path string) {
    t.Helper()
    db, err := sql.Open("sqlite",path)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    ddl := []string{
        "CREATE TABLE registrations(name TEXT PRIMARY KEY,project_dir TEXT NOT NULL,files_json TEXT NOT NULL,env_files_json TEXT NOT NULL,services_json TEXT NOT NULL,profiles_json TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL)",
        "CREATE TABLE monitor_policies(project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,platform TEXT NOT NULL,interval_seconds INTEGER NOT NULL,paused INTEGER NOT NULL DEFAULT 0,next_check_at TEXT NOT NULL,last_checked_at TEXT NOT NULL DEFAULT '',last_error TEXT NOT NULL DEFAULT '',consecutive_failures INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(project,service),FOREIGN KEY(project) REFERENCES registrations(name))",
        "CREATE TABLE inventory(container_id TEXT PRIMARY KEY,name TEXT NOT NULL,project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,image_id TEXT NOT NULL,state TEXT NOT NULL,health TEXT NOT NULL,observed_at TEXT NOT NULL)",
        "CREATE TABLE registry_checks(image TEXT NOT NULL,platform TEXT NOT NULL,index_digest TEXT NOT NULL,platform_digest TEXT NOT NULL,previous_digest TEXT NOT NULL,status TEXT NOT NULL,checked_at TEXT NOT NULL,PRIMARY KEY(image,platform))",
        "CREATE TABLE monitor_events(id INTEGER PRIMARY KEY AUTOINCREMENT,kind TEXT NOT NULL,project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,platform TEXT NOT NULL,previous_digest TEXT NOT NULL,digest TEXT NOT NULL,occurred_at TEXT NOT NULL,delivery_state TEXT NOT NULL,delivery_attempts INTEGER NOT NULL,next_delivery_at TEXT NOT NULL,last_delivery_error TEXT NOT NULL)",
        "CREATE TABLE notification_settings(id INTEGER PRIMARY KEY CHECK(id=1),digest_changes INTEGER NOT NULL,registry_failures INTEGER NOT NULL,registry_recovery INTEGER NOT NULL,prolonged_outage INTEGER NOT NULL,outage_failures INTEGER NOT NULL,retention_days INTEGER NOT NULL)",
    }
    for _, statement := range ddl {
        if _, err := db.Exec(statement); err != nil { t.Fatalf("fixture schema: %v",err) }
    }
    for _, project := range []string{"hello-js","hello-py"} {
        _,err=db.Exec(`INSERT INTO registrations VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
            project,"/srv/apps/"+project,`["/srv/apps/compose.yaml"]`,"[]",`["web"]`,"[]",0,"2026-10-25T21:00:00Z")
        if err != nil { t.Fatal(err) }
    }
    for _, row := range []struct{
        project, image, next, checked, lastErr string
        interval,paused,failures int
    }{
        {"hello-js","registry-1.docker.io/davidslusser/hello-js:latest","2026-10-25T22:29:00Z","2026-10-25T22:24:00Z","",300,0,0},
        {"hello-py","registry-1.docker.io/davidslusser/hello-py:latest","2026-10-25T22:30:00Z","2026-10-25T22:20:00Z","registry retry 429",600,1,2},
    } {
        _,err=db.Exec(`INSERT INTO monitor_policies(project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error,consecutive_failures)
        VALUES(?, 'web', ?, 'linux/amd64', ?, ?, ?, ?, ?, ?)`,
            row.project,row.image,row.interval,row.paused,row.next,row.checked,row.lastErr,row.failures)
        if err != nil { t.Fatal(err) }
    }
    _,err=db.Exec(`INSERT INTO inventory(container_id,name,project,service,image,image_id,state,health,observed_at)
        VALUES('abc','hello-js-web-1','hello-js','web','davidslusser/hello-js:latest','sha256:old','running','healthy','2026-10-25T22:24:00Z')`)
    if err!=nil {t.Fatal(err)}
    // beta.2 stores only the most recent registry status; the immutable
    // change event is the historical evidence of the preceding transition.
    _,err=db.Exec(`INSERT INTO registry_checks(image,platform,index_digest,platform_digest,previous_digest,status,checked_at)
      VALUES('registry-1.docker.io/davidslusser/hello-js:latest','linux/amd64','sha256:index-b','sha256:next','sha256:old','unchanged','2026-10-25T22:24:00Z')`)
    if err!=nil{t.Fatal(err)}
    _,err=db.Exec(`INSERT INTO monitor_events
        (id,kind,project,service,image,platform,previous_digest,digest,occurred_at,delivery_state,delivery_attempts,next_delivery_at,last_delivery_error)
        VALUES(17,'remote_digest_changed','','','registry-1.docker.io/davidslusser/hello-js:latest',
        'linux/amd64','sha256:old','sha256:next','2026-10-25T22:19:00Z','pending',0,
        '2026-10-25T22:19:00Z','')`)
    if err!=nil{t.Fatal(err)}
    _,err=db.Exec(`INSERT INTO notification_settings VALUES(1,1,0,1,1,5,90)`)
    if err!=nil{t.Fatal(err)}
    // beta.2 and older DBs carry user_version zero.
}

func versionOf(t *testing.T,db *sql.DB) int {
    t.Helper()
    var n int
    if err:=db.QueryRow("PRAGMA user_version").Scan(&n);err!=nil{t.Fatal(err)}
    return n
}

func TestBeta2UpgradePreservesMonitoringData(t *testing.T){
    ctx:=context.Background()
    path:=filepath.Join(t.TempDir(),"beta2.db")
    makeBeta2Fixture(t,path)
    s,err:=Open(path);if err!=nil{t.Fatal(err)}
    verify:=func(){
        t.Helper()
        if v:=versionOf(t,s.db);v!=schemaVersion{t.Fatalf("schema version %d",v)}
        regs,err:=s.Projects(ctx);if err!=nil||len(regs)!=2{t.Fatalf("registrations %+v %v",regs,err)}
        policies,err:=s.MonitorPolicies(ctx);if err!=nil||len(policies)!=2{t.Fatalf("policies %+v %v",policies,err)}
        if policies[0].Project!="hello-js"||policies[0].IntervalSeconds!=300||policies[0].LastCheckedAt!="2026-10-25T22:24:00Z"{t.Fatalf("js policy modified: %+v",policies[0])}
        if policies[1].Project!="hello-py"||!policies[1].Paused||policies[1].ConsecutiveFailures!=2||policies[1].LastError!="registry retry 429"{t.Fatalf("py policy modified: %+v",policies[1])}
        events,err:=s.Events(ctx,10);if err!=nil||len(events)!=1{t.Fatalf("events %+v %v",events,err)}
        if events[0].ID!=17||events[0].DeliveryState!="pending"||events[0].DeliveryAttempts!=0||events[0].PreviousDigest!="sha256:old"{t.Fatalf("event modified: %+v",events[0])}
        checks,err:=s.RegistryChecks(ctx);if err!=nil||len(checks)!=1||checks[0].PlatformDigest!="sha256:next"||checks[0].Status!="unchanged"{t.Fatalf("registry checks %+v %v",checks,err)}
        containers,err:=s.Containers(ctx);if err!=nil||len(containers)!=1||containers[0].ID!="abc"{t.Fatalf("inventory %+v %v",containers,err)}
        prefs,err:=s.NotificationSettings(ctx);if err!=nil||prefs.OutageFailures!=5||prefs.RetentionDays!=90||prefs.RegistryFailures{t.Fatalf("preferences %+v %v",prefs,err)}
        cfg,err:=s.MonitoringServiceSettings(ctx);if err!=nil||len(cfg)!=2{t.Fatalf("provenance %+v %v",cfg,err)}
        for _,p:=range cfg {
            if !p.LegacyEnrolled||p.GUIEnrolled||p.LabelOptIn!=nil||p.GUIIntervalSeconds!=nil||p.LabelIntervalSeconds!=nil||p.DiscoveryState!="unknown"{
                t.Fatalf("legacy policy not preserved: %+v",p)
            }
        }
    }
    verify()
    if interval,err:=s.DefaultMonitorInterval(ctx);err!=nil||interval!=300{t.Fatalf("default %d %v",interval,err)}
    if err:=s.SetDefaultMonitorInterval(ctx,120);err!=nil{t.Fatal(err)}
    // Re-open twice to establish version and backfill idempotence.
    if err:=s.Close();err!=nil{t.Fatal(err)}
    for i:=0;i<2;i++{
        s,err=Open(path);if err!=nil{t.Fatal(err)}
        verify()
        interval,err:=s.DefaultMonitorInterval(ctx)
        if err!=nil||interval!=120{t.Fatalf("default changed after restart: %d %v",interval,err)}
        if err:=s.Close();err!=nil{t.Fatal(err)}
    }
}

func TestFreshSchemaAndNewManualEnrollment(t *testing.T){
    ctx:=context.Background()
    path:=filepath.Join(t.TempDir(),"new.db")
    s,err:=Open(path);if err!=nil{t.Fatal(err)}
    if versionOf(t,s.db)!=schemaVersion{t.Fatal("new database missing schema version")}
    v,err:=s.DefaultMonitorInterval(ctx);if err!=nil||v!=300{t.Fatalf("default %d %v",v,err)}
    for _,invalid:=range []int{0,59,86401}{
        if err:=s.SetDefaultMonitorInterval(ctx,invalid);err==nil{t.Fatalf("accepted %d",invalid)}
    }
    if err:=s.Register(ctx,Registration{Name:"new",ProjectDir:"/srv/new",Files:[]string{"/srv/new/compose.yaml"},Services:[]string{"web"}});err!=nil{t.Fatal(err)}
    if err:=s.SetMonitorPolicy(ctx,MonitorPolicy{Project:"new",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:300});err!=nil{t.Fatal(err)}
    settings,err:=s.MonitoringServiceSettings(ctx)
    if err!=nil||len(settings)!=1||!settings[0].LegacyEnrolled{t.Fatalf("manual enrollment %+v %v",settings,err)}
    // Updating a manual policy must preserve the single metadata record.
    if err:=s.SetMonitorPolicy(ctx,MonitorPolicy{Project:"new",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:600});err!=nil{t.Fatal(err)}
    settings,err=s.MonitoringServiceSettings(ctx)
    if err!=nil||len(settings)!=1{t.Fatalf("duplicate metadata %+v %v",settings,err)}
    _=s.Close()
    s,err=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
    settings,err=s.MonitoringServiceSettings(ctx)
    if err!=nil||len(settings)!=1||!settings[0].LegacyEnrolled{t.Fatalf("lost metadata %+v %v",settings,err)}
}

func TestSchemaVersionGuardDoesNotModifyNewerDatabase(t *testing.T){
    path:=filepath.Join(t.TempDir(),"future.db")
    db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
    if _,err=db.Exec("PRAGMA user_version=99");err!=nil{t.Fatal(err)}
    if _,err=db.Exec("CREATE TABLE sentinel(value TEXT);INSERT INTO sentinel VALUES('keep')");err!=nil{t.Fatal(err)}
    _=db.Close()
    _,err=Open(path)
    if err==nil||!strings.Contains(err.Error(),"newer than supported"){t.Fatalf("expected version guard, got %v",err)}
    db,err=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer db.Close()
    if versionOf(t,db)!=99{t.Fatal("changed future database version")}
    var value string
    if err=db.QueryRow("SELECT value FROM sentinel").Scan(&value);err!=nil||value!="keep"{t.Fatalf("future DB modified: %q %v",value,err)}
}

func TestUpgradeFailureRollsBackTransaction(t *testing.T){
    path:=filepath.Join(t.TempDir(),"failed.db")
    makeBeta2Fixture(t,path)
    db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
    // Wrong pre-existing table shape fails the v1 INSERT. No partial
    // ALTER TABLE, version bump, or lost beta.2 rows may be committed.
    if _,err=db.Exec("CREATE TABLE monitoring_defaults(id INTEGER PRIMARY KEY)");err!=nil{t.Fatal(err)}
    _=db.Close()
    _,err=Open(path)
    if err==nil{t.Fatal("expected migration to fail")}
    db,err=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer db.Close()
    if versionOf(t,db)!=0{t.Fatal("migration incremented schema version on failure")}
    var count int
    if err=db.QueryRow("SELECT COUNT(*) FROM monitor_policies").Scan(&count);err!=nil||count!=2{t.Fatalf("lost policies %d %v",count,err)}
    if err=db.QueryRow("SELECT COUNT(*) FROM monitor_events").Scan(&count);err!=nil||count!=1{t.Fatalf("lost events %d %v",count,err)}
    cols,err:=func()(map[string]bool,error){
        tx,e:=db.Begin();if e!=nil{return nil,e};defer tx.Rollback()
        return tableColumns(context.Background(),tx,"inventory")
    }()
    if err!=nil{t.Fatal(err)}
    if cols["image_architecture"]{t.Fatal("partial inventory column migration committed")}
    if err=db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='monitor_service_settings'").Scan(&count);err!=nil||count!=0{
        t.Fatalf("partial beta.3 table created %d %v",count,err)
    }
}

func TestProvenanceRowsNotBackfilledAgainAfterMigration(t *testing.T) {
    // Future label-created policies must NOT be misinterpreted as legacy
    // registrations merely because the database is opened a second time.
    path:=filepath.Join(t.TempDir(),"source.db")
    s,err:=Open(path);if err!=nil{t.Fatal(err)}
    _,err=s.db.Exec(`INSERT INTO monitor_service_settings(project,service,label_opt_in,label_interval_seconds,discovery_state)
        VALUES('unregistered','web',1,600,'present')`)
    if err!=nil{t.Fatal(err)}
    _=s.Close()
    s,err=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
    rows,err:=s.MonitoringServiceSettings(context.Background())
    if err!=nil||len(rows)!=1||rows[0].LegacyEnrolled||rows[0].LabelOptIn==nil||!*rows[0].LabelOptIn||*rows[0].LabelIntervalSeconds!=600{t.Fatalf("label row changed %+v %v",rows,err)}
}

// Documented migration is from a transactionally captured SQLite snapshot.
// The upgrade must not affect the monitoring scheduler's concept of time.
func TestMigrationKeepsNextCheckTimestamp(t *testing.T) {
    path:=filepath.Join(t.TempDir(),"schedule.db")
    makeBeta2Fixture(t,path)
    s,err:=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
    due,err:=s.DueMonitorPolicies(context.Background(),time.Date(2026,10,25,22,29,1,0,time.UTC),10)
    if err!=nil||len(due)!=1||due[0].Project!="hello-js"{t.Fatalf("next due changed %+v %v",due,err)}
}
