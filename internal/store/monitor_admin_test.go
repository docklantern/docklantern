package store

import (
 "context"
 "errors"
 "path/filepath"
 "testing"
 "time"
)

func ptrInt(v int)*int{return &v}

func getOnePolicy(t *testing.T,s *Store)MonitorPolicy{
 t.Helper()
 list,err:=s.MonitorPolicies(context.Background())
 if err!=nil||len(list)!=1{t.Fatalf("want 1 policy: %+v %v",list,err)}
 return list[0]
}

func TestLiveGlobalDefaultInheritedAndLabelWins(t *testing.T){
 ctx:=context.Background()
 s:=newLabelTestStore(t)
 c:=testLabelContainer("test","web","nginx:latest",map[string]string{labelMonitor:"true"})
 doSnapshot(t,s,c)
 p:=getOnePolicy(t,s)
 if p.IntervalSeconds!=300{t.Fatalf("default %d",p.IntervalSeconds)}
 if err:=s.FinishMonitorCheck(ctx,p,time.Now().UTC(),nil);err!=nil{t.Fatal(err)}
 old:=getOnePolicy(t,s)
 if err:=s.SetDefaultMonitorInterval(ctx,120);err!=nil{t.Fatal(err)}
 newer:=getOnePolicy(t,s)
 if newer.IntervalSeconds!=120||newer.NextCheckAt==old.NextCheckAt||newer.Generation<=old.Generation{
  t.Fatalf("default did not reschedule: old %+v new %+v",old,newer)
 }
 if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("change should become promptly due")}
 if err:=s.FinishMonitorCheck(ctx,old,time.Now().UTC(),errors.New("obsolete"));err!=nil{t.Fatal(err)}
 if p=getOnePolicy(t,s);p.ConsecutiveFailures!=0||p.IntervalSeconds!=120{t.Fatalf("stale in-flight completion: %+v",p)}
 doSnapshot(t,s,c)
 if p=getOnePolicy(t,s);p.IntervalSeconds!=120{t.Fatalf("reconciliation changed default: %+v",p)}
 c.MonitorLabels[labelInterval]="10m"
 doSnapshot(t,s,c)
 if p=getOnePolicy(t,s);p.IntervalSeconds!=600{t.Fatalf("label not applied: %+v",p)}
 if err:=s.SetDefaultMonitorInterval(ctx,900);err!=nil{t.Fatal(err)}
 if p=getOnePolicy(t,s);p.IntervalSeconds!=600{t.Fatalf("default overwrote label: %+v",p)}
}

func TestLiveOverrideLabelResetAndPause(t *testing.T){
 ctx:=context.Background()
 s:=newLabelTestStore(t)
 c:=testLabelContainer("test","web","nginx:latest",map[string]string{labelMonitor:"true",labelInterval:"10m"})
 doSnapshot(t,s,c)
 old:=getOnePolicy(t,s)
 if err:=s.SetServiceIntervalOverride(ctx,"test","web",ptrInt(120));err!=nil{t.Fatal(err)}
 p:=getOnePolicy(t,s)
 if p.IntervalSeconds!=120||p.Generation<=old.Generation{t.Fatalf("override not effective %+v",p)}
 if err:=s.FinishMonitorCheck(ctx,old,time.Now().UTC(),nil);err!=nil{t.Fatal(err)}
 if p=getOnePolicy(t,s);p.LastCheckedAt!=""{t.Fatal("stale check completed after override")}
 c.MonitorLabels[labelInterval]="30m"
 doSnapshot(t,s,c)
 if p=getOnePolicy(t,s);p.IntervalSeconds!=120{t.Fatalf("new label overrode GUI: %+v",p)}
 views,err:=s.MonitorServiceViews(ctx)
 if err!=nil||len(views)!=1||views[0].IntervalSource!="GUI override"||views[0].EffectiveInterval!=120{t.Fatalf("UI data %+v %v",views,err)}
 if err:=s.SetServicePaused(ctx,"test","web",true);err!=nil{t.Fatal(err)}
 if p=getOnePolicy(t,s);!p.Paused{t.Fatal("pause ignored")}
 if len(dueAt(t,s,time.Now().Add(time.Hour)))!=0{t.Fatal("paused service still due")}
 if err:=s.SetServiceIntervalOverride(ctx,"test","web",nil);err!=nil{t.Fatal(err)}
 if p=getOnePolicy(t,s);p.IntervalSeconds!=1800{t.Fatalf("reset not label fallback %+v",p)}
 if err:=s.SetServicePaused(ctx,"test","web",false);err!=nil{t.Fatal(err)}
 if p=getOnePolicy(t,s);p.Paused||len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{
  t.Fatalf("resume did not make service due %+v",p)
 }
 views,_=s.MonitorServiceViews(ctx)
 if views[0].IntervalSource!="Docker label"||views[0].EffectiveInterval!=1800{t.Fatalf("wrong reset source %+v",views[0])}
}

func TestGUIEnrollmentWithoutComposeRegistration(t *testing.T){
 ctx:=context.Background()
 path:=filepath.Join(t.TempDir(),"test.sqlite")
 s,err:=Open(path);if err!=nil{t.Fatal(err)}
 c:=testLabelContainer("standalone-compose","web","nginx:latest",nil)
 if err:=s.Snapshot(ctx,[]Container{c});err!=nil{t.Fatal(err)}
 policies,_:=s.MonitorPolicies(ctx)
 if len(policies)!=0{t.Fatal("unlabeled service automatically enrolled")}
 if err:=s.SetServiceIntervalOverride(ctx,c.Project,c.Service,ptrInt(150));err!=nil{t.Fatal(err)}
 policies,_=s.MonitorPolicies(ctx)
 if len(policies)!=0{t.Fatal("override alone enrolled a service")}
 if err:=s.SetServiceGUIEnrollment(ctx,c.Project,c.Service,true);err!=nil{t.Fatal(err)}
 if p:=getOnePolicy(t,s);p.IntervalSeconds!=150||p.Image!="registry-1.docker.io/library/nginx:latest"{
  t.Fatalf("bad GUI policy %+v",p)
 }
 projects,_:=s.Projects(ctx);if len(projects)!=0{t.Fatal("monitoring manufactured source registration")}
 settings,_:=s.MonitoringServiceSettings(ctx)
 if len(settings)!=1||!settings[0].GUIEnrolled||settings[0].LegacyEnrolled{t.Fatalf("lost provenance: %+v",settings)}
 if err:=s.Close();err!=nil{t.Fatal(err)}
 s,err=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
 if p:=getOnePolicy(t,s);p.IntervalSeconds!=150{t.Fatal("GUI configuration not persisted")}
 settings,_=s.MonitoringServiceSettings(ctx);if !settings[0].GUIEnrolled{t.Fatal("GUI enrollment lost after restart")}
 if err:=s.SetServiceGUIEnrollment(ctx,c.Project,c.Service,false);err!=nil{t.Fatal(err)}
 if len(dueAt(t,s,time.Now().Add(time.Minute)))!=0{t.Fatal("disabled GUI watch remained due")}
 if err:=s.SetServiceGUIEnrollment(ctx,c.Project,c.Service,true);err!=nil{t.Fatal(err)}
 if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("GUI watch not reactivated")}
}

func TestNoEnablingServiceBlockedByDockerLabel(t *testing.T){
 ctx:=context.Background()
 s:=newLabelTestStore(t)
 c:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"false"})
 doSnapshot(t,s,c)
 if err:=s.SetServiceGUIEnrollment(ctx,c.Project,c.Service,true);err==nil{t.Fatal("GUI bypassed explicit false")}
 if err:=s.SetServicePaused(ctx,c.Project,c.Service,false);err==nil{t.Fatal("resume bypassed explicit false")}
 if err:=s.SetServiceGUIEnrollment(ctx,"missing","web",true);err==nil{t.Fatal("unknown service enrolled")}
 c.MonitorLabels=nil;c.State="exited"
 doSnapshot(t,s,c)
 if err:=s.SetServiceGUIEnrollment(ctx,c.Project,c.Service,true);err==nil{t.Fatal("stopped service enrolled")}
}

func TestLegacyBaselineSurvivesTemporaryGUIOverride(t *testing.T){
 ctx:=context.Background()
 path:=filepath.Join(t.TempDir(),"beta2.db")
 makeBeta2Fixture(t,path)
 s,err:=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
 policies,_:=s.MonitorPolicies(ctx)
 if len(policies)!=2{t.Fatal("lost migration policies")}
 for _,p:=range policies{
  base:=p.IntervalSeconds
  if err:=s.SetServiceIntervalOverride(ctx,p.Project,p.Service,ptrInt(120));err!=nil{t.Fatal(err)}
  if err:=s.SetServiceIntervalOverride(ctx,p.Project,p.Service,nil);err!=nil{t.Fatal(err)}
  ps,_:=s.MonitorPolicies(ctx)
  for _,got:=range ps{if got.Project==p.Project&&got.IntervalSeconds!=base{t.Fatalf("lost legacy baseline %+v",got)}}
 }
}

func TestDefaultAndOverrideRejectInvalidInputs(t *testing.T){
 ctx:=context.Background();s:=newLabelTestStore(t)
 c:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"true"})
 doSnapshot(t,s,c)
 for _,n:=range []int{0,59,86401}{
  if err:=s.SetDefaultMonitorInterval(ctx,n);err==nil{t.Fatalf("global accepted %d",n)}
  if err:=s.SetServiceIntervalOverride(ctx,c.Project,c.Service,&n);err==nil{t.Fatalf("override accepted %d",n)}
 }
 if err:=s.SetServiceGUIEnrollment(ctx,c.Project,c.Service,true);err==nil{t.Fatal("GUI took ownership of explicit label")}
}
