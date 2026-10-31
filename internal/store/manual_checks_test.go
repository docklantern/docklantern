package store

import (
    "context"
    "errors"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/docklantern/docklantern/internal/registry"
)

func TestManualCheckScheduledAndRestartSafe(t *testing.T){
    ctx:=context.Background()
    path:=filepath.Join(t.TempDir(),"manual-check.sqlite")
    s,err:=Open(path);if err!=nil{t.Fatal(err)}
    c:=testLabelContainer("site","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,c)
    now:=time.Now().UTC().Truncate(time.Second)
    initial:=getOnePolicy(t,s)
    if err:=s.FinishMonitorCheck(ctx,initial,now,nil);err!=nil{t.Fatal(err)}
    if state,err:=s.RequestServiceCheck(ctx,"site","web",now.Add(30*time.Second));err!=nil||state.State!=CheckDeferredRecent{
        t.Fatalf("fresh poll should defer: %+v %v",state,err)
    }
    requested:=now.Add(75*time.Second)
    before:=getOnePolicy(t,s)
    outcome,err:=s.RequestServiceCheck(ctx,"site","web",requested)
    if err!=nil||outcome.State!=CheckScheduled||outcome.Scheduled!=1{
        t.Fatalf("request not scheduled: %+v %v",outcome,err)
    }
    updated:=getOnePolicy(t,s)
    if updated.Image!=before.Image||updated.Platform!=before.Platform||
       updated.Generation<=before.Generation||updated.ConsecutiveFailures!=0||
       updated.NextCheckAt!=requested.Format(time.RFC3339){
        t.Fatalf("incorrect reschedule %+v",updated)
    }
    // A second click must not keep invalidating the scheduler generation.
    outcome,err=s.RequestServiceCheck(ctx,"site","web",requested.Add(5*time.Second))
    if err!=nil||outcome.State!=CheckDeferredRecent{
        t.Fatalf("repeat not throttled: %+v %v",outcome,err)
    }
    if got:=getOnePolicy(t,s).Generation;got!=updated.Generation{
        t.Fatalf("duplicate invalidated generation %d",got)
    }
    if err:=s.Close();err!=nil{t.Fatal(err)}
    s,err=Open(path);if err!=nil{t.Fatal(err)}
    defer s.Close()
    due,err:=s.DueMonitorPolicies(ctx,requested.Add(time.Second),20)
    if err!=nil||len(due)!=1||due[0].Image!=updated.Image{
        t.Fatalf("request did not survive process restart %+v %v",due,err)
    }
    outcome,err=s.RequestServiceCheck(ctx,"site","web",requested.Add(10*time.Second))
    if err!=nil||outcome.State!=CheckDeferredRecent{
        t.Fatalf("restart bypassed cooldown %+v %v",outcome,err)
    }
    if err:=s.FinishMonitorCheck(ctx,before,requested,nil);err!=nil{t.Fatal(err)}
    if p:=getOnePolicy(t,s);p.LastCheckedAt!=before.LastCheckedAt{
        t.Fatal("obsolete in-flight result clobbered manual request")
    }
}

func TestManualCheckBackoffAndPauseAreNeverBypassed(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    c:=testLabelContainer("site","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,c)
    now:=time.Now().UTC().Truncate(time.Second)
    p:=getOnePolicy(t,s)
    failure:=&registry.RetryableError{Status:429,RetryAfter:2*time.Hour}
    if err:=s.FinishMonitorCheck(ctx,p,now,failure);err!=nil{t.Fatal(err)}
    prior:=getOnePolicy(t,s)
    result,err:=s.RequestServiceCheck(ctx,"site","web",now.Add(61*time.Second))
    if err!=nil||result.State!=CheckDeferredBackoff{
        t.Fatalf("retry backoff bypassed %+v %v",result,err)
    }
    after:=getOnePolicy(t,s)
    if after.NextCheckAt!=prior.NextCheckAt||after.Generation!=prior.Generation||
        after.ConsecutiveFailures!=prior.ConsecutiveFailures{t.Fatal("backoff was altered")}
    if err:=s.SetServicePaused(ctx,"site","web",true);err!=nil{t.Fatal(err)}
    result,err=s.RequestServiceCheck(ctx,"site","web",now.Add(2*time.Hour))
    if err!=nil||result.State!=CheckNotEligible{t.Fatalf("paused check accepted %+v %v",result,err)}
    c.MonitorLabels[labelMonitor]="false"
    doSnapshot(t,s,c)
    result,err=s.RequestServiceCheck(ctx,"site","web",now.Add(3*time.Hour))
    if err!=nil||result.State!=CheckNotEligible{t.Fatalf("label opt-out bypass %+v %v",result,err)}
}

func TestManualCheckImageGroupAndCrossServiceCooldown(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    a:=testLabelContainer("a","web","nginx:latest",map[string]string{labelMonitor:"true"})
    b:=testLabelContainer("b","web","nginx:latest",map[string]string{labelMonitor:"true"})
    c:=testLabelContainer("c","web","nginx:latest",map[string]string{labelMonitor:"false"})
    doSnapshot(t,s,a,b,c)
    now:=time.Now().UTC().Truncate(time.Second)
    due,err:=s.DueMonitorPolicies(ctx,now.Add(time.Minute),20)
    if err!=nil||len(due)!=2{t.Fatalf("unexpected watchers %+v %v",due,err)}
    for _,p:=range due{
        if err:=s.FinishMonitorCheck(ctx,p,now,nil);err!=nil{t.Fatal(err)}
    }
    target:=now.Add(65*time.Second)
    outcome,err:=s.RequestImageCheck(ctx,"registry-1.docker.io/library/nginx:latest","linux/amd64",target)
    if err!=nil||outcome.State!=CheckScheduled||outcome.Scheduled!=2||outcome.Eligible!=2{
        t.Fatalf("image group scheduling failed %+v %v",outcome,err)
    }
    due,err=s.DueMonitorPolicies(ctx,target.Add(time.Second),20)
    if err!=nil||len(due)!=2{t.Fatalf("not all eligible watches scheduled %+v %v",due,err)}
    outcome,err=s.RequestServiceCheck(ctx,"a","web",target.Add(5*time.Second))
    if err!=nil||outcome.State!=CheckDeferredRecent{
        t.Fatalf("cross-service cooldown missing %+v %v",outcome,err)
    }
    if x,err:=s.RequestServiceCheck(ctx,"c","web",target);err!=nil||x.State!=CheckNotEligible{
        t.Fatalf("disabled service requested check %+v %v",x,err)
    }
    if x,err:=s.RequestImageCheck(ctx,"nginx:latest","linux/amd64",target);err!=nil||x.State!=CheckNotEligible{
        t.Fatalf("noncanonical image accepted %+v %v",x,err)
    }
    if x,err:=s.RequestImageCheck(ctx,"registry-1.docker.io/library/nginx:latest","linux/arm64",target);err!=nil||x.State!=CheckNotEligible{
        t.Fatalf("unknown platform accepted %+v %v",x,err)
    }
}

func TestManualCheckRequestPreservesUnchangedAndInvalidState(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    c:=testLabelContainer("site","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,c)
    now:=time.Now().UTC().Truncate(time.Second)
    // Initial registration is already due; avoid unnecessary writes.
    initial:=getOnePolicy(t,s)
    outcome,err:=s.RequestServiceCheck(ctx,"site","web",now.Add(time.Second))
    if err!=nil||outcome.State!=CheckAlreadyDue{
        t.Fatalf("already-due watch was modified %+v %v",outcome,err)
    }
    if getOnePolicy(t,s).Generation!=initial.Generation{t.Fatal("generation changed for already due work")}
    if x,err:=s.RequestServiceCheck(ctx,"absent","web",now);err!=nil||x.State!=CheckNotEligible{
        t.Fatalf("unknown service accepted %+v %v",x,err)
    }
    target:="nginx:stable"
    if err:=s.SetServiceWatchOverride(ctx,"site","web",&target);err!=nil{t.Fatal(err)}
    c.Image="redis:latest"
    doSnapshot(t,s,c)
    outcome,err=s.RequestServiceCheck(ctx,"site","web",now.Add(time.Second))
    if err!=nil||outcome.State!=CheckNotEligible{t.Fatalf("conflicting discovered image accepted %+v %v",outcome,err)}
    if len(dueAt(t,s,now.Add(time.Hour)))!=0{t.Fatal("conflict left eligible check")}
    if err:=s.SetServiceWatchOverride(ctx,"site","web",nil);err!=nil{t.Fatal(err)}
    outcome,err=s.RequestServiceCheck(ctx,"site","web",now.Add(time.Second))
    if err!=nil||outcome.State!=CheckAlreadyDue{
        t.Fatalf("validated recovery not eligible %+v %v",outcome,err)
    }
    // Mutating a fake watch cannot create a job.
    if err:=s.SetServiceGUIEnrollment(ctx,"site","web",true);err==nil{
       t.Fatal("explicit label enrollment unexpectedly overridden")
    }
}

func TestManualCheckLegacyAndWorkerIntegration(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    if err:=s.Register(ctx,Registration{Name:"legacy",ProjectDir:"/srv/legacy",
      Files:[]string{"/srv/legacy/compose.yaml"},Services:[]string{"web"}});err!=nil{t.Fatal(err)}
    if err:=s.SetMonitorPolicy(ctx,MonitorPolicy{Project:"legacy",Service:"web",
      Image:"nginx:stable",Platform:"linux/amd64",IntervalSeconds:600});err!=nil{t.Fatal(err)}
    now:=time.Now().UTC().Truncate(time.Second)
    p:=getOnePolicy(t,s)
    if err:=s.FinishMonitorCheck(ctx,p,now,nil);err!=nil{t.Fatal(err)}
    r,err:=s.RequestServiceCheck(ctx,"legacy","web",now.Add(75*time.Second))
    if err!=nil||r.State!=CheckScheduled{t.Fatalf("legacy not eligible %+v %v",r,err)}
    if len(dueAt(t,s,now.Add(76*time.Second)))!=1{t.Fatal("legacy not scheduled")}
}

func TestManualCheckRejectsMalformedScheduleWithoutMutation(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    c:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,c)
    if _,err:=s.db.ExecContext(ctx,`UPDATE monitor_policies SET next_check_at='not-a-date'
        WHERE project='app' AND service='web'`);err!=nil{t.Fatal(err)}
    result,err:=s.RequestServiceCheck(ctx,"app","web",time.Now())
    if err==nil || !strings.Contains(err.Error(),"invalid persisted schedule"){
        t.Fatalf("failed to detect corrupted timestamp %+v %v",result,err)
    }
    if !errors.Is(ctx.Err(),nil){t.Fatal("unexpected context cancellation")}
}

func TestManualCheckGroupBackoffAndRecentAttemptsAreShared(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    a:=testLabelContainer("a","web","nginx:latest",map[string]string{labelMonitor:"true"})
    b:=testLabelContainer("b","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,a,b)
    now:=time.Now().UTC().Truncate(time.Second)
    policies,err:=s.MonitorPolicies(ctx)
    if err!=nil||len(policies)!=2{t.Fatal(err)}
    if err:=s.FinishMonitorCheck(ctx,policies[0],now.Add(-4*time.Minute),
        &registry.RetryableError{Status:429,RetryAfter:3*time.Hour});err!=nil{t.Fatal(err)}
    if err:=s.FinishMonitorCheck(ctx,policies[1],now.Add(-3*time.Minute),nil);err!=nil{t.Fatal(err)}
    // Service B is individually healthy and due in the future, but a
    // sister service's registry 429 prevents a manual group bypass.
    r,err:=s.RequestServiceCheck(ctx,policies[1].Project,"web",now)
    if err!=nil||r.State!=CheckDeferredBackoff{
        t.Fatalf("shared backoff bypassed %+v %v",r,err)
    }
    if len(dueAt(t,s,now))!=0{t.Fatal("shared rate limit caused early check")}
    // Move A's backed-off schedule out of the future to emulate its
    // retry window expiring; a recent ordinary check on B still throttles.
    if _,err:=s.db.ExecContext(ctx,`UPDATE monitor_policies SET next_check_at=?
        WHERE project='a' AND service='web'`,now.Add(-time.Second).Format(time.RFC3339));err!=nil{t.Fatal(err)}
    // With a successful B check only 20 seconds ago, a manual request
    // against A must be deferred as a shared-image cooldown.
    if _,err:=s.db.ExecContext(ctx,`UPDATE monitor_policies SET
        last_checked_at=? WHERE project='b' AND service='web'`,
        now.Add(-20*time.Second).Format(time.RFC3339));err!=nil{t.Fatal(err)}
    r,err=s.RequestServiceCheck(ctx,"b","web",now)
    if err!=nil||r.State!=CheckDeferredRecent{
        t.Fatalf("recent shared observation bypassed %+v %v",r,err)
    }
}
