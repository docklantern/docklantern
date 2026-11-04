package executorpreflight

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "syscall"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrSourceChanged = errors.New("trusted Compose source unavailable or changed")

const maxSourceBytes = 1 << 20
const maxSourceTotalBytes = 4 << 20

// SourceSnapshot is opaque evidence only. It contains no Compose/env contents,
// secrets, paths or deployment authorization. Files are processed in their
// registered order, so swapping -f precedence changes the fingerprint.
type SourceSnapshot struct {
 SHA256 string
 FileCount int
}

// CaptureTrustedSources hashes the exact bytes of registered Compose and env
// files using descriptor-relative O_NOFOLLOW opens, a trusted-owner path walk,
// bounded reads, and pre/post descriptor identity checks. It does not invoke
// Docker or Compose. Neither this nor RecheckTrustedSources pins a file across
// the eventual Compose command: an independently designed stable-source
// handoff is still mandatory before Docker writes.
func CaptureTrustedSources(reg executorregistration.Registration) (SourceSnapshot,error) {
 var zero SourceSnapshot
 if os.Geteuid()!=0 || reg.Validate()!=nil || reg.TrustedSourceUID==nil {
  return zero,ErrSourceChanged
 }
 files:=append(append([]string{},reg.ComposeFiles...),reg.EnvFiles...)
 if len(files)==0{return zero,ErrSourceChanged}
 if err:=validateSourceDirectory(reg.ProjectDirectory,*reg.TrustedSourceUID);err!=nil {
  return zero,ErrSourceChanged
 }
 h:=sha256.New()
 total:=0
 for i,path:=range files{
  data,err:=readTrustedSource(path,*reg.TrustedSourceUID)
  if err!=nil{return zero,ErrSourceChanged}
  total+=len(data)
  if total>maxSourceTotalBytes{return zero,ErrSourceChanged}
  if i<len(reg.ComposeFiles)&&ValidateRawComposeSource(data)!=nil{
   return zero,ErrSourceChanged
  }
  // Length-prefixed content and index distinguish file order/boundaries.
  // The registration itself separately pins exact trusted path identities.
  var prefix [16]byte
  putUint64(prefix[:8],uint64(i))
  putUint64(prefix[8:],uint64(len(data)))
  _,_ = h.Write(prefix[:])
  _,_ = h.Write(data)
 }
 return SourceSnapshot{SHA256:"sha256:"+hex.EncodeToString(h.Sum(nil)),FileCount:len(files)},nil
}

// RecheckTrustedSources is a mandatory fail-closed diagnostic to use after
// review. It NEVER authorizes execution on its own.
func RecheckTrustedSources(reg executorregistration.Registration,original SourceSnapshot) error {
 if original.FileCount==0||!validDigest(original.SHA256){return ErrSourceChanged}
 current,err:=CaptureTrustedSources(reg)
 if err!=nil||current!=original{return ErrSourceChanged}
 return nil
}

func putUint64(p []byte,n uint64){
 for i:=7;i>=0;i--{p[i]=byte(n);n>>=8}
}

func validateSourceDirectory(path string,uid uint32)error{
 fd,err:=walkTrustedSourcePath(path,uid,true)
 if err!=nil{return err}
 defer syscall.Close(fd)
 return nil
}

func readTrustedSource(path string,uid uint32)([]byte,error){
 fd,err:=walkTrustedSourcePath(path,uid,false)
 if err!=nil{return nil,err}
 f:=os.NewFile(uintptr(fd),"trusted-compose-source")
 defer f.Close()
 before,ok:=trustedSourceStat(fd,uid,false)
 if !ok||before.Size<0||before.Size>maxSourceBytes{return nil,ErrSourceChanged}
 data,err:=io.ReadAll(io.LimitReader(f,maxSourceBytes+1))
 if err!=nil||len(data)>maxSourceBytes||int64(len(data))!=before.Size{return nil,ErrSourceChanged}
 after,ok:=trustedSourceStat(fd,uid,false)
 if !ok||before.Dev!=after.Dev||before.Ino!=after.Ino||
  before.Mode!=after.Mode||before.Uid!=after.Uid||before.Nlink!=after.Nlink||
  before.Size!=after.Size||before.Mtim!=after.Mtim||before.Ctim!=after.Ctim {
  return nil,ErrSourceChanged
 }
 return data,nil
}

// All path components are opened relative to a verified directory FD. The
// final file cannot be a symlink, socket, FIFO, directory or hardlink.
func walkTrustedSourcePath(path string,uid uint32,directory bool)(int,error){
 if !filepath.IsAbs(path)||path=="/"||filepath.Clean(path)!=path||len(path)>512||
  strings.IndexByte(path,0)>=0{return -1,ErrSourceChanged}
 parts:=strings.Split(strings.TrimPrefix(path,"/"),"/")
 if len(parts)<2||len(parts)>32{return -1,ErrSourceChanged}
 fd,err:=syscall.Open("/",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
 if err!=nil{return -1,ErrSourceChanged}
 good:=false
 defer func(){if !good{_ = syscall.Close(fd)}}()
 if _,ok:=trustedSourceStat(fd,uid,true);!ok{return -1,ErrSourceChanged}
 for i,part:=range parts {
  if part==""||part=="."||part==".."{return -1,ErrSourceChanged}
  last:=i==len(parts)-1
  flags:=syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK
  if !last||directory {flags|=syscall.O_DIRECTORY}
  next,e:=syscall.Openat(fd,part,flags,0)
  if e!=nil{return -1,ErrSourceChanged}
  _=syscall.Close(fd)
  fd=next
  _,ok:=trustedSourceStat(fd,uid,!last||directory)
  if !ok{return -1,ErrSourceChanged}
 }
 good=true
 return fd,nil
}

func trustedSourceStat(fd int,uid uint32,directory bool)(syscall.Stat_t,bool){
 var st syscall.Stat_t
 if syscall.Fstat(fd,&st)!=nil||(st.Uid!=0&&st.Uid!=uid)||st.Mode&0o022!=0 {
  return st,false
 }
 if directory {
  return st,st.Mode&syscall.S_IFMT==syscall.S_IFDIR
 }
 return st,st.Mode&syscall.S_IFMT==syscall.S_IFREG&&st.Nlink==1
}
