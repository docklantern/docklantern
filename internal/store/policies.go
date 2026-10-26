package store

import (
 "context"
 "errors"
 "fmt"
 "time"
 "github.com/docklantern/docklantern/internal/registry"
)

// MonitorPolicy is a monitoring-only service policy. The interval is expressed in seconds.
// Credential secrets are never persisted in this table.
type MonitorPolicy struct {
 Project string `json:"project"`
 Service string `json:"service"`
 Image string `json:"image"`
 Platform string `json:"platform"`
 IntervalSeconds int `json:"interval_seconds"`
 Paused bool `json:"paused"`
 NextCheckAt string `json:"next_check_at"`
 LastCheckedAt string `json:"last_checked_at,omitempty"`
 LastError string `json:"last_error,omitempty"`
 ConsecutiveFailures int `json:"consecutive_failures"`
}

func (s *Store) SetMonitorPolicy(ctx context.Context,p MonitorPolicy) error {
 ref,err:=registry.Parse(p.Image);if err!=nil{return fmt.Errorf("image: %w",err)}
 if p.IntervalSeconds<60||p.IntervalSeconds>86400{return errors.New("interval must be between 60 and 86400 seconds")}
 if !validPlatform(p.Platform){return errors.New("invalid platform")}
 // Service must be present in an explicit monitoring registration, not just discovered Docker labels.
 var allowed string
 if err=s.db.QueryRowContext(ctx,"SELECT services_json FROM registrations WHERE name=?",p.Project).Scan(&allowed);err!=nil{return errors.New("project is not registered")}
 var services []string
 if err=decodeServices(allowed,&services);err!=nil{return err}
 matched:=false;for _,v:=range services{if v==p.Service{matched=true;break}}
 if !matched{return errors.New("service is not registered for monitoring")}
 // A new image/tag is checked immediately; existing policy is kept if only interval changes.
 p.Image=ref.Key()
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 _,err=tx.ExecContext(ctx,`INSERT INTO monitor_policies(project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error)
 VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(project,service) DO UPDATE SET
 image=excluded.image,platform=excluded.platform,interval_seconds=excluded.interval_seconds,
 paused=excluded.paused,
 next_check_at=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN excluded.next_check_at ELSE monitor_policies.next_check_at END,
 last_checked_at=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN '' ELSE monitor_policies.last_checked_at END,
 last_error=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN '' ELSE monitor_policies.last_error END,
 consecutive_failures=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN 0 ELSE monitor_policies.consecutive_failures END`,
 p.Project,p.Service,p.Image,p.Platform,p.IntervalSeconds,p.Paused,time.Now().UTC().Format(time.RFC3339),"","")
 if err!=nil{return err}
 // A manually saved policy is explicitly legacy-enrolled, not label-enrolled.
 // The new metadata and old monitoring policy must commit together.
 _,err=tx.ExecContext(ctx,`INSERT INTO monitor_service_settings(project,service,legacy_enrolled,legacy_interval_seconds)
 VALUES(?,?,1,?) ON CONFLICT(project,service) DO UPDATE SET legacy_enrolled=1,legacy_interval_seconds=excluded.legacy_interval_seconds,gui_interval_seconds=NULL`,p.Project,p.Service,p.IntervalSeconds)
 if err!=nil{return err}
 return tx.Commit()
}

func (s *Store) MonitorPolicies(ctx context.Context)([]MonitorPolicy,error){
 rows,err:=s.db.QueryContext(ctx,"SELECT project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error,consecutive_failures FROM monitor_policies ORDER BY project,service")
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]MonitorPolicy{}
 for rows.Next(){var p MonitorPolicy;if err=rows.Scan(&p.Project,&p.Service,&p.Image,&p.Platform,&p.IntervalSeconds,&p.Paused,&p.NextCheckAt,&p.LastCheckedAt,&p.LastError,&p.ConsecutiveFailures);err!=nil{return nil,err};out=append(out,p)}
 return out,rows.Err()
}

func(s *Store) DueMonitorPolicies(ctx context.Context,now time.Time,limit int)([]MonitorPolicy,error){
 if limit<1||limit>100{return nil,errors.New("invalid limit")}
 rows,err:=s.db.QueryContext(ctx,`SELECT p.project,p.service,p.image,p.platform,p.interval_seconds,p.paused,p.next_check_at,p.last_checked_at,p.last_error,p.consecutive_failures
 FROM monitor_policies p
 LEFT JOIN monitor_service_settings m ON m.project=p.project AND m.service=p.service
 WHERE p.paused=0 AND p.next_check_at<=?
   AND (m.label_opt_in IS NULL OR m.label_opt_in!=0)
   AND m.discovery_state!='conflict'
   AND (m.legacy_enrolled=1 OR
        (m.discovery_state='present' AND (m.gui_enrolled=1 OR m.label_opt_in=1)))
 ORDER BY p.next_check_at,p.project,p.service LIMIT ?`,now.UTC().Format(time.RFC3339),limit)
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]MonitorPolicy{}
 for rows.Next(){var p MonitorPolicy;if err=rows.Scan(&p.Project,&p.Service,&p.Image,&p.Platform,&p.IntervalSeconds,&p.Paused,&p.NextCheckAt,&p.LastCheckedAt,&p.LastError,&p.ConsecutiveFailures);err!=nil{return nil,err};out=append(out,p)}
 return out,rows.Err()
}

// FinishMonitorCheck schedules after the completed attempt. Compare-and-set prevents an
// old in-flight check from overwriting policy changes or an administrator pause.
func(s *Store) FinishMonitorCheck(ctx context.Context,p MonitorPolicy,finished time.Time,checkErr error)error{
 message:="";if checkErr!=nil{message=checkErr.Error();if len(message)>160{message=message[:160]}}
 failures:=0
 delay:=time.Duration(p.IntervalSeconds)*time.Second
 if checkErr!=nil{
  failures=p.ConsecutiveFailures+1
  // Exponential backoff: 2x,4x,8x, capped at six hours.
  for i:=0;i<failures&&i<10;i++{delay*=2;if delay>=6*time.Hour{delay=6*time.Hour;break}}
  if delay>6*time.Hour{delay=6*time.Hour}
  var retry *registry.RetryableError
  if errors.As(checkErr,&retry)&&retry.RetryAfter>delay{delay=retry.RetryAfter}
  if delay>24*time.Hour{delay=24*time.Hour}
 }
 next:=finished.UTC().Add(delay)
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 result,err:=tx.ExecContext(ctx,`UPDATE monitor_policies SET last_checked_at=?,last_error=?,next_check_at=?,consecutive_failures=?
 WHERE project=? AND service=? AND image=? AND platform=? AND interval_seconds=? AND paused=0 AND next_check_at=?
 AND EXISTS(SELECT 1 FROM monitor_service_settings m
            WHERE m.project=monitor_policies.project AND m.service=monitor_policies.service
              AND (m.label_opt_in IS NULL OR m.label_opt_in!=0)
              AND m.discovery_state!='conflict'
              AND (m.legacy_enrolled=1 OR
                   (m.discovery_state='present' AND (m.gui_enrolled=1 OR m.label_opt_in=1))))`,
 finished.UTC().Format(time.RFC3339),message,next.Format(time.RFC3339),failures,p.Project,p.Service,p.Image,p.Platform,p.IntervalSeconds,p.NextCheckAt)
 if err!=nil{return err}
 changed,err:=result.RowsAffected();if err!=nil{return err}
 if changed==0{return tx.Commit()} // changed/paused policy; do not emit stale events.
 var kind string
 if checkErr!=nil && p.ConsecutiveFailures==0 {kind="registry_failure"}
 if checkErr==nil && p.ConsecutiveFailures>0 {kind="registry_recovered"}
 if checkErr!=nil {
  var threshold int
  if err=tx.QueryRowContext(ctx,"SELECT outage_failures FROM notification_settings WHERE id=1").Scan(&threshold);err!=nil{return err}
  if failures==threshold {kind="registry_outage_prolonged"}
 }
 if kind!=""{
  if err=insertEvent(ctx,tx,Event{Kind:kind,Project:p.Project,Service:p.Service,Image:p.Image,Platform:p.Platform},finished);err!=nil{return err}
 }
 return tx.Commit()
}

func validPlatform(v string)bool{
 parts:=splitPlatform(v);if len(parts)<2||len(parts)>3{return false}
 for _,p:=range parts{if p==""{return false};for _,r:=range p{if !((r>='a'&&r<='z')||(r>='0'&&r<='9')||r=='_'||r=='-'){return false}}}
 return true
}
