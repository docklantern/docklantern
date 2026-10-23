package store

import(
 "context"
 "errors"
 "time"
)
type WorkerStatus struct{
 Name string `json:"name"`
 Enabled bool `json:"enabled"`
 State string `json:"state"`
 LastStartedAt string `json:"last_started_at,omitempty"`
 LastSuccessAt string `json:"last_success_at,omitempty"`
 LastErrorAt string `json:"last_error_at,omitempty"`
 LastError string `json:"last_error,omitempty"`
 LastHeartbeatAt string `json:"last_heartbeat_at,omitempty"`
 LastItems int `json:"last_items"`
}
// RecordWorkerRun only records bounded, non-secret operational status. Each worker
// emits one heartbeat after a completed pass, including empty passes.
func(s *Store)RecordWorkerRun(ctx context.Context,name string,finished time.Time,items int,runErr error)error{
 if !workerName(name)||items<0{return errors.New("invalid worker status")}
 stamp:=finished.UTC().Format(time.RFC3339Nano)
 state:="ok";message:=""
 if runErr!=nil{state="error";message="Worker pass failed"} // Never persist registry/webhook error details
 _,err:=s.db.ExecContext(ctx,`INSERT INTO worker_status(name,state,last_started_at,last_success_at,last_error_at,last_error,last_heartbeat_at,last_items)
 VALUES(?,?,?,?,'','',?,?)
 ON CONFLICT(name) DO UPDATE SET
 state=excluded.state,
 last_success_at=CASE WHEN excluded.state='ok' THEN excluded.last_success_at ELSE worker_status.last_success_at END,
 last_error_at=CASE WHEN excluded.state='error' THEN excluded.last_heartbeat_at ELSE worker_status.last_error_at END,
 last_error=CASE WHEN excluded.state='error' THEN ? ELSE '' END,
 last_heartbeat_at=excluded.last_heartbeat_at,last_items=excluded.last_items`,
 name,state,stamp,stamp,stamp,items,message)
 return err
}
func(s *Store)RecordWorkerStart(ctx context.Context,name string,started time.Time)error{
 if !workerName(name){return errors.New("invalid worker")}
 stamp:=started.UTC().Format(time.RFC3339Nano)
 _,err:=s.db.ExecContext(ctx,`INSERT INTO worker_status(name,state,last_started_at,last_success_at,last_error_at,last_error,last_heartbeat_at,last_items)
 VALUES(?,'starting',?,'','','','',0)
 ON CONFLICT(name) DO UPDATE SET state='starting',last_started_at=excluded.last_started_at`,name,stamp)
 return err
}
func(s *Store)WorkerStatuses(ctx context.Context)([]WorkerStatus,error){
 rows,err:=s.db.QueryContext(ctx,`SELECT name,state,last_started_at,last_success_at,last_error_at,last_error,last_heartbeat_at,last_items FROM worker_status ORDER BY name`)
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]WorkerStatus{}
 for rows.Next(){var v WorkerStatus;err=rows.Scan(&v.Name,&v.State,&v.LastStartedAt,&v.LastSuccessAt,&v.LastErrorAt,&v.LastError,&v.LastHeartbeatAt,&v.LastItems);if err!=nil{return nil,err};out=append(out,v)}
 return out,rows.Err()
}
func workerName(s string)bool{
 switch s{case "registry_monitor","webhook_notifier","docker_discovery":return true};return false
}
