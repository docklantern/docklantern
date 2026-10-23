// Package store persists explicit project registrations and inventory snapshots.
package store

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "time"
 _ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Registration struct{
 Name string `json:"name"`
 ProjectDir string `json:"project_dir"`
 Files []string `json:"files"`
 EnvFiles []string `json:"env_files,omitempty"`
 Services []string `json:"services"`
 Profiles []string `json:"profiles,omitempty"`
 Enabled bool `json:"enabled"`
 CreatedAt string `json:"created_at"`
}
type Container struct{
 ID string `json:"id"`
 Name string `json:"name"`
 Project string `json:"project"`
 Service string `json:"service"`
 Image string `json:"image"`
 ImageID string `json:"image_id"`
 State string `json:"state"`
 Health string `json:"health,omitempty"`
 RepoDigests []string `json:"repo_digests,omitempty"`
 ImageOS string `json:"image_os,omitempty"`
 ImageArchitecture string `json:"image_architecture,omitempty"`
 ImageVariant string `json:"image_variant,omitempty"`
 ObservedAt string `json:"observed_at,omitempty"`
}
func Open(path string)(*Store,error){
 if path==""{return nil,errors.New("database path required")}
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return nil,err}
 db,err:=sql.Open("sqlite",path);if err!=nil{return nil,err}
 db.SetMaxOpenConns(1)
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 for _,q:=range []string{
  "PRAGMA busy_timeout=5000",
  "PRAGMA journal_mode=WAL",
  "CREATE TABLE IF NOT EXISTS registrations(name TEXT PRIMARY KEY,project_dir TEXT NOT NULL,files_json TEXT NOT NULL,env_files_json TEXT NOT NULL,services_json TEXT NOT NULL,profiles_json TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL)",
  "CREATE TABLE IF NOT EXISTS inventory(container_id TEXT PRIMARY KEY,name TEXT NOT NULL,project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,image_id TEXT NOT NULL,state TEXT NOT NULL,health TEXT NOT NULL,observed_at TEXT NOT NULL)",
  "CREATE TABLE IF NOT EXISTS registry_checks(image TEXT NOT NULL,platform TEXT NOT NULL,index_digest TEXT NOT NULL,platform_digest TEXT NOT NULL,previous_digest TEXT NOT NULL,status TEXT NOT NULL,checked_at TEXT NOT NULL,PRIMARY KEY(image,platform))",
   "CREATE TABLE IF NOT EXISTS monitor_policies(project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,platform TEXT NOT NULL,interval_seconds INTEGER NOT NULL,paused INTEGER NOT NULL DEFAULT 0,next_check_at TEXT NOT NULL,last_checked_at TEXT NOT NULL DEFAULT '',last_error TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,service),FOREIGN KEY(project) REFERENCES registrations(name))",
 }{if _,err=db.ExecContext(ctx,q);err!=nil{db.Close();return nil,fmt.Errorf("sqlite init: %w",err)}}
 // Safe additive migration of older persisted inventory databases.
 rows,err:=db.QueryContext(ctx,"PRAGMA table_info(inventory)")
 if err!=nil{db.Close();return nil,err}
 columns:=make(map[string]bool)
 for rows.Next(){var id int;var name,typ string;var notnull,pk int;var def sql.NullString
  if err=rows.Scan(&id,&name,&typ,&notnull,&def,&pk);err!=nil{break}
  columns[name]=true
 }
 if scanErr:=rows.Err();err==nil{err=scanErr}
 rows.Close()
 if err!=nil{db.Close();return nil,err}
 for _,column:=range []string{"repo_digests_json","image_os","image_architecture","image_variant"}{
  if columns[column]{continue}
  if _,err=db.ExecContext(ctx,"ALTER TABLE inventory ADD COLUMN "+column+" TEXT NOT NULL DEFAULT ''");err!=nil{db.Close();return nil,err}
 }
 // Persist the backoff counter across restarts (older databases lack this column).
 policyRows,err:=db.QueryContext(ctx,"PRAGMA table_info(monitor_policies)")
 if err!=nil{db.Close();return nil,err}
 hasFailures:=false
 for policyRows.Next(){
  var id,required,pk int;var name,kind string;var def sql.NullString
  if err=policyRows.Scan(&id,&name,&kind,&required,&def,&pk);err!=nil{break}
  if name=="consecutive_failures"{hasFailures=true}
 }
 if err==nil{err=policyRows.Err()}
 policyRows.Close()
 if err!=nil{db.Close();return nil,err}
 if !hasFailures{
  if _,err=db.ExecContext(ctx,"ALTER TABLE monitor_policies ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0");err!=nil{db.Close();return nil,err}
 }
 return &Store{db:db},nil
}
func(s *Store)Close()error{return s.db.Close()}
func(s *Store)Register(ctx context.Context,p Registration)error{
 if p.Name==""||p.ProjectDir==""||len(p.Files)==0||len(p.Services)==0{return errors.New("incomplete registration")}
 files,_:=json.Marshal(p.Files);env,_:=json.Marshal(p.EnvFiles);services,_:=json.Marshal(p.Services);profiles,_:=json.Marshal(p.Profiles)
 _,err:=s.db.ExecContext(ctx,"INSERT INTO registrations(name,project_dir,files_json,env_files_json,services_json,profiles_json,enabled,created_at) VALUES(?,?,?,?,?,?,0,?)",p.Name,p.ProjectDir,string(files),string(env),string(services),string(profiles),time.Now().UTC().Format(time.RFC3339))
 return err
}
func(s *Store)Projects(ctx context.Context)([]Registration,error){
 rows,err:=s.db.QueryContext(ctx,"SELECT name,project_dir,files_json,env_files_json,services_json,profiles_json,enabled,created_at FROM registrations ORDER BY name")
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]Registration{}
 for rows.Next(){
  var p Registration;var f,e,sv,pr string;var enabled int
  if err=rows.Scan(&p.Name,&p.ProjectDir,&f,&e,&sv,&pr,&enabled,&p.CreatedAt);err!=nil{return nil,err}
  if err=json.Unmarshal([]byte(f),&p.Files);err!=nil{return nil,err}
  if err=json.Unmarshal([]byte(e),&p.EnvFiles);err!=nil{return nil,err}
  if err=json.Unmarshal([]byte(sv),&p.Services);err!=nil{return nil,err}
  if err=json.Unmarshal([]byte(pr),&p.Profiles);err!=nil{return nil,err}
  p.Enabled=enabled==1;out=append(out,p)
 }
 return out,rows.Err()
}
func(s *Store)Snapshot(ctx context.Context,items []Container)error{
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,"DELETE FROM inventory");err!=nil{return err}
 stmt,err:=tx.PrepareContext(ctx,"INSERT INTO inventory(container_id,name,project,service,image,image_id,state,health,observed_at,repo_digests_json,image_os,image_architecture,image_variant) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)")
 if err!=nil{return err};defer stmt.Close()
 now:=time.Now().UTC().Format(time.RFC3339)
 for _,c:=range items{
  digests,err:=json.Marshal(c.RepoDigests);if err!=nil{return err}
  if _,err=stmt.ExecContext(ctx,c.ID,c.Name,c.Project,c.Service,c.Image,c.ImageID,c.State,c.Health,now,string(digests),c.ImageOS,c.ImageArchitecture,c.ImageVariant);err!=nil{return err}
 }
 return tx.Commit()
}
func(s *Store)Containers(ctx context.Context)([]Container,error){
 rows,err:=s.db.QueryContext(ctx,"SELECT container_id,name,project,service,image,image_id,state,health,repo_digests_json,image_os,image_architecture,image_variant,observed_at FROM inventory ORDER BY project,service,name")
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]Container{}
 for rows.Next(){var c Container;var digests string;if err=rows.Scan(&c.ID,&c.Name,&c.Project,&c.Service,&c.Image,&c.ImageID,&c.State,&c.Health,&digests,&c.ImageOS,&c.ImageArchitecture,&c.ImageVariant,&c.ObservedAt);err!=nil{return nil,err};if digests!=""{if err=json.Unmarshal([]byte(digests),&c.RepoDigests);err!=nil{return nil,err}};out=append(out,c)}
 return out,rows.Err()
}
