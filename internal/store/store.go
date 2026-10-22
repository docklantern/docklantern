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
 }{if _,err=db.ExecContext(ctx,q);err!=nil{db.Close();return nil,fmt.Errorf("sqlite init: %w",err)}}
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
 stmt,err:=tx.PrepareContext(ctx,"INSERT INTO inventory(container_id,name,project,service,image,image_id,state,health,observed_at) VALUES(?,?,?,?,?,?,?,?,?)")
 if err!=nil{return err};defer stmt.Close()
 now:=time.Now().UTC().Format(time.RFC3339)
 for _,c:=range items{
  if _,err=stmt.ExecContext(ctx,c.ID,c.Name,c.Project,c.Service,c.Image,c.ImageID,c.State,c.Health,now);err!=nil{return err}
 }
 return tx.Commit()
}
func(s *Store)Containers(ctx context.Context)([]Container,error){
 rows,err:=s.db.QueryContext(ctx,"SELECT container_id,name,project,service,image,image_id,state,health FROM inventory ORDER BY project,service,name")
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]Container{}
 for rows.Next(){var c Container;if err=rows.Scan(&c.ID,&c.Name,&c.Project,&c.Service,&c.Image,&c.ImageID,&c.State,&c.Health);err!=nil{return nil,err};out=append(out,c)}
 return out,rows.Err()
}
