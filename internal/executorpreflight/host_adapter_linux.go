package executorpreflight

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "os/exec"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/executorhost"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

// ErrHostEvidenceUnavailable never includes source contents, secrets or
// untrusted child output. No Compose/Pull/Up/Apply mutation is implemented.
var ErrHostEvidenceUnavailable = errors.New("read-only host evidence unavailable")

const hostReadTimeout = 12 * time.Second

// hostReadCommand is a narrow read-only command contract; no arbitrary
// subprocess arguments from the proposal or GUI reach this interface.
type hostReadCommand struct {
 args []string
 stdin []byte
 limit int
}
type hostReadRunner interface {
 run(context.Context,hostReadCommand)([]byte,error)
}

// HostReadOnlyAdapter implements source, config, Docker actual and
// restricted anonymous Docker Hub OCI evidence reads. It cannot authorize
// updates or mutate Docker resources.
type HostReadOnlyAdapter struct {
 runner hostReadRunner
 clock func()time.Time
}

// NewHostReadOnlyAdapter validates root and the trusted fixed Docker CLI.
// No monitor, proposal, or environment variable configures the executable.
func NewHostReadOnlyAdapter() (*HostReadOnlyAdapter,error) {
 if os.Geteuid()!=0 ||
  executorhost.ValidateTrustedExecutable(executorhost.RealFS{},"/usr/bin/docker")!=nil {
  return nil,ErrHostEvidenceUnavailable
 }
 return &HostReadOnlyAdapter{runner:hostDockerRunner{},clock:time.Now},nil
}

func (h *HostReadOnlyAdapter) CaptureSources(_ context.Context, reg executorregistration.Registration)(SourceSnapshot,error){
 return CaptureTrustedSources(reg)
}

// NormalizedCompose uses the registered ordered source files only, with
// explicit project and profile selection and controlled environment. The
// candidate is delivered through Compose's -f - stdin stream instead of
// creating a writable override or touching a source file.
//
// This is a *read-only diagnostic*, NOT a stable-source write authorization:
// production apply must pin the exact bytes across the future Compose up.
func (h *HostReadOnlyAdapter) NormalizedCompose(ctx context.Context,reg executorregistration.Registration,service,candidate string)([]byte,error){
 return h.normalizedComposeSources(ctx,reg,service,candidate,reg.ComposeFiles,reg.EnvFiles)
}

// NormalizedComposeFromHandoff runs the same *read-only* Compose validation
// against root-private copies of trusted source files. The --project-directory
// remains the approved original project to preserve explicitly supported
// relative-path semantics. This does not enable Compose up or pull.
func (h *HostReadOnlyAdapter) NormalizedComposeFromHandoff(
 ctx context.Context,reg executorregistration.Registration,
 handoff *SourceHandoff,service,candidate string)([]byte,error){
 if handoff==nil||handoff.RecheckOriginal(reg)!=nil{return nil,ErrHostEvidenceUnavailable}
 files,envs,err:=handoff.FilesForReadOnlyCompose(reg)
 if err!=nil{return nil,ErrHostEvidenceUnavailable}
 result,err:=h.normalizedComposeSources(ctx,reg,service,candidate,files,envs)
 if err!=nil||handoff.RecheckOriginal(reg)!=nil{return nil,ErrHostEvidenceUnavailable}
 return result,nil
}

func (h *HostReadOnlyAdapter) normalizedComposeSources(
 ctx context.Context,reg executorregistration.Registration,
 service,candidate string,composeFiles,envFiles []string)([]byte,error){
 if h==nil||h.runner==nil||ctx==nil||ctx.Err()!=nil||reg.Validate()!=nil||
  !executorregistration.ValidID(service) {return nil,ErrHostEvidenceUnavailable}
 found:=false
 for _,s:=range reg.Services {if s.Name==service&&s.DeploymentEnabled{found=true}}
 if !found{return nil,ErrHostEvidenceUnavailable}
 var override []byte
 if candidate!=""{
  // Input is strictly an approved repository@immutable digest, not YAML.
  s:=strings.Split(candidate,"@")
  if len(s)!=2||!executorregistration.ValidRepository(s[0])||!validDigest(s[1])||
   !reg.AllowsTarget(service,s[0],executorregistration.Platform{OS:"linux",Architecture:"amd64"}){
   return nil,ErrHostEvidenceUnavailable
  }
  // JSON is a YAML subset. Encode only the already validated service and
  // image strings, preventing YAML syntax or interpolation injection.
  var err error
  override,err=json.Marshal(map[string]any{"services":map[string]any{service:map[string]string{"image":candidate}}})
  if err!=nil{return nil,ErrHostEvidenceUnavailable}
 }
 args:=[]string{"--host",executorhost.LocalSocket,"compose",
  "--project-directory",reg.ProjectDirectory,"-p",reg.ProjectName}
 for _,path:=range composeFiles{args=append(args,"-f",path)}
 if candidate!=""{args=append(args,"-f","-")}
 for _,path:=range envFiles{args=append(args,"--env-file",path)}
 for _,profile:=range reg.Profiles{args=append(args,"--profile",profile)}
 args=append(args,"config","--no-env-resolution","--format","json")
 out,err:=h.runner.run(ctx,hostReadCommand{args:args,stdin:override,limit:MaxConfigBytes})
 if err!=nil||len(out)==0||len(out)>MaxConfigBytes{return nil,ErrHostEvidenceUnavailable}
 normalized,err:=parseNormalized(out)
 if err!=nil{return nil,ErrHostEvidenceUnavailable}
 // Reject unsupported configuration constructs even in *unselected* services.
 // Checking only the target could allow Compose to affect a sibling.
 services,ok:=normalized["services"].(map[string]any)
 if !ok||len(services)==0||len(services)>executorregistration.MaxServices||ValidateNormalizedFeatureSubset(normalized)!=nil{
  return nil,ErrHostEvidenceUnavailable
 }
 for _,s:=range services {
  item,ok:=s.(map[string]any)
  if !ok||unsupportedService(item){return nil,ErrHostEvidenceUnavailable}
 }
 return out,nil
}

// InspectActual discovers exactly one container with trusted Compose identity
// labels, then inspects its immutable image ID. No monitor-provided container
// identifier is passed as a command argument; it is compared later by Qualify.
func (h *HostReadOnlyAdapter) InspectActual(ctx context.Context,reg executorregistration.Registration,service string)(Actual,error){
 var empty Actual
 if h==nil||h.runner==nil||h.clock==nil||ctx==nil||ctx.Err()!=nil||
  reg.Validate()!=nil||!executorregistration.ValidID(service){return empty,ErrHostEvidenceUnavailable}
 enabled:=false
 for _,s:=range reg.Services{if s.Name==service&&s.DeploymentEnabled{enabled=true}}
 if !enabled{return empty,ErrHostEvidenceUnavailable}
 names,err:=h.runner.run(ctx,hostReadCommand{
  args:[]string{"--host",executorhost.LocalSocket,"ps","--all","--no-trunc",
   "--filter","label=com.docker.compose.project="+reg.ProjectName,
   "--filter","label=com.docker.compose.service="+service,
   "--format","{{.ID}}"},limit:256})
 if err!=nil{return empty,ErrHostEvidenceUnavailable}
 id:=strings.TrimSuffix(string(names),"\n")
 if !validContainerID(id){
  return empty,ErrHostEvidenceUnavailable
 }
 data,err:=h.runner.run(ctx,hostReadCommand{
  args:[]string{"--host",executorhost.LocalSocket,"inspect","--format","{{json .}}",id},
  limit:32<<10})
 if err!=nil{return empty,ErrHostEvidenceUnavailable}
 var found struct{
  ID string `json:"Id"`
  Image string `json:"Image"`
  Config struct{Labels map[string]string `json:"Labels"`} `json:"Config"`
  State struct{Running bool `json:"Running"`} `json:"State"`
 }
 decoder:=json.NewDecoder(bytes.NewReader(data))
 if decoder.Decode(&found)!=nil{return empty,ErrHostEvidenceUnavailable}
 var tail any
 if decoder.Decode(&tail)!=io.EOF||found.ID!=id||!found.State.Running||
  !validDigest(found.Image)||found.Config.Labels["com.docker.compose.project"]!=reg.ProjectName||
  found.Config.Labels["com.docker.compose.service"]!=service{return empty,ErrHostEvidenceUnavailable}
 // This fingerprint is a hash of the complete inspected identity/labels,
 // not the Compose config fingerprint. It is refreshed on each qualification.
 stable,_:=json.Marshal(struct {
  ID string
  Image string
  Project string
  Service string
 }{found.ID,found.Image,reg.ProjectName,service})
 hash:=sha256.Sum256(stable)
 return Actual{ContainerID:found.ID,ImageID:found.Image,
  SourceFingerprint:"sha256:"+hex.EncodeToString(hash[:]),CheckedAt:h.clock()},nil
}
func validContainerID(s string)bool{
 if len(s)!=64{return false}
 for _,c:=range s{if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}}
 return true
}

// hostDockerRunner is not exported and accepts only narrowly allowlisted
// read-only calls constructed by the adapter. No shell, inherited Docker
// env/config, inherited stdin or unbounded child output is permitted.
type hostDockerRunner struct{}
func (hostDockerRunner) run(ctx context.Context,c hostReadCommand)([]byte,error){
 if ctx==nil||ctx.Err()!=nil||os.Geteuid()!=0||!approvedHostRead(c.args)||
  c.limit<=0||c.limit>MaxConfigBytes{return nil,ErrHostEvidenceUnavailable}
 cctx,cancel:=context.WithTimeout(ctx,hostReadTimeout)
 defer cancel()
 cmd:=exec.CommandContext(cctx,"/usr/bin/docker",c.args...)
 cmd.Dir="/"
 cmd.Env=executorhost.MinimalChildEnvironment(nil)
 cmd.Stdin=bytes.NewReader(c.stdin)
 var stdout boundedHostOutput
 stdout.limit=c.limit
 cmd.Stdout=&stdout
 cmd.Stderr=io.Discard
 cmd.WaitDelay=time.Second
 if err:=cmd.Run();err!=nil||cctx.Err()!=nil||stdout.overflow{
  return nil,ErrHostEvidenceUnavailable
 }
 return stdout.buf,nil
}
type boundedHostOutput struct{buf []byte;limit int;overflow bool}
func (b *boundedHostOutput)Write(p []byte)(int,error){
 if len(p)>b.limit-len(b.buf){b.overflow=true;return len(p),ErrHostEvidenceUnavailable}
 b.buf=append(b.buf,p...)
 return len(p),nil
}

// Defense in depth for the actual subprocess: only precisely typed readonly
// docker ps, inspect and compose config commands reach exec.CommandContext.
func approvedHostRead(args []string)bool{
 if len(args)<4||args[0]!="--host"||args[1]!=executorhost.LocalSocket{return false}
 switch args[2]{
 case "ps":
  return len(args)==11&&args[3]=="--all"&&args[4]=="--no-trunc"&&
   args[5]=="--filter"&&strings.HasPrefix(args[6],"label=com.docker.compose.project=")&&
   args[7]=="--filter"&&strings.HasPrefix(args[8],"label=com.docker.compose.service=")&&
   args[9]=="--format"&&args[10]=="{{.ID}}"
 case "inspect":
  return len(args)==6&&args[3]=="--format"&&args[4]=="{{json .}}"&&validContainerID(args[5])
 case "compose":
  if len(args)<12||args[3]!="--project-directory"||args[5]!="-p"||
   !executorregistration.ValidID(args[6])||args[len(args)-4]!="config"||args[len(args)-3]!="--no-env-resolution"||
   args[len(args)-2]!="--format"||args[len(args)-1]!="json"{return false}
  // Caller validates registration source paths and fixed file ordering.
  // Disallow dangerous tokens anywhere inside the constructed command.
  for _,arg:=range args[7:len(args)-4] {
   if arg=="up"||arg=="pull"||arg=="down"||arg=="exec"||arg=="run"||
    arg=="build"||arg=="rm"||arg=="create"{return false}
  }
  return true
 }
 return false
}
