package executorpreflight

import (
 "crypto/sha256"
 "errors"
 "os"
 "path/filepath"
 "strconv"
 "sync"

 "github.com/docklantern/docklantern/internal/executorhost"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

const HostSourceSnapshotsDirectory = "/var/lib/docklantern/executor/source-snapshots"

var ErrSourceHandoff = errors.New("trusted Compose source handoff unavailable")

// SourceHandoff is a short-lived, root-private *copy of exact source bytes*.
// It does not copy an arbitrary Compose project directory or authorize any
// Docker mutation. Its paths are not accepted from proposals or the monitor.
// The handle must remain open until the entire future Compose subprocess ends.
type SourceHandoff struct {
 mu sync.Mutex
 directory string
 composeFiles []string
 envFiles []string
 contentHashes [][32]byte
 original SourceSnapshot
 registrationID string
 closed bool
}

// StageTrustedSources only uses the executor-owned private snapshots root;
// no parent directories are created implicitly on a production host.
func StageTrustedSources(reg executorregistration.Registration)(*SourceHandoff,error){
 return stageTrustedSourcesAt(reg,HostSourceSnapshotsDirectory)
}

// stageTrustedSourcesAt is for isolated disposable root CI fixtures only.
func stageTrustedSourcesAt(reg executorregistration.Registration,base string)(*SourceHandoff,error){
 if os.Geteuid()!=0||reg.Validate()!=nil||reg.TrustedSourceUID==nil||
  executorhost.ValidatePrivateDirectory(executorhost.RealFS{},base)!=nil{
  return nil,ErrSourceHandoff
 }
 first,err:=CaptureTrustedSources(reg)
 if err!=nil{return nil,ErrSourceHandoff}
 root,err:=os.MkdirTemp(base,"source-")
 if err!=nil{return nil,ErrSourceHandoff}
 keep:=false
 defer func(){if !keep{_ = os.RemoveAll(root)}}()
 files:=append(append([]string{},reg.ComposeFiles...),reg.EnvFiles...)
 h:=&SourceHandoff{
  directory:root,registrationID:reg.RegistrationID,original:first,
  composeFiles:make([]string,0,len(reg.ComposeFiles)),
  envFiles:make([]string,0,len(reg.EnvFiles)),
 }
 var total int
 for i,source:=range files{
  bytes,err:=readTrustedSource(source,*reg.TrustedSourceUID)
  if err!=nil{return nil,ErrSourceHandoff}
  total+=len(bytes)
  if total>maxSourceTotalBytes{return nil,ErrSourceHandoff}
  if i<len(reg.ComposeFiles)&&ValidateRawComposeSource(bytes)!=nil{
   return nil,ErrSourceHandoff
  }
  // Deterministic root-private names preserve ordered -f and --env-file
  // precedence. No untrusted original basenames are interpolated into a path.
  ext:=".yaml"
  if i>=len(reg.ComposeFiles){ext=".env"}
  name:="input-"+strconv.Itoa(i)+ext
  dest:=filepath.Join(root,name)
  file,err:=os.OpenFile(dest,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0o600)
  if err!=nil{return nil,ErrSourceHandoff}
  _,writeErr:=file.Write(bytes)
  syncErr:=file.Sync()
  modeErr:=file.Chmod(0o400)
  closeErr:=file.Close()
  if writeErr!=nil||syncErr!=nil||modeErr!=nil||closeErr!=nil{
   return nil,ErrSourceHandoff
  }
  h.contentHashes=append(h.contentHashes,sha256.Sum256(bytes))
  if i<len(reg.ComposeFiles){h.composeFiles=append(h.composeFiles,dest)
  }else{h.envFiles=append(h.envFiles,dest)}
 }
 // Do not grant group/world traversal, and make the directory immutable to
 // non-root operators for the lifetime of this short-lived snapshot.
 if err=os.Chmod(root,0o500);err!=nil{return nil,ErrSourceHandoff}
 if err=syncDirectory(root);err!=nil{return nil,ErrSourceHandoff}
 second,err:=CaptureTrustedSources(reg)
 if err!=nil||first!=second{return nil,ErrSourceHandoff}
 if err=h.verifyLocked(reg);err!=nil{return nil,ErrSourceHandoff}
 keep=true
 return h,nil
}

func syncDirectory(path string)error{
 d,err:=os.Open(path)
 if err!=nil{return err}
 defer d.Close()
 return d.Sync()
}

// FilesForReadOnlyCompose revalidates every staged file before returning its
// private paths to the trusted host adapter. The paths cannot outlive Close.
// This check is evidence only and not a stable descriptor across exec.
func (h *SourceHandoff) FilesForReadOnlyCompose(reg executorregistration.Registration)([]string,[]string,error){
 if h==nil{return nil,nil,ErrSourceHandoff}
 h.mu.Lock()
 defer h.mu.Unlock()
 if err:=h.verifyLocked(reg);err!=nil{return nil,nil,ErrSourceHandoff}
 return append([]string(nil),h.composeFiles...),append([]string(nil),h.envFiles...),nil
}

func (h *SourceHandoff) verifyLocked(reg executorregistration.Registration)error{
 if h.closed||os.Geteuid()!=0||reg.Validate()!=nil||
  h.registrationID!=reg.RegistrationID||
  len(h.composeFiles)!=len(reg.ComposeFiles)||
  len(h.envFiles)!=len(reg.EnvFiles)||
  len(h.contentHashes)!=len(reg.ComposeFiles)+len(reg.EnvFiles){
  return ErrSourceHandoff
 }
 st,err:=os.Lstat(h.directory)
 if err!=nil||!st.IsDir()||st.Mode().Perm()!=0o500{
  return ErrSourceHandoff
 }
 all:=append(append([]string{},h.composeFiles...),h.envFiles...)
 for i,path:=range all{
  b,err:=readTrustedSource(path,0)
  if err!=nil||sha256.Sum256(b)!=h.contentHashes[i]{
   return ErrSourceHandoff
  }
  if i<len(h.composeFiles)&&ValidateRawComposeSource(b)!=nil{
   return ErrSourceHandoff
  }
 }
 return nil
}

// RecheckOriginal fails closed on post-staging source modification, even
// though Compose must consume the staged immutable byte copy.
func (h *SourceHandoff) RecheckOriginal(reg executorregistration.Registration)error{
 if h==nil{return ErrSourceHandoff}
 h.mu.Lock()
 defer h.mu.Unlock()
 if err:=h.verifyLocked(reg);err!=nil{return err}
 if RecheckTrustedSources(reg,h.original)!=nil{return ErrSourceHandoff}
 return nil
}

// Close clears the private staging tree. It never changes Compose source
// files or user-authored overrides and never launches Docker.
func (h *SourceHandoff) Close()error{
 if h==nil{return ErrSourceHandoff}
 h.mu.Lock()
 defer h.mu.Unlock()
 if h.closed{return ErrSourceHandoff}
 h.closed=true
 if err:=os.Chmod(h.directory,0o700);err!=nil{return ErrSourceHandoff}
 if err:=os.RemoveAll(h.directory);err!=nil{return ErrSourceHandoff}
 return nil
}
