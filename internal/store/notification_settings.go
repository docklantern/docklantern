package store

import (
 "context"
 "errors"
 "time"
)

// NotificationSettings contains no endpoint URLs or shared secrets.
// The webhook sender remains opt-in using environment and secret files.
type NotificationSettings struct {
 DigestChanges bool `json:"digest_changes"`
 RegistryFailures bool `json:"registry_failures"`
 RegistryRecovery bool `json:"registry_recovery"`
 ProlongedOutage bool `json:"prolonged_outage"`
 OutageFailures int `json:"outage_failures"`
 RetentionDays int `json:"retention_days"`
}
func DefaultNotificationSettings()NotificationSettings{
 return NotificationSettings{true,true,true,true,3,30}
}
func validateSettings(c NotificationSettings)error{
 if c.OutageFailures<2||c.OutageFailures>100{return errors.New("outage threshold must be between 2 and 100 consecutive failures")}
 if c.RetentionDays<1||c.RetentionDays>3650{return errors.New("retention days must be between 1 and 3650")}
 return nil
}
func(s *Store)NotificationSettings(ctx context.Context)(NotificationSettings,error){
 var c NotificationSettings
 err:=s.db.QueryRowContext(ctx,`SELECT digest_changes,registry_failures,registry_recovery,prolonged_outage,outage_failures,retention_days FROM notification_settings WHERE id=1`).Scan(
 &c.DigestChanges,&c.RegistryFailures,&c.RegistryRecovery,&c.ProlongedOutage,&c.OutageFailures,&c.RetentionDays)
 return c,err
}
func(s *Store)SetNotificationSettings(ctx context.Context,c NotificationSettings)error{
 if err:=validateSettings(c);err!=nil{return err}
 _,err:=s.db.ExecContext(ctx,`UPDATE notification_settings SET digest_changes=?,registry_failures=?,registry_recovery=?,prolonged_outage=?,outage_failures=?,retention_days=? WHERE id=1`,
 c.DigestChanges,c.RegistryFailures,c.RegistryRecovery,c.ProlongedOutage,c.OutageFailures,c.RetentionDays)
 return err
}
type EventFilter struct{
 Kind string
 DeliveryState string
 Search string
 BeforeID int64
 Limit int
}
func validEventKind(s string)bool{
 switch s{case "","remote_digest_changed","registry_failure","registry_recovered","registry_outage_prolonged":return true}
 return false
}
func validDeliveryState(s string)bool{
 switch s{case "","pending","delivered":return true}
 return false
}
// FilterEvents uses keyset pagination and explicit allowlists; wildcard query
// characters in Search are escaped before matching.
func(s *Store)FilterEvents(ctx context.Context,f EventFilter)([]Event,error){
 if f.Limit<1||f.Limit>200||!validEventKind(f.Kind)||!validDeliveryState(f.DeliveryState)||len(f.Search)>128||f.BeforeID<0{
  return nil,errors.New("invalid event filters")
 }
 query:="SELECT "+eventColumns+" FROM monitor_events WHERE 1=1"
 var args []any
 if f.Kind!=""{query+=" AND kind=?";args=append(args,f.Kind)}
 if f.DeliveryState!=""{query+=" AND delivery_state=?";args=append(args,f.DeliveryState)}
 if f.BeforeID>0{query+=" AND id<?";args=append(args,f.BeforeID)}
 if f.Search!=""{
  term:=escapeLike(f.Search)
  query+=" AND (project LIKE ? ESCAPE '!' OR service LIKE ? ESCAPE '!' OR image LIKE ? ESCAPE '!')"
  for i:=0;i<3;i++{args=append(args,"%"+term+"%")}
 }
 query+=" ORDER BY id DESC LIMIT ?"
 args=append(args,f.Limit)
 rows,err:=s.db.QueryContext(ctx,query,args...);if err!=nil{return nil,err}
 defer rows.Close()
 result:=[]Event{}
 for rows.Next(){e,err:=scanEvent(rows);if err!=nil{return nil,err};result=append(result,e)}
 return result,rows.Err()
}
func escapeLike(s string)string{
 out:=""
 for _,r:=range s{
  if r=='!'||r=='%'||r=='_'{out+="!"}
  out+=string(r)
 }
 return out
}
// PruneDeliveredEvents never deletes pending alerts, even if older than
// retention. It is explicit maintenance, not an automatic destructive job.
func(s *Store)PruneDeliveredEvents(ctx context.Context,now time.Time)(int64,error){
 settings,err:=s.NotificationSettings(ctx);if err!=nil{return 0,err}
 cutoff:=now.UTC().AddDate(0,0,-settings.RetentionDays).Format(time.RFC3339Nano)
 result,err:=s.db.ExecContext(ctx,"DELETE FROM monitor_events WHERE delivery_state='delivered' AND occurred_at<?",cutoff)
 if err!=nil{return 0,err}
 return result.RowsAffected()
}
