package executorapply

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "os"
 "os/exec"
 "reflect"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/executorhost"
 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrNativeEffect=errors.New("guarded host Docker operation unavailable")

// NativeCommand is never supplied by the monitor, web UI or proposal.
// Every command is constructed by this package from an already protected
// registration and preflight qualification.
type nativeCommand struct{ args []string; stdin []byte; limit int }
type nativeRunner interface{run(context.Context,nativeCommand)([]byte,error)}
type dockerNativeRunner struct{}

func (dockerNativeRunner)run(ctx context.Context,c nativeCommand)([]byte,error){
 if os.Geteuid()!=0||ctx==nil||ctx.Err()!=nil||c.limit<1||c.limit>65536||
  !approvedNativeArgs(c.args)||len(c.stdin)>8192||
  executorhost.ValidateTrustedExecutable(executorhost.RealFS{},"/usr/bin/docker")!=nil{
  return nil,ErrNativeEffect
 }
 runctx,cancel:=context.WithTimeout(ctx,90*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(runctx,"/usr/bin/docker",c.args...)
 cmd.Dir="/"
 cmd.Env=executorhost.MinimalChildEnvironment(nil)
 cmd.Stdin=bytes.NewReader(c.stdin)
 var output bytes.Buffer
 cmd.Stdout=&limitWriter{dest:&output,remain:c.limit}
 cmd.Stderr=io.Discard
 cmd.WaitDelay=time.Second
 if cmd.Run()!=nil||runctx.Err()!=nil{return nil,ErrNativeEffect}
 return output.Bytes(),nil
}
type limitWriter struct{dest *bytes.Buffer;remain int}
func(w *limitWriter)Write(b []byte)(int,error){
 if len(b)>w.remain{return 0,ErrNativeEffect}
 w.remain-=len(b)
 return w.dest.Write(b)
}

func approvedNativeArgs(args []string)bool{
 if len(args)<4||args[0]!="--host"||args[1]!=executorhost.LocalSocket{return false}
 switch args[2]{
 case "version":
  return len(args)==5&&args[3]=="--format"&&args[4]=="{{json .Server}}"
 case "pull":
  return len(args)==4&&approvedDigestImage(args[3])
 case "image":
  return len(args)==7&&args[3]=="inspect"&&args[4]=="--format"&&
   args[5]=="{{.Id}}"&&approvedDigestImage(args[6])
 case "ps":
  return len(args)==9&&args[3]=="--all"&&args[4]=="--no-trunc"&&
   args[5]=="--filter"&&strings.HasPrefix(args[6],"label=com.docker.compose.project=")&&
   executorregistration.ValidID(strings.TrimPrefix(args[6],"label=com.docker.compose.project="))&&
   args[7]=="--format"&&args[8]=="{{.ID}}"
 case "inspect":
  return len(args)==7&&args[3]=="--format"&&args[4]=="{{json .}}"&&
   args[5]=="--"&&validContainerHex(args[6])
 case "compose":
  return approvedNativeCompose(args)
 }
 return false
}
func approvedDigestImage(s string)bool{
 parts:=strings.Split(s,"@")
 return len(parts)==2&&executorregistration.ValidRepository(parts[0])&&sameDigest(parts[1])
}
func validContainerHex(s string)bool{
 if len(s)!=64{return false}
 for _,c:=range s{if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}}
 return true
}
func approvedNativeCompose(args []string)bool{
 // Exact ordered fixed flags: caller must not add arbitrary '--' options,
 // service names or developer profile paths after the typed construction.
 if len(args)<18||args[3]!="--project-directory"||
  args[5]!="-p"||!executorregistration.ValidID(args[6]){
  return false
 }
 end:=len(args)
 if !executorregistration.ValidID(args[end-1])||
  args[end-2]!="-d"||args[end-3]!="--force-recreate"||
  args[end-4]!="never"||args[end-5]!="--pull"||
  args[end-6]!="--no-build"||args[end-7]!="--no-deps"||
  args[end-8]!="up"{return false}
 seenCompose,seenStdin:=0,0
 i:=7
 for i<end-8 {
  switch args[i]{
  case "-f":
   if i+1>=end-8{return false}
   if args[i+1]=="-"{seenStdin++}else if !strings.HasPrefix(args[i+1],executorpreflight.HostSourceSnapshotsDirectory+"/"){return false}
   seenCompose++;i+=2
  case "--env-file":
   if i+1>=end-8||!strings.HasPrefix(args[i+1],executorpreflight.HostSourceSnapshotsDirectory+"/"){return false}
   i+=2
  case "--profile":
   if i+1>=end-8||!executorregistration.ValidID(args[i+1]){return false}
   i+=2
  default:return false
  }
 }
 return seenCompose>=2&&seenStdin==1
}

type nativeObserved struct{
 ID string `json:"Id"`
 Image string `json:"Image"`
 Config struct{Labels map[string]string `json:"Labels"`} `json:"Config"`
 NetworkSettings json.RawMessage `json:"NetworkSettings"`
 Mounts json.RawMessage `json:"Mounts"`
 State struct{
  Running bool `json:"Running"`
  Health *struct{Status string `json:"Status"`} `json:"Health"`
 } `json:"State"`
}
func decodeNativeObserved(data []byte)(nativeObserved,error){
 var found nativeObserved
 dec:=json.NewDecoder(bytes.NewReader(data))
 if dec.Decode(&found)!=nil{return found,ErrNativeEffect}
 var trailing any
 if dec.Decode(&trailing)!=io.EOF||!validContainerHex(found.ID)||!sameDigest(found.Image){
  return nativeObserved{},ErrNativeEffect
 }
 return found,nil
}

// HostEffects implements the narrow one-target, immutable-image change.
// Construction is separate from CLI authorization; this object never asks
// for a terminal approval and must only run under RunLockedCandidate.
type HostEffects struct{
 reg executorregistration.Registration
 service,repository,digest string
 sourceHash,baselineContainer,baselineImage string
 runner nativeRunner
 handoff *executorpreflight.SourceHandoff
 pulledImageID string
 before map[string]nativeObserved
 verified bool
}
func newHostEffects(reg executorregistration.Registration,q executorpreflight.Qualification,repository string,runner nativeRunner)(*HostEffects,error){
 // beta.6 never self-updates DockLantern, its socket proxy or Traefik.
 // Those infrastructure services need an independently designed recovery path.
 if reg.ProjectName=="docklantern"||reg.ProjectName=="traefik"||
  reg.ProjectName=="docker-socket-proxy"||
  q.Service=="docklantern"||q.Service=="traefik"||
  q.Service=="docker-socket-proxy"||
  reg.Validate()!=nil||!reg.AllowsTarget(q.Service,repository,executorregistration.Platform{OS:"linux",Architecture:"amd64"})||
  !sameDigest(q.SourceSHA256)||!sameDigest(q.CandidateDigest)||
  !validContainerHex(q.ContainerID)||!sameDigest(q.ImageID)||
  runner==nil{return nil,ErrNativeEffect}
 return &HostEffects{reg:reg,service:q.Service,repository:repository,digest:q.CandidateDigest,
  sourceHash:q.SourceSHA256,baselineContainer:q.ContainerID,baselineImage:q.ImageID,runner:runner},nil
}
func (h *HostEffects)run(ctx context.Context,args []string,stdin []byte,limit int)([]byte,error){
 if h==nil||h.runner==nil{return nil,ErrNativeEffect}
 return h.runner.run(ctx,nativeCommand{args:append([]string{"--host",executorhost.LocalSocket},args...),stdin:stdin,limit:limit})
}

// requirePinnedHostVersion checks the exact beta.6 test-VPS Docker Engine
// and API contract. A different runner version fails closed until a separate
// compatibility acceptance is explicitly reviewed.
func (h *HostEffects)requirePinnedHostVersion(ctx context.Context)error{
 out,err:=h.run(ctx,[]string{"version","--format","{{json .Server}}"},nil,2048)
 if err!=nil{return ErrNativeEffect}
 var server struct{
  Version string `json:"Version"`
  API string `json:"ApiVersion"`
  OS string `json:"Os"`
  Arch string `json:"Arch"`
 }
 if json.Unmarshal(out,&server)!=nil||server.Version!="29.8.1"||
  server.API!="1.56"||server.OS!="linux"||server.Arch!="amd64"{
  return ErrNativeEffect
 }
 return nil
}

func(h *HostEffects)ReviewApply(_ context.Context,_ executorjournal.Operation,_ executorpreflight.Qualification)error{
 // The independent gate, not this environment, owns human approval.
 return ErrNativeEffect
}
func(h *HostEffects)Requalify(ctx context.Context,_ executorjournal.Operation,q executorpreflight.Qualification)error{
 if ctx==nil||ctx.Err()!=nil||h==nil||q.Service!=h.service||
  q.CandidateDigest!=h.digest||q.SourceSHA256!=h.sourceHash||
  q.ContainerID!=h.baselineContainer||q.ImageID!=h.baselineImage{
  return ErrNativeEffect
 }
 return nil
}
func(h *HostEffects)PullImmutable(ctx context.Context,_ executorjournal.Operation)error{
 if h==nil||h.pulledImageID!=""||ctx==nil||ctx.Err()!=nil||h.requirePinnedHostVersion(ctx)!=nil{return ErrNativeEffect}
 image:=h.repository+"@"+h.digest
 if _,err:=h.run(ctx,[]string{"pull",image},nil,8192);err!=nil{return ErrNativeEffect}
 output,err:=h.run(ctx,[]string{"image","inspect","--format","{{.Id}}",image},nil,256)
 if err!=nil{return ErrNativeEffect}
 id:=strings.TrimSpace(string(output))
 if !sameDigest(id){return ErrNativeEffect}
 h.pulledImageID=id
 return nil
}

func(h *HostEffects)captureProject(ctx context.Context)(map[string]nativeObserved,error){
 if h==nil||ctx==nil||ctx.Err()!=nil{return nil,ErrNativeEffect}
 data,err:=h.run(ctx,[]string{"ps","--all","--no-trunc","--filter",
  "label=com.docker.compose.project="+h.reg.ProjectName,"--format","{{.ID}}"},nil,4096)
 if err!=nil{return nil,ErrNativeEffect}
 result:=map[string]nativeObserved{}
 foundTarget:=false
 for _,id:=range strings.Fields(string(data)){
  if !validContainerHex(id)||len(result)>=32{return nil,ErrNativeEffect}
  raw,e:=h.run(ctx,[]string{"inspect","--format","{{json .}}","--",id},nil,65536)
  if e!=nil{return nil,ErrNativeEffect}
  found,e:=decodeNativeObserved(raw)
  if e!=nil||found.ID!=id||
   found.Config.Labels["com.docker.compose.project"]!=h.reg.ProjectName{
   return nil,ErrNativeEffect
  }
  service:=found.Config.Labels["com.docker.compose.service"]
  if !executorregistration.ValidID(service){return nil,ErrNativeEffect}
  if service==h.service{
   if foundTarget{return nil,ErrNativeEffect}
   foundTarget=true
  }
  result[id]=found
 }
 if !foundTarget{return nil,ErrNativeEffect}
 return result,nil
}
func(h *HostEffects)StageOverride(ctx context.Context,_ executorjournal.Operation)error{
 if h==nil||h.handoff!=nil||h.pulledImageID==""||ctx==nil||ctx.Err()!=nil{
  return ErrNativeEffect
 }
 stage,err:=executorpreflight.StageTrustedSources(h.reg)
 if err!=nil{return ErrNativeEffect}
 h.handoff=stage // always retain and clean this handle on later errors
 if stage.SourceSHA256()!=h.sourceHash||stage.RecheckOriginal(h.reg)!=nil{
  return ErrNativeEffect
 }
 before,err:=h.captureProject(ctx)
 if err!=nil{return ErrNativeEffect}
 base,ok:=before[h.baselineContainer]
 if !ok||base.Image!=h.baselineImage||!base.State.Running||
  base.Config.Labels["com.docker.compose.service"]!=h.service{
  return ErrNativeEffect
 }
 h.before=before
 return nil
}
func(h *HostEffects)ApplyTargetOnly(ctx context.Context,_ executorjournal.Operation)error{
 if h==nil||h.handoff==nil||h.before==nil||h.pulledImageID==""||
  ctx==nil||ctx.Err()!=nil||h.handoff.RecheckOriginal(h.reg)!=nil{
  return ErrNativeEffect
 }
 current,err:=h.captureProject(ctx)
 if err!=nil||!sameNativeProject(h.before,current){
  return ErrNativeEffect
 }
 sources,envs,err:=h.handoff.FilesForReadOnlyCompose(h.reg)
 if err!=nil{return ErrNativeEffect}
 image:=h.repository+"@"+h.digest
 // JSON is YAML. No interpolation, shell or untrusted text can become keys.
 override,err:=json.Marshal(map[string]any{
  "services":map[string]any{h.service:map[string]string{"image":image}},
 })
 if err!=nil{return ErrNativeEffect}
 args:=[]string{"compose","--project-directory",h.reg.ProjectDirectory,"-p",h.reg.ProjectName}
 for _,path:=range sources{args=append(args,"-f",path)}
 args=append(args,"-f","-")
 for _,path:=range envs{args=append(args,"--env-file",path)}
 for _,profile:=range h.reg.Profiles{args=append(args,"--profile",profile)}
 args=append(args,"up","--no-deps","--no-build","--pull","never","--force-recreate","-d",h.service)
 if _,err=h.run(ctx,args,override,8192);err!=nil{return ErrNativeEffect}
 return nil
}
func sameNativeProject(a,b map[string]nativeObserved)bool{
 if len(a)!=len(b){return false}
 for id,previous:=range a{
  current,ok:=b[id]
  if !ok||current.ID!=previous.ID||current.Image!=previous.Image||
   current.State.Running!=previous.State.Running||
   !sameNativeLabels(current.Config.Labels,previous.Config.Labels)||
   !reflect.DeepEqual(current.NetworkSettings,previous.NetworkSettings)||
   !reflect.DeepEqual(current.Mounts,previous.Mounts){
   return false
  }
 }
 return true
}
func sameNativeLabels(a,b map[string]string)bool{
 if len(a)!=len(b){return false}
 for k,v:=range a{if b[k]!=v{return false}}
 return true
}
func(h *HostEffects)VerifyTargetAndSiblings(ctx context.Context,_ executorjournal.Operation)error{
 if h==nil||h.handoff==nil||h.before==nil||h.verified||
  ctx==nil||ctx.Err()!=nil||h.handoff.RecheckOriginal(h.reg)!=nil{
  return ErrNativeEffect
 }
 actual,err:=h.captureProject(ctx)
 if err!=nil{return ErrNativeEffect}
 if _,still:=actual[h.baselineContainer];still{return ErrNativeEffect}
 newCount:=0
 for id,item:=range actual{
  if item.Config.Labels["com.docker.compose.service"]==h.service{
   newCount++
   if id==h.baselineContainer||item.Image!=h.pulledImageID||
    !item.State.Running{return ErrNativeEffect}
   allowRunningOnly:=false
   for _,service:=range h.reg.Services{
    if service.Name==h.service{allowRunningOnly=service.AllowRunningOnly}
   }
   if item.State.Health!=nil{
    if item.State.Health.Status!="healthy"{return ErrNativeEffect}
   }else if !allowRunningOnly{return ErrNativeEffect}
  }else{
   previous,exists:=h.before[id]
   if !exists||previous.Image!=item.Image||!previous.State.Running||
    !item.State.Running||
    !sameNativeLabels(previous.Config.Labels,item.Config.Labels)||
    !reflect.DeepEqual(previous.NetworkSettings,item.NetworkSettings)||
    !reflect.DeepEqual(previous.Mounts,item.Mounts){
    return ErrNativeEffect
   }
  }
 }
 if newCount!=1||len(actual)!=len(h.before){return ErrNativeEffect}
 h.verified=true
 return nil
}
func(h *HostEffects)Close()error{
 if h==nil{return ErrNativeEffect}
 if h.handoff==nil{return nil}
 err:=h.handoff.Close()
 h.handoff=nil
 if err!=nil{return ErrNativeEffect}
 return nil
}
var _ Environment=(*HostEffects)(nil)


func(h *HostEffects)VerifiedEvidence(ctx context.Context,op executorjournal.Operation)(executorjournal.VerifiedEvidence,error){
 var empty executorjournal.VerifiedEvidence
 if h==nil||!h.verified||h.handoff==nil||ctx==nil||ctx.Err()!=nil||
  h.handoff.RecheckOriginal(h.reg)!=nil||
  op.ProjectID!=h.reg.ProjectName||op.ServiceID!=h.service||
  op.CandidateDigest!=h.digest{return empty,ErrNativeEffect}
 actual,err:=h.captureProject(ctx)
 if err!=nil||len(actual)!=len(h.before){return empty,ErrNativeEffect}
 nextID:=""
 for id,item:=range actual{
  if item.Config.Labels["com.docker.compose.service"]==h.service{
   if nextID!=""||id==h.baselineContainer||item.Image!=h.pulledImageID||
    !item.State.Running{return empty,ErrNativeEffect}
   nextID=id
  }else{
   prior,exists:=h.before[id]
   if !exists||prior.Image!=item.Image||
    prior.State.Running!=item.State.Running||
    !sameNativeLabels(prior.Config.Labels,item.Config.Labels)||
    !reflect.DeepEqual(prior.NetworkSettings,item.NetworkSettings)||
    !reflect.DeepEqual(prior.Mounts,item.Mounts){
    return empty,ErrNativeEffect
   }
  }
 }
 if nextID==""||h.handoff.RecheckOriginal(h.reg)!=nil{return empty,ErrNativeEffect}
 target:=actual[nextID]
 grade:="running_only"
 if target.State.Health!=nil{
  if target.State.Health.Status!="healthy"{return empty,ErrNativeEffect}
  grade="healthy"
 }else{
  found:=false
  for _,s:=range h.reg.Services{
   if s.Name==h.service&&s.AllowRunningOnly{found=true}
  }
  if !found{return empty,ErrNativeEffect}
 }
 return executorjournal.VerifiedEvidence{
  ProjectID:op.ProjectID,ServiceID:op.ServiceID,
  SourceSHA256:h.sourceHash,CandidateDigest:h.digest,
  BaselineContainerID:h.baselineContainer,BaselineImageID:h.baselineImage,
  UpdatedContainerID:nextID,UpdatedImageID:h.pulledImageID,
  HealthGrade:grade,SiblingsUnchanged:true,
 },nil
}
