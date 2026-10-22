package store
import (
 "context"
 "path/filepath"
 "testing"
)
func TestRegistrationAndSnapshot(t *testing.T){
 ctx:=context.Background()
 s,e:=Open(filepath.Join(t.TempDir(),"nested","inventory.db"));if e!=nil{t.Fatal(e)};defer s.Close()
 p:=Registration{Name:"demo",ProjectDir:"/tmp/demo",Files:[]string{"/tmp/demo/compose.yaml"},Services:[]string{"web"}}
 if e=s.Register(ctx,p);e!=nil{t.Fatal(e)}
 if e=s.Register(ctx,p);e==nil{t.Fatal("duplicate should fail")}
 projects,e:=s.Projects(ctx);if e!=nil||len(projects)!=1||projects[0].Enabled||projects[0].Services[0]!="web"{t.Fatalf("%+v %v",projects,e)}
 items:=[]Container{{ID:"abc",Name:"demo-web-1",Project:"demo",Service:"web",Image:"nginx:1",State:"running"}}
 if e=s.Snapshot(ctx,items);e!=nil{t.Fatal(e)}
 got,e:=s.Containers(ctx);if e!=nil||len(got)!=1||got[0].Image!="nginx:1"{t.Fatalf("%+v %v",got,e)}
 if e=s.Snapshot(ctx,nil);e!=nil{t.Fatal(e)}
 got,e=s.Containers(ctx);if e!=nil||len(got)!=0{t.Fatalf("%+v %v",got,e)}
}
