package store

import(
 "context"
 "database/sql"
 "path/filepath"
 "reflect"
 "testing"

 _ "modernc.org/sqlite"
)
func TestExistingInventoryMigrationAndProvenance(t *testing.T){
 path:=filepath.Join(t.TempDir(),"existing.db")
 old,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 _,err=old.Exec("CREATE TABLE inventory(container_id TEXT PRIMARY KEY,name TEXT NOT NULL,project TEXT NOT NULL,service TEXT NOT NULL,image TEXT NOT NULL,image_id TEXT NOT NULL,state TEXT NOT NULL,health TEXT NOT NULL,observed_at TEXT NOT NULL)")
 if err!=nil{t.Fatal(err)}
 _,err=old.Exec("INSERT INTO inventory VALUES('old','web','app','web','nginx:latest','sha256:old','running','','2026-01-01T00:00:00Z')")
 if err!=nil{t.Fatal(err)}
 _=old.Close()
 s,err:=Open(path);if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 got,err:=s.Containers(ctx)
 if err!=nil||len(got)!=1||got[0].ID!="old"{t.Fatalf("migration: %+v %v",got,err)}
 expected:=Container{ID:"new",Name:"app-web",Project:"app",Service:"web",Image:"nginx:latest",ImageID:"sha256:abc",ImageOS:"linux",ImageArchitecture:"amd64",RepoDigests:[]string{"docker.io/library/nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
 if err=s.Snapshot(ctx,[]Container{expected});err!=nil{t.Fatal(err)}
 got,err=s.Containers(ctx);if len(got)==1{expected.ObservedAt=got[0].ObservedAt};if err!=nil||len(got)!=1||!reflect.DeepEqual(got[0],expected){t.Fatalf("stored: %+v expected %+v error %v",got,expected,err)}
 _=s.Close()
 reopened,err:=Open(path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 got,err=reopened.Containers(ctx);if err!=nil||len(got)!=1||!reflect.DeepEqual(got[0],expected){t.Fatalf("reopened %+v %v",got,err)}
}
