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
 _,err=s.db.ExecContext(ctx,`INSERT INTO monitor_policies(project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error)
 VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(project,service) DO UPDATE SET
 image=excluded.image,platform=excluded.platform,interval_seconds=excluded.interval_seconds,
 paused=excluded.paused,
 next_check_at=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN excluded.next_check_at ELSE monitor_policies.next_check_at END,
 last_checked_at=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN '' ELSE monitor_policies.last_checked_at END,
 last_error=CASE WHEN monitor_policies.image!=excluded.image OR monitor_policies.platform!=excluded.platform
 THEN '' ELSE monitor_policies.last_error END`,
 p.Project,p.Service,p.Image,p.Platform,p.IntervalSeconds,p.Paused,time.Now().UTC().Format(time.RFC3339),"","")
 return err
}

func (s *Store) MonitorPolicies(ctx context.Context)([]MonitorPolicy,error){
 rows,err:=s.db.QueryContext(ctx,"SELECT project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error FROM monitor_policies ORDER BY project,service")
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]MonitorPolicy{}
 for rows.Next(){var p MonitorPolicy;if err=rows.Scan(&p.Project,&p.Service,&p.Image,&p.Platform,&p.IntervalSeconds,&p.Paused,&p.NextCheckAt,&p.LastCheckedAt,&p.LastError);err!=nil{return nil,err};out=append(out,p)}
 return out,rows.Err()
}

func(s *Store) DueMonitorPolicies(ctx context.Context,now time.Time,limit int)([]MonitorPolicy,error){
 if limit<1||limit>100{return nil,errors.New("invalid limit")}
 rows,err:=s.db.QueryContext(ctx,`SELECT project,service,image,platform,interval_seconds,paused,next_check_at,last_checked_at,last_error
 FROM monitor_policies WHERE paused=0 AND next_check_at<=? ORDER BY next_check_at,project,service LIMIT ?`,now.UTC().Format(time.RFC3339),limit)
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]MonitorPolicy{}
 for rows.Next(){var p MonitorPolicy;if err=rows.Scan(&p.Project,&p.Service,&p.Image,&p.Platform,&p.IntervalSeconds,&p.Paused,&p.NextCheckAt,&p.LastCheckedAt,&p.LastError);err!=nil{return nil,err};out=append(out,p)}
 return out,rows.Err()
}

// FinishMonitorCheck schedules after the completed attempt. Compare-and-set prevents an
// old in-flight check from overwriting policy changes or an administrator pause.
func(s *Store) FinishMonitorCheck(ctx context.Context,p MonitorPolicy,finished time.Time,checkErr error)error{
 message:="";if checkErr!=nil{message=checkErr.Error();if len(message)>160{message=message[:160]}}
 next:=finished.UTC().Add(time.Duration(p.IntervalSeconds)*time.Second)
 _,err:=s.db.ExecContext(ctx,`UPDATE monitor_policies SET last_checked_at=?,last_error=?,next_check_at=?
 WHERE project=? AND service=? AND image=? AND platform=? AND paused=0 AND next_check_at=?`,
 finished.UTC().Format(time.RFC3339),message,next.Format(time.RFC3339),p.Project,p.Service,p.Image,p.Platform,p.NextCheckAt)
 return err
}

func validPlatform(v string)bool{
 parts:=splitPlatform(v);if len(parts)<2||len(parts)>3{return false}
 for _,p:=range parts{if p==""{return false};for _,r:=range p{if !((r>='a'&&r<='z')||(r>='0'&&r<='9')||r=='_'||r=='-'){return false}}}
 return true
}
