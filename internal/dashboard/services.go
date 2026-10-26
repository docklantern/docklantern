package dashboard

import (
 "net/url"
 "strconv"
 "strings"

 "github.com/docklantern/docklantern/internal/store"
 "github.com/docklantern/docklantern/internal/updates"
)

// serviceRow is a single Compose project/service, regardless of replica count.
// It is intentionally a read-only projection of inventory, watch provenance,
// remote registry observations, and deployment evidence.
type serviceRow struct {
 Project string
 Service string
 Image string
 Platform string
 Enrollment string
 MonitoringStatus string
 StatusReason string
 EffectiveInterval int
 IntervalSource string
 ContainerState string
 ContainerHealth string
 ContainerCount int
 RunningCount int
 LastInventoryAt string
 Paused bool
 NextCheckAt string
 LastCheckedAt string
 LastError string
 RegistryStatus string
 RegistryCheckedAt string
 AssessmentState string
 AssessmentReason string
 SettingsURL string
 EventsURL string
}

func buildServiceRows(settings []store.MonitorServiceView,containers []store.Container,
 policies []store.MonitorPolicy,checks []store.RegistryCheck,assessments []updates.Assessment) []serviceRow {
 type identity struct{project,service string}
 byPolicy:=make(map[identity]store.MonitorPolicy,len(policies))
 for _,p:=range policies {byPolicy[identity{p.Project,p.Service}]=p}
 byCheck:=make(map[string]store.RegistryCheck,len(checks))
 for _,c:=range checks {byCheck[c.Image+"|"+c.Platform]=c}
 byAssessment:=make(map[identity]updates.Assessment,len(assessments))
 for _,a:=range assessments {byAssessment[identity{a.Project,a.Service}]=a}
 byContainers:=make(map[identity][]store.Container)
 for _,c:=range containers {
  k:=identity{c.Project,c.Service}
  byContainers[k]=append(byContainers[k],c)
 }
 rows:=make([]serviceRow,0,len(settings))
 for _,v:=range settings{
  key:=identity{v.Project,v.Service}
  r:=serviceRow{Project:v.Project,Service:v.Service,Enrollment:v.EnrollmentSource,
   EffectiveInterval:v.EffectiveInterval,IntervalSource:v.IntervalSource,
   Paused:v.Paused,NextCheckAt:v.NextCheckAt,RegistryStatus:"Not checked",
   AssessmentState:"Not assessed",ContainerState:"Not present",ContainerHealth:"Not reported"}
  settingsQuery:=url.Values{"project":{v.Project},"service":{v.Service}}
  r.SettingsURL="/settings?"+settingsQuery.Encode()+"#service-monitoring"
  r.EventsURL="/events?search="+url.QueryEscape(v.Service)
  if p,ok:=byPolicy[key];ok{
   r.Image=p.Image;r.Platform=p.Platform
   r.LastCheckedAt=p.LastCheckedAt;r.LastError=p.LastError
   // A registry event has image/platform scope, and may be shared by many
   // services. Search by the exact image identity, not the service name.
   r.EventsURL="/events?search="+url.QueryEscape(p.Image)
   if check,ok:=byCheck[p.Image+"|"+p.Platform];ok{
    r.RegistryCheckedAt=check.CheckedAt
    switch check.Status {
    case "first_seen":r.RegistryStatus="Remote tag first observed"
    case "changed":r.RegistryStatus="Remote digest changed"
    case "unchanged":r.RegistryStatus="Remote digest unchanged"
    default:r.RegistryStatus=check.Status
    }
   }
   if a,ok:=byAssessment[key];ok{
    r.AssessmentState=strings.ReplaceAll(a.State,"_"," ")
    r.AssessmentReason=a.Reason
   }
  }
  observed:=byContainers[key]
  r.ContainerCount=len(observed)
  health:=""
  for _,c:=range observed{
   if r.Image==""{r.Image=c.Image}
   if c.State=="running"{r.RunningCount++}
   if health==""{health=c.Health}else if c.Health!=health{health="Mixed / not reported"}
   if c.ObservedAt>r.LastInventoryAt{r.LastInventoryAt=c.ObservedAt}
  }
  if health!=""{r.ContainerHealth=health}
  if r.ContainerCount>0{
   switch {
   case r.RunningCount==r.ContainerCount:r.ContainerState="Running ("+strconv.Itoa(r.RunningCount)+"/"+strconv.Itoa(r.ContainerCount)+")"
   case r.RunningCount==0:r.ContainerState="Stopped ("+strconv.Itoa(r.ContainerCount)+" containers)"
   default:r.ContainerState="Partially running ("+strconv.Itoa(r.RunningCount)+"/"+strconv.Itoa(r.ContainerCount)+")"
   }
  }
  // Do not confuse the registry's digest movement with a verified newer
  // application version, nor a webhook pending status with a failed check.
  switch {
  case v.LabelOptIn!=nil&&!*v.LabelOptIn:
   r.MonitoringStatus="Disabled by label";r.StatusReason="Explicit com.docklantern.monitor=false"
  case v.State=="conflict":
   r.MonitoringStatus="Invalid discovery";r.StatusReason=v.ValidationError
  case v.Paused:
   r.MonitoringStatus="Paused";r.StatusReason="Monitoring paused by administrator"
  case v.EnrollmentSource=="Not monitored":
   r.MonitoringStatus="Not monitored";r.StatusReason="Opt in using a Docker label or Settings"
  case v.State=="missing"&&!v.LegacyEnrolled:
   r.MonitoringStatus="Not running";r.StatusReason="No running Compose service in the last complete scan"
  case !v.HasPolicy:
   r.MonitoringStatus="Not scheduled";r.StatusReason="No active monitoring policy"
  default:
   r.MonitoringStatus="Monitoring"
   if v.LegacyEnrolled&&v.State=="missing"{r.StatusReason="Legacy manual monitoring; no running container recorded"}
  }
  if r.MonitoringStatus=="Monitoring"&&r.LastError!=""{
   r.MonitoringStatus="Registry check failed";r.StatusReason=r.LastError
  }
  rows=append(rows,r)
 }
 return rows
}
