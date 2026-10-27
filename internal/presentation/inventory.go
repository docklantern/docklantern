package presentation

import (
    "encoding/base64"
    "errors"
    "sort"
    "strings"
    "time"

    "github.com/docklantern/docklantern/internal/store"
    "github.com/docklantern/docklantern/internal/updates"
)

// ImageIdentity mirrors beta.3's persisted registry observation identity.
// Credential-scoped identities are not stored in beta.3 and must be modeled
// explicitly if future registries use multiple authorization scopes.
type ImageIdentity struct {
    Reference string
    Platform string
}

// ImageID is a stable URL-safe identifier, not an authorization token.
// It encodes only reference/platform information exposed by inventory APIs.
func ImageID(id ImageIdentity) string {
    return base64.RawURLEncoding.EncodeToString([]byte(id.Reference+"\x00"+id.Platform))
}

func ParseImageID(s string) (ImageIdentity, error) {
    if s == "" || len(s) > 8192 {
        return ImageIdentity{}, errors.New("invalid image ID")
    }
    decoded, err := base64.RawURLEncoding.DecodeString(s)
    if err != nil || len(decoded) > 4096 {
        return ImageIdentity{}, errors.New("invalid image ID")
    }
    parts := strings.Split(string(decoded), "\x00")
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" || ImageID(ImageIdentity{parts[0],parts[1]}) != s {
        return ImageIdentity{}, errors.New("invalid image ID")
    }
    return ImageIdentity{Reference:parts[0], Platform:parts[1]}, nil
}

type ServiceIdentity struct {
    Project string
    Service string
}

type DeploymentCounts struct {
    Current int
    Different int
    Unknown int
    NotRunning int
}

type ImageSummary struct {
    ID string
    Reference string
    Platform string
    Services []ServiceIdentity
    RegistryStatus string // not_checked, first_seen, changed, unchanged, stale, check_failed
    LatestObservation *store.RegistryCheck
    LastChangeAt string
    FailedServices int
    DeploymentStatus string // current, different, mixed, unknown, not_running
    DeploymentCounts DeploymentCounts
}

type ServiceSummary struct {
    ServiceIdentity
    Image string
    Platform string
    ImageID string
    DiscoveredImage string
    MixedDiscoveredImages bool
    InventoryObservedAt string
    Enrollment string
    DiscoveryState string
    ValidationError string
    GUIOverride *int
    LabelInterval *int
    LabelOptIn *bool
    Monitoring string
    IntervalSeconds int
    IntervalSource string
    Paused bool
    HasPolicy bool
    NextCheckAt string
    LastCheckedAt string
    LastError string
    Assessment string
    ContainerCount int
    RunningCount int
    ContainerHealth string
    CanGUIEnable bool
    CanGUIDisable bool
    CanPause bool
    CanResume bool
    CanEditInterval bool
}

// BuildImageSummaries groups monitored references using read-only stored
// observations, never by performing new remote requests. The last digest
// change is derived from event history, not the most recent check status.
func BuildImageSummaries(policies []store.MonitorPolicy, checks []store.RegistryCheck, assessments []updates.Assessment, events []store.Event, now time.Time) []ImageSummary {
    byCheck := make(map[ImageIdentity]store.RegistryCheck,len(checks))
    for _,c := range checks { byCheck[ImageIdentity{c.Image,c.Platform}]=c }
    byAssessment := make(map[ServiceIdentity]updates.Assessment,len(assessments))
    for _,a := range assessments { byAssessment[ServiceIdentity{a.Project,a.Service}]=a }
    lastChange := make(map[ImageIdentity]string)
    lastChangeTime := make(map[ImageIdentity]time.Time)
    for _,e := range events {
        if e.Kind!="remote_digest_changed" || e.Image=="" || e.Platform=="" { continue }
        at,err := time.Parse(time.RFC3339Nano,e.OccurredAt)
        if err!=nil { continue }
        id := ImageIdentity{e.Image,e.Platform}
        if at.After(lastChangeTime[id]) { lastChangeTime[id]=at;lastChange[id]=e.OccurredAt }
    }
    // An observation should not be called stale before its monitoring policy
    // would reasonably have rechecked it. The fastest active watcher sets the
    // group's expected cadence; a two-hour floor tolerates retries and polling
    // jitter for the default five-minute interval.
    freshness := make(map[ImageIdentity]time.Duration)
    for _,p := range policies {
        if p.Paused || p.Image=="" || p.Platform=="" || p.IntervalSeconds<=0 {continue}
        id := ImageIdentity{p.Image,p.Platform}
        window := 2*time.Duration(p.IntervalSeconds)*time.Second
        if window < 2*time.Hour {window=2*time.Hour}
        if old,found := freshness[id]; !found || window<old {freshness[id]=window}
    }
    groups := make(map[ImageIdentity]*ImageSummary)
    for _,p := range policies {
        if p.Image=="" || p.Platform=="" { continue }
        id:=ImageIdentity{p.Image,p.Platform}
        g:=groups[id]
        if g==nil {
            g=&ImageSummary{ID:ImageID(id),Reference:p.Image,Platform:p.Platform,RegistryStatus:"not_checked",LastChangeAt:lastChange[id]}
            if c,ok:=byCheck[id];ok {
                lastSuccessful:=c
                g.LatestObservation=&lastSuccessful
                g.RegistryStatus=c.Status
                window:=freshness[id]
                if window==0 {window=2*time.Hour}
                if !freshRegistry(c.CheckedAt,now,window) {g.RegistryStatus="stale"}
            }
            groups[id]=g
        }
        sid:=ServiceIdentity{p.Project,p.Service}
        g.Services=append(g.Services,sid)
        if p.LastError!="" && !p.Paused {g.FailedServices++}
        a,found:=byAssessment[sid]
        switch {
        case !found:g.DeploymentCounts.Unknown++
        case a.State=="current":g.DeploymentCounts.Current++
        case a.State=="different":g.DeploymentCounts.Different++
        case a.State=="not_running":g.DeploymentCounts.NotRunning++
        default:g.DeploymentCounts.Unknown++
        }
    }
    out:=make([]ImageSummary,0,len(groups))
    for _,g:=range groups {
        if g.FailedServices>0 {g.RegistryStatus="check_failed"} // latest success still stored separately
        g.DeploymentStatus=summarizeDeployments(g.DeploymentCounts)
        sort.Slice(g.Services,func(i,j int)bool {
            a,b:=g.Services[i],g.Services[j]
            if a.Project!=b.Project{return a.Project<b.Project}
            return a.Service<b.Service
        })
        out=append(out,*g)
    }
    sort.Slice(out,func(i,j int)bool{
        if out[i].Reference!=out[j].Reference{return out[i].Reference<out[j].Reference}
        return out[i].Platform<out[j].Platform
    })
    return out
}

func summarizeDeployments(c DeploymentCounts) string {
    total:=c.Current+c.Different+c.Unknown+c.NotRunning
    switch {
    case total==0:return "unknown"
    case c.Current==total:return "current"
    case c.Different==total:return "different"
    case c.NotRunning==total:return "not_running"
    case c.Unknown==total:return "unknown"
    case c.Current==0 && c.Different==0:return "unknown"
    default:return "mixed"
    }
}

func freshRegistry(value string,now time.Time,window time.Duration)bool{
    at,err:=time.Parse(time.RFC3339Nano,value)
    if err!=nil || now.IsZero() || window<=0 {return false}
    age:=now.Sub(at)
    return age>=-5*time.Second && age<=window
}

// BuildServiceSummaries includes discovered but unenrolled Compose services.
// Capability flags originate in the store; authenticated POST handlers must
// continue to enforce them server-side.
func BuildServiceSummaries(views []store.MonitorServiceView,containers []store.Container,policies []store.MonitorPolicy,assessments []updates.Assessment) []ServiceSummary {
    byPolicy:=make(map[ServiceIdentity]store.MonitorPolicy,len(policies))
    for _,p:=range policies{byPolicy[ServiceIdentity{p.Project,p.Service}]=p}
    byAssessment:=make(map[ServiceIdentity]updates.Assessment,len(assessments))
    for _,a:=range assessments{byAssessment[ServiceIdentity{a.Project,a.Service}]=a}
    byContainers:=make(map[ServiceIdentity][]store.Container)
    for _,c:=range containers{
        id:=ServiceIdentity{c.Project,c.Service}
        byContainers[id]=append(byContainers[id],c)
    }
    out:=make([]ServiceSummary,0,len(views))
    for _,v:=range views{
        id:=ServiceIdentity{v.Project,v.Service}
        s:=ServiceSummary{
            ServiceIdentity:id,Image:v.Image,Platform:v.Platform,
            Enrollment:v.EnrollmentSource,DiscoveryState:v.State,ValidationError:v.ValidationError,
            GUIOverride:v.GUIOverride,LabelInterval:v.LabelInterval,LabelOptIn:v.LabelOptIn,Monitoring:"not_enrolled",
            IntervalSeconds:v.EffectiveInterval,IntervalSource:v.IntervalSource,
            Paused:v.Paused,HasPolicy:v.HasPolicy,NextCheckAt:v.NextCheckAt,
            Assessment:"unknown",ContainerHealth:"not_reported",
            CanGUIEnable:v.CanGUIEnable,CanGUIDisable:v.CanGUIDisable,
            CanPause:v.CanPause,CanResume:v.CanResume,CanEditInterval:v.CanEditInterval,
        }
        if p,ok:=byPolicy[id];ok{
            s.Image,s.Platform,s.LastCheckedAt,s.LastError=p.Image,p.Platform,p.LastCheckedAt,p.LastError
        }
        if a,ok:=byAssessment[id];ok{s.Assessment=a.State}
        health:=""
        healthSeen:=false
        for _,c:=range byContainers[id]{
            // Persisted registry watch policy may intentionally refer to an older
            // image (e.g. a legacy manual policy). Keep its identity separate
            // from what Docker actually reported at the latest inventory scan.
            if s.ContainerCount==0 {
                s.DiscoveredImage=c.Image
            } else if s.DiscoveredImage!=c.Image {
                s.MixedDiscoveredImages=true
            }
            if c.ObservedAt>s.InventoryObservedAt {s.InventoryObservedAt=c.ObservedAt}
            s.ContainerCount++
            if c.State=="running"{s.RunningCount++}
            if s.Image==""{s.Image=c.Image}
            if s.Platform=="" && c.ImageOS!="" && c.ImageArchitecture!=""{
                s.Platform=c.ImageOS+"/"+c.ImageArchitecture
                if c.ImageVariant!=""{s.Platform+="/"+c.ImageVariant}
            }
            if !healthSeen{health=c.Health;healthSeen=true
            } else if health!=c.Health{health="mixed"}
        }
        if health!=""{s.ContainerHealth=health}
        if v.HasPolicy && s.Image!="" && s.Platform!=""{
            s.ImageID=ImageID(ImageIdentity{s.Image,s.Platform})
        }
        switch {
        case v.LabelOptIn!=nil && !*v.LabelOptIn:s.Monitoring="disabled_by_label"
        case v.State=="conflict":s.Monitoring="invalid_discovery"
        case v.Paused:s.Monitoring="paused"
        case v.EnrollmentSource=="Not monitored":s.Monitoring="not_enrolled"
        case v.State=="missing" && !v.LegacyEnrolled:s.Monitoring="not_running"
        case !v.HasPolicy:s.Monitoring="not_scheduled"
        case s.LastError!="":s.Monitoring="check_failed"
        default:s.Monitoring="monitoring"
        }
        out=append(out,s)
    }
    sort.Slice(out,func(i,j int)bool{
        if out[i].Project!=out[j].Project{return out[i].Project<out[j].Project}
        return out[i].Service<out[j].Service
    })
    return out
}
