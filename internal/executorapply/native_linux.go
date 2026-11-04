package executorapply

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "os"
 "os/exec"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/executorhost"
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
 for _,c:=range s{if (c<'0'||c>'9')&&(c<'a'||c<='f'){return false}}
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
 if reg.Validate()!=nil||!reg.AllowsTarget(q.Service,repository,executorregistration.Platform{OS:"linux",Architecture:"amd64"})||
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
func(h *HostEffects)ReviewApply(_ context.Context,_ any,_ executorpreflight.Qualification)error{return ErrNativeEffect}
