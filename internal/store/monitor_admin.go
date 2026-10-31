package store

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "time"
)

// MonitorServiceView is safe display data from validated Docker discovery and
// monitoring preferences. It never carries Compose source paths or secrets.
type MonitorServiceView struct {
 Project string `json:"project"`
 Service string `json:"service"`
 Image string `json:"image"`
 Platform string `json:"platform"`
 State string `json:"state"`
 ValidationError string `json:"validation_error"`
 EnrollmentSource string `json:"enrollment_source"`
 IntervalSource string `json:"interval_source"`
 EffectiveInterval int `json:"effective_interval_seconds"`
 LabelInterval *int `json:"label_interval_seconds"`
 GUIOverride *int `json:"gui_override_seconds"`
 WatchImageOverride *string `json:"watch_image_override,omitempty"`
 WatchSource string `json:"watch_source"`
 LegacyEnrolled bool `json:"legacy_enrolled"`
 GUIEnrolled bool `json:"gui_enrolled"`
 LabelOptIn *bool `json:"label_opt_in"`
 HasPolicy bool `json:"has_policy"`
 Paused bool `json:"paused"`
 NextCheckAt string `json:"next_check_at"`
 CanGUIEnable bool `json:"can_gui_enable"`
 CanGUIDisable bool `json:"can_gui_disable"`
 CanPause bool `json:"can_pause"`
 CanResume bool `json:"can_resume"`
 CanEditInterval bool `json:"can_edit_interval"`
}

func (s *Store) MonitorServiceViews(ctx context.Context) ([]MonitorServiceView,error) {
 defaults,err:=s.DefaultMonitorInterval(ctx);if err!=nil{return nil,err}
 settings,err:=s.MonitoringServiceSettings(ctx);if err!=nil{return nil,err}
 policies,err:=s.MonitorPolicies(ctx);if err!=nil{return nil,err}
 byID:=make(map[serviceIdentity]MonitorPolicy,len(policies))
 for _,p:=range policies {byID[serviceIdentity{p.Project,p.Service}]=p}
 out:=make([]MonitorServiceView,0,len(settings))
 for _,m:=range settings {
  v:=MonitorServiceView{Project:m.Project,Service:m.Service,State:m.DiscoveryState,
   ValidationError:m.ValidationError,LegacyEnrolled:m.LegacyEnrolled,GUIEnrolled:m.GUIEnrolled,
   LabelOptIn:m.LabelOptIn,GUIOverride:m.GUIIntervalSeconds,LabelInterval:m.LabelIntervalSeconds,
   WatchImageOverride:m.WatchImageOverride,WatchSource:"Docker-observed image",
   EffectiveInterval:defaults,IntervalSource:"Global default",EnrollmentSource:"Not monitored"}
  if m.LegacyEnrolled {v.EnrollmentSource="Legacy manual";v.WatchSource="Legacy manual";if m.LegacyIntervalSeconds!=nil {
   v.EffectiveInterval=*m.LegacyIntervalSeconds;v.IntervalSource="Legacy manual"
  }}
  if !m.LegacyEnrolled && m.LabelIntervalSeconds!=nil {
   v.EffectiveInterval=*m.LabelIntervalSeconds;v.IntervalSource="Docker label"
  }
  if m.GUIIntervalSeconds!=nil {
   v.EffectiveInterval=*m.GUIIntervalSeconds;v.IntervalSource="GUI override"
  }
  if !m.LegacyEnrolled {
   if m.LabelOptIn!=nil&&*m.LabelOptIn {v.EnrollmentSource="Docker label"
   } else if m.GUIEnrolled {v.EnrollmentSource="GUI"}
  }
  if m.LabelOptIn!=nil&&!*m.LabelOptIn {v.EnrollmentSource="Disabled by Docker label"}
  if m.WatchImageOverride!=nil&&!m.LegacyEnrolled {v.WatchSource="GUI watch override"}
  if p,ok:=byID[serviceIdentity{m.Project,m.Service}];ok {
   v.Image=p.Image;v.Platform=p.Platform;v.HasPolicy=true
   v.Paused=p.Paused;v.NextCheckAt=p.NextCheckAt
  }
  v.CanGUIEnable=!m.LegacyEnrolled&&m.LabelOptIn==nil&&m.DiscoveryState=="present"&&!m.GUIEnrolled
  v.CanGUIDisable=!m.LegacyEnrolled&&m.LabelOptIn==nil&&m.GUIEnrolled
  v.CanEditInterval=v.HasPolicy || m.DiscoveryState=="present"
  active:=(m.LabelOptIn==nil||*m.LabelOptIn)&&m.DiscoveryState!="conflict"&&
    (m.LegacyEnrolled||(m.DiscoveryState=="present"&&(m.GUIEnrolled||(m.LabelOptIn!=nil&&*m.LabelOptIn))))
  v.CanPause=v.HasPolicy&&!v.Paused&&active
  v.CanResume=v.HasPolicy&&v.Paused&&active
  out=append(out,v)
 }
 return out,nil
}

func intervalRange(n int)error {
 if n<60||n>86400{return errors.New("interval must be between 60 and 86400 seconds")}
 return nil
}

// rescheduleInterval changes the effective interval atomically with the GUI
// preference. A non-backed-off, unpaused watch is due immediately on a
// changed interval; registry rate-limit backoff is deliberately preserved.
func rescheduleInterval(ctx context.Context,tx *sql.Tx,project,service string,seconds int,now string)error {
 if err:=intervalRange(seconds);err!=nil{return err}
 _,err:=tx.ExecContext(ctx,`UPDATE monitor_policies SET interval_seconds=?,
    next_check_at=CASE WHEN paused=0 AND consecutive_failures=0 THEN ? ELSE next_check_at END,
    generation=generation+1
    WHERE project=? AND service=? AND interval_seconds!=?`,
    seconds,now,project,service,seconds)
 return err
}

func(s *Store) SetServiceIntervalOverride(ctx context.Context,project,service string,override *int)error {
 if project==""||service==""{return errors.New("service identity required")}
 if override!=nil{if err:=intervalRange(*override);err!=nil{return err}}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 var legacy int
 var baseline,label sql.NullInt64
 err=tx.QueryRowContext(ctx,`SELECT legacy_enrolled,legacy_interval_seconds,label_interval_seconds
    FROM monitor_service_settings WHERE project=? AND service=?`,project,service).Scan(&legacy,&baseline,&label)
 if errors.Is(err,sql.ErrNoRows){return errors.New("service not discovered or registered")}
 if err!=nil{return err}
 var value any
 if override!=nil{value=*override}
 _,err=tx.ExecContext(ctx,`UPDATE monitor_service_settings SET gui_interval_seconds=?
    WHERE project=? AND service=?`,value,project,service)
 if err!=nil{return err}
 effective:=300
 if err=tx.QueryRowContext(ctx,"SELECT interval_seconds FROM monitoring_defaults WHERE id=1").Scan(&effective);err!=nil{return err}
 if legacy==1&&baseline.Valid{effective=int(baseline.Int64)
 }else if legacy==0&&label.Valid{effective=int(label.Int64)}
 if override!=nil{effective=*override}
 if err=rescheduleInterval(ctx,tx,project,service,effective,time.Now().UTC().Format(time.RFC3339));err!=nil{return err}
 return tx.Commit()
}

// SetServiceGUIEnrollment opts an *already discovered* unlabeled Compose
// service into read-only registry monitoring. No Compose source registration,
// Docker mutation or filesystem permission is implied.
func(s *Store) SetServiceGUIEnrollment(ctx context.Context,project,service string,enabled bool)error{
 if project==""||service==""{return errors.New("service identity required")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 var legacy,gui int
 var label sql.NullInt64
 var state string
 err=tx.QueryRowContext(ctx,`SELECT legacy_enrolled,gui_enrolled,label_opt_in,discovery_state
   FROM monitor_service_settings WHERE project=? AND service=?`,project,service).
   Scan(&legacy,&gui,&label,&state)
 if errors.Is(err,sql.ErrNoRows){return errors.New("service not discovered")}
 if err!=nil{return err}
 if legacy!=0{return errors.New("legacy service is managed by its registered policy")}
 if label.Valid{return errors.New("Docker label controls monitoring enrollment")}
 if enabled&&state!="present"{return errors.New("service is not running with valid image metadata")}
 if !enabled&&gui==0{return errors.New("service has no GUI enrollment")}
 var image,osName,arch,variant,stateOfContainer string
 if enabled {
  // Inventory is written only following a successful discovery scan. Its
  // project/service identity and image metadata were reconciled as 'present'.
  err=tx.QueryRowContext(ctx,`SELECT image,image_os,image_architecture,image_variant,state
   FROM inventory WHERE project=? AND service=? ORDER BY container_id LIMIT 1`,
   project,service).Scan(&image,&osName,&arch,&variant,&stateOfContainer)
  if err!=nil{return errors.New("discovered service has no verified Docker inventory")}
  candidate:=parseMonitorCandidate(Container{
   Project:project,Service:service,Image:image,ImageOS:osName,
   ImageArchitecture:arch,ImageVariant:variant,State:stateOfContainer})
  if candidate.issue!=""||!candidate.running{return errors.New("invalid discovered image metadata")}
  _,err=tx.ExecContext(ctx,`UPDATE monitor_service_settings SET gui_enrolled=1
   WHERE project=? AND service=?`,project,service)
  if err!=nil{return err}
  var override,labelInt sql.NullInt64
  var watchImage sql.NullString
  var global int
  err=tx.QueryRowContext(ctx,`SELECT gui_interval_seconds,label_interval_seconds,watch_image_override
   FROM monitor_service_settings WHERE project=? AND service=?`,project,service).Scan(&override,&labelInt,&watchImage)
  if err!=nil{return err}
  if err=tx.QueryRowContext(ctx,"SELECT interval_seconds FROM monitoring_defaults WHERE id=1").Scan(&global);err!=nil{return err}
  if labelInt.Valid{global=int(labelInt.Int64)}
  if override.Valid{global=int(override.Int64)}
  if watchImage.Valid {
   requested,e:=explicitWatchReference(watchImage.String)
   if e!=nil||!sameImageRepository(candidate.image,requested){
    return errors.New("watched image no longer matches Docker image repository")
   }
   candidate.image=requested.Key()
  }
  stamp:=time.Now().UTC().Format(time.RFC3339)
  _,err=tx.ExecContext(ctx,`INSERT INTO monitor_policies
   (project,service,image,platform,interval_seconds,paused,next_check_at)
   VALUES(?,?,?,?,?,0,?)
   ON CONFLICT(project,service) DO UPDATE SET
     image=excluded.image,platform=excluded.platform,
     interval_seconds=excluded.interval_seconds,
     paused=0,next_check_at=excluded.next_check_at,
     generation=monitor_policies.generation+1,
     last_checked_at=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform THEN '' ELSE monitor_policies.last_checked_at END,
     last_error=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform THEN '' ELSE monitor_policies.last_error END,
     consecutive_failures=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform THEN 0 ELSE monitor_policies.consecutive_failures END`,
     project,service,candidate.image,candidate.platform,global,stamp)
  if err!=nil{return fmt.Errorf("enable GUI monitoring: %w",err)}
 }else{
  _,err=tx.ExecContext(ctx,`UPDATE monitor_service_settings SET gui_enrolled=0 WHERE project=? AND service=?`,project,service)
  if err!=nil{return err}
  _,err=tx.ExecContext(ctx,`UPDATE monitor_policies SET generation=generation+1 WHERE project=? AND service=?`,project,service)
  if err!=nil{return err}
 }
 return tx.Commit()
}

func(s *Store) SetServicePaused(ctx context.Context,project,service string,paused bool)error{
 if project==""||service==""{return errors.New("service identity required")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 var legacy,gui int
 var label sql.NullInt64
 var state string
 err=tx.QueryRowContext(ctx,`SELECT legacy_enrolled,gui_enrolled,label_opt_in,discovery_state
   FROM monitor_service_settings WHERE project=? AND service=?`,project,service).
   Scan(&legacy,&gui,&label,&state)
 if errors.Is(err,sql.ErrNoRows){return errors.New("service not discovered or registered")}
 if err!=nil{return err}
 if !paused&&(label.Valid&&label.Int64==0||state=="conflict"||
  (legacy==0&&(gui==0&&(!label.Valid||label.Int64!=1)||state!="present"))) {
  return errors.New("monitoring is not currently eligible to resume")
 }
 stamp:=time.Now().UTC().Format(time.RFC3339)
 result,err:=tx.ExecContext(ctx,`UPDATE monitor_policies SET paused=?,
   next_check_at=CASE WHEN ?=0 THEN ? ELSE next_check_at END,
   generation=generation+1 WHERE project=? AND service=?`,
   boolAsInt(paused),boolAsInt(paused),stamp,project,service)
 if err!=nil{return err}
 n,err:=result.RowsAffected();if err!=nil{return err}
 if n!=1{return errors.New("monitoring policy does not exist")}
 return tx.Commit()
}
func boolAsInt(b bool)int{if b{return 1};return 0}
