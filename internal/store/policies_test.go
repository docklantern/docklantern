package store
import(
 "context"
 "path/filepath"
 "testing"
 "time"
)
func TestMonitorPolicyRegistrationValidationAndSchedule(t *testing.T){
 ctx:=context.Background();s,e:=Open(filepath.Join(t.TempDir(),"policy.db"));if e!=nil{t.Fatal(e)};defer s.Close()
 p:=MonitorPolicy{Project:"app",Service:"web",Image:"nginx:1.27-alpine",Platform:"linux/amd64",IntervalSeconds:90}
 if e=s.SetMonitorPolicy(ctx,p);e==nil{t.Fatal("unregistered project accepted")}
 if e=s.Register(ctx,Registration{Name:"app",ProjectDir:"/tmp/app",Files:[]string{"/tmp/app/compose.yaml"},Services:[]string{"web"}});e!=nil{t.Fatal(e)}
 p.Service="db";if e=s.SetMonitorPolicy(ctx,p);e==nil{t.Fatal("unregistered service accepted")}
 p.Service="web";p.IntervalSeconds=30;if e=s.SetMonitorPolicy(ctx,p);e==nil{t.Fatal("too frequent interval accepted")}
 p.IntervalSeconds=90;if e=s.SetMonitorPolicy(ctx,p);e!=nil{t.Fatal(e)}
 policies,e:=s.MonitorPolicies(ctx);if e!=nil||len(policies)!=1||policies[0].Image!="registry-1.docker.io/library/nginx:1.27-alpine"{t.Fatalf("%+v %v",policies,e)}
 due,e:=s.DueMonitorPolicies(ctx,time.Now().Add(time.Second),10);if e!=nil||len(due)!=1{t.Fatalf("due %+v %v",due,e)}
 first:=due[0]
 if e=s.FinishMonitorCheck(ctx,first,time.Now().UTC(),nil);e!=nil{t.Fatal(e)}
 due,e=s.DueMonitorPolicies(ctx,time.Now(),10);if e!=nil||len(due)!=0{t.Fatalf("rescheduled %+v %v",due,e)}
 if e=s.SetMonitorPolicy(ctx,MonitorPolicy{Project:"app",Service:"web",Image:"nginx:1.27-alpine",Platform:"linux/amd64",IntervalSeconds:120,Paused:true});e!=nil{t.Fatal(e)}
 paused,e:=s.MonitorPolicies(ctx);if e!=nil||!paused[0].Paused{t.Fatalf("not paused %+v %v",paused,e)}
}
func TestFinishCannotOverwriteChangedPolicy(t *testing.T){
 ctx:=context.Background();s,e:=Open(filepath.Join(t.TempDir(),"db.sqlite"));if e!=nil{t.Fatal(e)};defer s.Close()
 _=s.Register(ctx,Registration{Name:"p",ProjectDir:"/tmp/p",Files:[]string{"/tmp/p/compose.yml"},Services:[]string{"api"}})
 p:=MonitorPolicy{Project:"p",Service:"api",Image:"ghcr.io/p/api:latest",Platform:"linux/amd64",IntervalSeconds:90}
 if e=s.SetMonitorPolicy(ctx,p);e!=nil{t.Fatal(e)}
 due,e:=s.DueMonitorPolicies(ctx,time.Now().Add(time.Second),10);if e!=nil||len(due)!=1{t.Fatal(e)}
 p.Image="ghcr.io/p/api:v2";if e=s.SetMonitorPolicy(ctx,p);e!=nil{t.Fatal(e)}
 if e=s.FinishMonitorCheck(ctx,due[0],time.Now(),nil);e!=nil{t.Fatal(e)}
 checks,e:=s.MonitorPolicies(ctx);if e!=nil||checks[0].LastCheckedAt!=""{t.Fatal("stale check wrote to changed policy")}
}
