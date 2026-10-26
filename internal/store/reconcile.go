package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "sort"
    "strings"
    "time"

    "github.com/docklantern/docklantern/internal/registry"
)

const (
    labelMonitor = "com.docklantern.monitor"
    labelInterval = "com.docklantern.interval"
)

type serviceIdentity struct {project,service string}

type monitorCandidate struct {
    image,platform string
    label *bool
    interval *int
    running bool
    issue string
}

func parseMonitorCandidate(c Container) monitorCandidate {
    out:=monitorCandidate{running:c.State=="running"}
    if raw,ok:=c.MonitorLabels[labelMonitor];ok {
        switch raw {
        case "true": b:=true;out.label=&b
        case "false": b:=false;out.label=&b
        default: out.issue="invalid monitoring opt-in label"
        }
    }
    if raw,ok:=c.MonitorLabels[labelInterval];ok {
        d,err:=time.ParseDuration(raw)
        if err!=nil||d<time.Minute||d>24*time.Hour||d%time.Second!=0 {
            out.issue="invalid monitoring interval label"
        } else {n:=int(d/time.Second);out.interval=&n}
    }
    ref,err:=registry.Parse(c.Image)
    if err!=nil {out.issue="unsupported or untagged image reference"} else {out.image=ref.Key()}
    out.platform=c.ImageOS+"/"+c.ImageArchitecture
    if c.ImageVariant!="" {out.platform+="/"+c.ImageVariant}
    if !validPlatform(out.platform)||c.ImageOS==""||c.ImageArchitecture==""||strings.ContainsAny(c.ImageOS,"/")||strings.ContainsAny(c.ImageArchitecture,"/")||strings.ContainsAny(c.ImageVariant,"/") {
        out.issue="image platform unavailable"
    }
    return out
}
func sameCandidate(a,b monitorCandidate) bool {
    if a.image!=b.image||a.platform!=b.platform||a.issue!=b.issue {return false}
    if (a.label==nil)!=(b.label==nil)||(a.interval==nil)!=(b.interval==nil){return false}
    if a.label!=nil&&*a.label!=*b.label{return false}
    if a.interval!=nil&&*a.interval!=*b.interval{return false}
    return true
}
func optionalBool(v *bool) any {
    if v==nil{return nil}
    if *v{return 1}
    return 0
}
func optionalInt(v *int) any {
    if v==nil{return nil}
    return *v
}

// reconcileMonitors runs only after a successful, complete Docker scan and
// inside the same transaction as the inventory snapshot. It never acts on a
// Docker socket or Compose source file; it only records registry watch state.
// Existing beta.2 manual policies are never overwritten by automatic labels.
func (s *Store) reconcileMonitors(ctx context.Context,tx *sql.Tx,items []Container,now time.Time) error {
    groups:=make(map[serviceIdentity][]monitorCandidate)
    for _,c:=range items {
        if c.Project==""||c.Service=="" {continue}
        key:=serviceIdentity{c.Project,c.Service}
        groups[key]=append(groups[key],parseMonitorCandidate(c))
    }
    keys:=make([]serviceIdentity,0,len(groups))
    for k:=range groups {keys=append(keys,k)}
    sort.Slice(keys,func(i,j int)bool{
        if keys[i].project==keys[j].project{return keys[i].service<keys[j].service}
        return keys[i].project<keys[j].project
    })
    var defaultInterval int
    if err:=tx.QueryRowContext(ctx,"SELECT interval_seconds FROM monitoring_defaults WHERE id=1").Scan(&defaultInterval);err!=nil {
        return fmt.Errorf("read monitoring default: %w",err)
    }
    stamp:=now.UTC().Format(time.RFC3339)
    for _,k:=range keys {
        replicas:=groups[k]
        desired:=replicas[0]
        running:=desired.running
        for _,other:=range replicas[1:] {
            running=running||other.running
            if !sameCandidate(desired,other) {desired.issue="conflicting Compose service replicas"}
        }
        state:="present"
        if !running{state="missing"}
        if desired.issue!=""{state="conflict"}
        // Upsert keeps operator-controlled GUI fields and legacy provenance
        // exactly as they were; only label and observation data are replaced.
        _,err:=tx.ExecContext(ctx,`INSERT INTO monitor_service_settings
         (project,service,label_opt_in,label_interval_seconds,discovery_state,last_seen_at,validation_error)
         VALUES(?,?,?,?,?,?,?)
         ON CONFLICT(project,service) DO UPDATE SET
         label_opt_in=excluded.label_opt_in,
         label_interval_seconds=excluded.label_interval_seconds,
         discovery_state=excluded.discovery_state,
         last_seen_at=excluded.last_seen_at,
         validation_error=excluded.validation_error`,
            k.project,k.service,optionalBool(desired.label),optionalInt(desired.interval),state,stamp,desired.issue)
        if err!=nil{return fmt.Errorf("record monitoring discovery: %w",err)}
        if state!="present" {continue}
        var legacy,gui int
        var override sql.NullInt64
        err=tx.QueryRowContext(ctx,`SELECT legacy_enrolled,gui_enrolled,gui_interval_seconds
         FROM monitor_service_settings WHERE project=? AND service=?`,k.project,k.service).
            Scan(&legacy,&gui,&override)
        if err!=nil{return err}
        // The explicit opt-out always wins over legacy and GUI enrollment.
        if desired.label!=nil&&!*desired.label {continue}
        if legacy!=0 {continue} // Never silently overwrite beta.2 policies.
        if gui==0&&(desired.label==nil||!*desired.label){continue}
        interval:=defaultInterval
        if desired.interval!=nil{interval=*desired.interval}
        if override.Valid {interval=int(override.Int64)}
        if interval<60||interval>86400{return errors.New("invalid effective monitoring interval")}

        var image,platform,next string
        var oldInterval, failures int
        err=tx.QueryRowContext(ctx,`SELECT image,platform,interval_seconds,next_check_at,consecutive_failures
          FROM monitor_policies WHERE project=? AND service=?`,k.project,k.service).
          Scan(&image,&platform,&oldInterval,&next,&failures)
        if err==sql.ErrNoRows {
            _,err=tx.ExecContext(ctx,`INSERT INTO monitor_policies
              (project,service,image,platform,interval_seconds,paused,next_check_at)
              VALUES(?,?,?,?,?,0,?)`,k.project,k.service,desired.image,desired.platform,interval,stamp)
            if err!=nil{return fmt.Errorf("create label monitoring policy: %w",err)}
            continue
        }
        if err!=nil{return err}
        if image==desired.image&&platform==desired.platform&&oldInterval==interval {continue}
        // A new image must be checked immediately. Interval-only edits honor
        // existing failure backoff rather than flooding a rate-limited registry.
        nextAt:=next
        if image!=desired.image||platform!=desired.platform||failures==0 {nextAt=stamp}
        _,err=tx.ExecContext(ctx,`UPDATE monitor_policies SET
          image=?,platform=?,interval_seconds=?,next_check_at=?,generation=generation+1,
          last_checked_at=CASE WHEN image!=? OR platform!=? THEN '' ELSE last_checked_at END,
          last_error=CASE WHEN image!=? OR platform!=? THEN '' ELSE last_error END,
          consecutive_failures=CASE WHEN image!=? OR platform!=? THEN 0 ELSE consecutive_failures END
          WHERE project=? AND service=?`,
          desired.image,desired.platform,interval,nextAt,
          desired.image,desired.platform,desired.image,desired.platform,
          desired.image,desired.platform,k.project,k.service)
        if err!=nil{return err}
    }
    // Services absent from a *successful* snapshot are inactive. Preserve
    // policies, GUI overrides and history for later reappearance.
    _,err:=tx.ExecContext(ctx,`UPDATE monitor_service_settings SET discovery_state='missing',
       label_opt_in=NULL,label_interval_seconds=NULL,validation_error=''
       WHERE discovery_state IN ('present','conflict')
       AND NOT EXISTS (SELECT 1 FROM inventory i WHERE i.project=monitor_service_settings.project
         AND i.service=monitor_service_settings.service)`)
    if err!=nil{return err}
    return nil
}

