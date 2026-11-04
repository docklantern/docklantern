package executorpreflight

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestSourceHandoffRootFixture(t *testing.T){
 reg,_,env:=sourceFixture(t)
 h,err:=stageTrustedSourcesAt(reg,"/root")
 if err!=nil{t.Fatal("trusted sources rejected",err)}
 path:=h.directory
 t.Cleanup(func(){if !h.closed{_ = h.Close()}})
 before,envs,err:=h.FilesForReadOnlyCompose(reg)
 if err!=nil||len(before)!=1||len(envs)!=1{
  t.Fatalf("snapshot unusable: %v",err)
 }
 if err=h.RecheckOriginal(reg);err!=nil{t.Fatal("unchanged source rejected",err)}
 if err=os.WriteFile(env,[]byte("SAMPLE=mutated\n"),0o600);err!=nil{t.Fatal(err)}
 if err=h.RecheckOriginal(reg);!errors.Is(err,ErrSourceHandoff){
  t.Fatal("original input replacement not detected",err)
 }
 if err=h.Close();err!=nil{t.Fatal(err)}
 if _,err=os.Lstat(path);!os.IsNotExist(err){t.Fatal("staging tree not removed")}
 if _,_,err=h.FilesForReadOnlyCompose(reg);!errors.Is(err,ErrSourceHandoff){
  t.Fatal("closed source handle accepted",err)
 }
}

func TestSourceHandoffRejectsPrivateCopyTamperRootFixture(t *testing.T){
 reg,_,_:=sourceFixture(t)
 h,err:=stageTrustedSourcesAt(reg,"/root")
 if err!=nil{t.Fatal(err)}
 defer h.Close()
 paths,_,err:=h.FilesForReadOnlyCompose(reg)
 if err!=nil{t.Fatal(err)}
 if err=os.Chmod(paths[0],0o600);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(paths[0],[]byte("tamper\n"),0o600);err!=nil{t.Fatal(err)}
 if err=os.Chmod(paths[0],0o400);err!=nil{t.Fatal(err)}
 if _,_,err=h.FilesForReadOnlyCompose(reg);!errors.Is(err,ErrSourceHandoff){
  t.Fatal("tampered private source accepted",err)
 }
}

func TestSourceHandoffRejectsSymlinkAndUntrustedStagingRootFixture(t *testing.T){
 reg,compose,_:=sourceFixture(t)
 if err=os.Remove(compose);err!=nil{t.Fatal(err)}
 if err=os.Symlink("/etc/passwd",compose);err!=nil{t.Fatal(err)}
 if h,err:=stageTrustedSourcesAt(reg,"/root");err==nil{
  _=h.Close();t.Fatal("untrusted input accepted")
 }
 reg,_,_ = sourceFixture(t)
 base:=filepath.Join(reg.ProjectDirectory,"untrusted-stage")
 if err:=os.Mkdir(base,0o777);err!=nil{t.Fatal(err)}
 if err:=os.Chmod(base,0o777);err!=nil{t.Fatal(err)}
 if h,err:=stageTrustedSourcesAt(reg,base);err==nil{
  _=h.Close();t.Fatal("writable staging root accepted")
 }
}
