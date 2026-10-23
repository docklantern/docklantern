package store

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "time"
)

// Event is an immutable monitoring observation. It is not a verified newer
// release and never authorizes a Docker deployment.
type Event struct {
 ID int64 `json:"id"`
 Kind string `json:"kind"`
 Project string `json:"project,omitempty"`
 Service string `json:"service,omitempty"`
 Image string `json:"image,omitempty"`
 Platform string `json:"platform,omitempty"`
 PreviousDigest string `json:"previous_digest,omitempty"`
 Digest string `json:"digest,omitempty"`
 OccurredAt string `json:"occurred_at"`
 DeliveryState string `json:"delivery_state"`
 DeliveryAttempts int `json:"delivery_attempts"`
 NextDeliveryAt string `json:"next_delivery_at,omitempty"`
 LastDeliveryError string `json:"last_delivery_error,omitempty"`
}
const eventColumns="id,kind,project,service,image,platform,previous_digest,digest,occurred_at,delivery_state,delivery_attempts,next_delivery_at,last_delivery_error"

func insertEvent(ctx context.Context,tx *sql.Tx,e Event,at time.Time)error{
 if e.Kind=="" {return errors.New("missing event kind")}
 stamp:=at.UTC().Format(time.RFC3339Nano)
 _,err:=tx.ExecContext(ctx,`INSERT INTO monitor_events(kind,project,service,image,platform,previous_digest,digest,occurred_at,delivery_state,delivery_attempts,next_delivery_at,last_delivery_error)
 VALUES(?,?,?,?,?,?,?,?,'pending',0,?,'')`,
 e.Kind,e.Project,e.Service,e.Image,e.Platform,e.PreviousDigest,e.Digest,stamp,stamp)
 return err
}
func scanEvent(rows interface{Scan(...any)error})(Event,error){
 var e Event
 err:=rows.Scan(&e.ID,&e.Kind,&e.Project,&e.Service,&e.Image,&e.Platform,&e.PreviousDigest,&e.Digest,&e.OccurredAt,&e.DeliveryState,&e.DeliveryAttempts,&e.NextDeliveryAt,&e.LastDeliveryError)
 return e,err
}
func(s *Store)Events(ctx context.Context,limit int)([]Event,error){
 if limit<1||limit>500 {return nil,errors.New("event limit must be between 1 and 500")}
 rows,err:=s.db.QueryContext(ctx,"SELECT "+eventColumns+" FROM monitor_events ORDER BY id DESC LIMIT ?",limit)
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]Event{}
 for rows.Next(){e,err:=scanEvent(rows);if err!=nil{return nil,err};out=append(out,e)}
 return out,rows.Err()
}
// DueEvents returns bounded pending webhook messages. The caller must run a
// single notifier instance per SQLite database; multi-host leases are not yet supported.
func(s *Store)DueEvents(ctx context.Context,now time.Time,limit int)([]Event,error){
 if limit<1||limit>100{return nil,errors.New("invalid delivery limit")}
 rows,err:=s.db.QueryContext(ctx,`SELECT `+eventColumns+` FROM monitor_events WHERE delivery_state='pending' AND next_delivery_at<=?
 AND CASE kind
 WHEN 'remote_digest_changed' THEN (SELECT digest_changes FROM notification_settings WHERE id=1)
 WHEN 'registry_failure' THEN (SELECT registry_failures FROM notification_settings WHERE id=1)
 WHEN 'registry_recovered' THEN (SELECT registry_recovery FROM notification_settings WHERE id=1)
 WHEN 'registry_outage_prolonged' THEN (SELECT prolonged_outage FROM notification_settings WHERE id=1)
 ELSE 0 END = 1
 ORDER BY id LIMIT ?`,now.UTC().Format(time.RFC3339Nano),limit)
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]Event{}
 for rows.Next(){e,err:=scanEvent(rows);if err!=nil{return nil,err};out=append(out,e)}
 return out,rows.Err()
}
// CompleteDelivery persists at-least-once delivery progress. An event is never
// removed on failure; a failed webhook does not affect OCI check scheduling.
func(s *Store)CompleteDelivery(ctx context.Context,e Event,now time.Time,success bool)error{
 state:="pending";next:="";lastError:=""
 attempts:=e.DeliveryAttempts+1
 if success{state="delivered"}else{
  // Retry at 1, 2, 4, 8, 16, 32, 60 minutes (bounded).
  delay:=time.Minute
  for i:=1;i<attempts&&i<6;i++{delay*=2}
  if delay>time.Hour{delay=time.Hour}
  next=now.UTC().Add(delay).Format(time.RFC3339Nano)
  lastError="webhook delivery failed"
 }
 result,err:=s.db.ExecContext(ctx,`UPDATE monitor_events SET delivery_state=?,delivery_attempts=?,next_delivery_at=?,last_delivery_error=?
 WHERE id=? AND delivery_state='pending' AND delivery_attempts=?`,
 state,attempts,next,lastError,e.ID,e.DeliveryAttempts)
 if err!=nil{return err}
 changed,err:=result.RowsAffected();if err!=nil{return err}
 if changed!=1{return fmt.Errorf("event %d delivery state changed concurrently",e.ID)}
 return nil
}
