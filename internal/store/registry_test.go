package store
import("context";"path/filepath";"testing")
func TestRegistryHistory(t *testing.T){
 s,e:=Open(filepath.Join(t.TempDir(),"history.db"));if e!=nil{t.Fatal(e)};defer s.Close()
 ctx:=context.Background()
 a,e:=s.RecordRegistryCheck(ctx,"ghcr.io/org/web:latest","linux/amd64","sha256:index1","sha256:a")
 if e!=nil||a.Status!="first_seen"{t.Fatalf("%+v %v",a,e)}
 b,e:=s.RecordRegistryCheck(ctx,a.Image,a.Platform,"sha256:index2","sha256:a")
 if e!=nil||b.Status!="unchanged"{t.Fatalf("%+v %v",b,e)}
 c,e:=s.RecordRegistryCheck(ctx,a.Image,a.Platform,"sha256:index3","sha256:b")
 if e!=nil||c.Status!="changed"||c.PreviousDigest!="sha256:a"{t.Fatalf("%+v %v",c,e)}
 got,e:=s.RegistryChecks(ctx);if e!=nil||len(got)!=1||got[0].Status!="changed"{t.Fatalf("%+v %v",got,e)}
}
