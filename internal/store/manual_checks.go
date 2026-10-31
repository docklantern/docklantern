package store

import (
    "context"
    "database/sql"
    "errors"
    "time"

    "github.com/docklantern/docklantern/internal/registry"
)

// A manual registry check is a durable *schedule request*, not an immediate
// HTTP/network operation. The existing bounded monitor worker services it.
//
// States are stable, safe for display, and intentionally reveal no registry
// URL, credentials or raw SQLite errors.
const (
    CheckScheduled       = "scheduled"
    CheckAlreadyDue      = "already_due"
    CheckDeferredBackoff = "deferred_backoff"
    CheckDeferredRecent  = "deferred_recent"
    CheckNotEligible     = "not_eligible"
)
const manualCheckSpacing = time.Minute
const maxManualImageWatches = 100

type CheckRequestResult struct {
    State string
    Scheduled int
    Eligible int
}

type checkRequestTarget struct {
    project, service, image, platform string
    isImage bool
}

type checkRequestPolicy struct {
    project, service, image, platform string
    nextAt, lastAt, manualAt string
    failures int
}

func (s *Store) RequestServiceCheck(ctx context.Context, project, service string, now time.Time) (CheckRequestResult,error) {
    if project=="" || service=="" || len(project)>128 || len(service)>128 {
        return CheckRequestResult{State:CheckNotEligible},nil
    }
    return s.requestManualCheck(ctx,checkRequestTarget{project:project,service:service},now)
}

func (s *Store) RequestImageCheck(ctx context.Context, image,platform string, now time.Time) (CheckRequestResult,error) {
    // The caller should decode a known ImageID. This additional canonical
    // check refuses aliases, unsupported platforms and arbitrary URI input.
    ref,err:=registry.Parse(image)
    if err!=nil || ref.Key()!=image || !validPlatform(platform) {
        return CheckRequestResult{State:CheckNotEligible},nil
    }
    return s.requestManualCheck(ctx,checkRequestTarget{image:image,platform:platform,isImage:true},now)
}

func checkTime(raw string)(time.Time,bool){
    if raw==""{return time.Time{},false}
    value,e:=time.Parse(time.RFC3339,raw)
    return value,e==nil
}

func (s *Store) requestManualCheck(ctx context.Context, target checkRequestTarget,now time.Time)(CheckRequestResult,error) {
    stamp:=now.UTC().Truncate(time.Second)
    if stamp.IsZero(){return CheckRequestResult{},errors.New("invalid request time")}
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return CheckRequestResult{},err};defer tx.Rollback()

    // Reuse the *same policy eligibility* constraints as DueMonitorPolicies.
    // No arbitrary image, paused watch, opt-out or invalid discovery can be
    // scheduled even if the caller forges an authenticated POST.
    query:=`SELECT p.project,p.service,p.image,p.platform,p.next_check_at,
                   p.last_checked_at,p.last_manual_request_at,p.consecutive_failures
        FROM monitor_policies p
        JOIN monitor_service_settings m
          ON m.project=p.project AND m.service=p.service
        WHERE p.paused=0
          AND (m.label_opt_in IS NULL OR m.label_opt_in!=0)
          AND m.discovery_state!='conflict'
          AND (m.legacy_enrolled=1 OR
              (m.discovery_state='present' AND (m.gui_enrolled=1 OR m.label_opt_in=1)))`
    var args []any
    if target.isImage {
        query+=` AND p.image=? AND p.platform=?`
        args=append(args,target.image,target.platform)
    } else {
        query+=` AND p.project=? AND p.service=?`
        args=append(args,target.project,target.service)
    }
    query+=` ORDER BY p.project,p.service LIMIT 101`
    rows,err:=tx.QueryContext(ctx,query,args...)
    if err!=nil{return CheckRequestResult{},err}
    watches:=[]checkRequestPolicy{}
    for rows.Next(){
        var p checkRequestPolicy
        err=rows.Scan(&p.project,&p.service,&p.image,&p.platform,
            &p.nextAt,&p.lastAt,&p.manualAt,&p.failures)
        if err!=nil{break}
        watches=append(watches,p)
    }
    rowErr:=rows.Err()
    rows.Close()
    if err!=nil{return CheckRequestResult{},err}
    if rowErr!=nil{return CheckRequestResult{},rowErr}
    if len(watches)==0{return CheckRequestResult{State:CheckNotEligible},nil}
    if len(watches)>maxManualImageWatches{return CheckRequestResult{State:CheckNotEligible},nil}

    // Image-scoped request cooldown covers ALL watches sharing that exact
    // image/platform. A separate POST for service B cannot cause another
    // registry request immediately after service A.
    var groupLast sql.NullString
    err=tx.QueryRowContext(ctx,
        `SELECT MAX(last_manual_request_at) FROM monitor_policies
          WHERE image=? AND platform=?`,watches[0].image,watches[0].platform).
        Scan(&groupLast)
    if err!=nil{return CheckRequestResult{},err}
    if groupLast.Valid && groupLast.String!="" {
        previous,ok:=checkTime(groupLast.String)
        if !ok || stamp.Sub(previous)<manualCheckSpacing {
            return CheckRequestResult{State:CheckDeferredRecent,Eligible:len(watches)},nil
        }
    }

    count:=0
    due:=false
    backoff:=false
    recent:=false
    for _,p:=range watches {
        next,ok:=checkTime(p.nextAt)
        if !ok{return CheckRequestResult{},errors.New("invalid persisted schedule")}
        if !next.After(stamp){due=true;continue}
        if p.failures>0 {backoff=true;continue}
        if last,valid:=checkTime(p.lastAt);p.lastAt!=""&&(!valid||stamp.Sub(last)<manualCheckSpacing){
            recent=true;continue
        }
        // CAS on the pre-read next_check_at prevents changing work that
        // has been rescheduled by another transaction.
        result,e:=tx.ExecContext(ctx,`UPDATE monitor_policies
          SET next_check_at=?,last_manual_request_at=?,generation=generation+1
          WHERE project=? AND service=? AND next_check_at=?
            AND paused=0 AND consecutive_failures=0`,
          stamp.Format(time.RFC3339),stamp.Format(time.RFC3339),
          p.project,p.service,p.nextAt)
        if e!=nil{return CheckRequestResult{},e}
        n,e:=result.RowsAffected();if e!=nil{return CheckRequestResult{},e}
        if n!=1{return CheckRequestResult{},errors.New("monitor schedule changed concurrently")}
        count++
    }
    if count>0 {
        if err=tx.Commit();err!=nil{return CheckRequestResult{},err}
        return CheckRequestResult{State:CheckScheduled,Eligible:len(watches),Scheduled:count},nil
    }
    // No write was necessary: existing due checks are already eligible for
    // the worker, while backed-off checks retain their Retry-After budget.
    state:=CheckNotEligible
    switch {
    case due: state=CheckAlreadyDue
    case backoff: state=CheckDeferredBackoff
    case recent: state=CheckDeferredRecent
    }
    return CheckRequestResult{State:state,Eligible:len(watches)},nil
}
