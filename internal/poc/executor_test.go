package poc
import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
type fake struct{calls [][]string;before,after string}
func(f *fake)Run(_ context.Context,n string,a ...string)([]byte,error){
 f.calls=append(f.calls,append([]string{n},a...))
 if len(a)>0&&a[0]=="ps"{return []byte("c1"),nil}
 if len(a)>0&&a[0]=="inspect"{return []byte(`[{"Name":"/web","Image":"sha256:abc","Config":{"Image":"nginx:old","Labels":{"com.docker.compose.project":"test","com.docker.compose.service":"web"}}}]`),nil}
 for i,s:=range a{if s=="-f"&&i+1<len(a)&&strings.Contains(a[i+1],".image-"){return []byte(f.after),nil}}
 return []byte(f.before),nil
}
func registration(t *testing.T)(Registration,[]string,string){t.Helper();root:=t.TempDir();d:=filepath.Join(root,"app");_ =os.MkdirAll(d,0700);file:=filepath.Join(d,"compose.yaml");_ =os.WriteFile(file,[]byte("services: {}"),0600);return Registration{Name:"test",ProjectDir:d,Files:[]string{file},Services:[]string{"web"}},[]string{root},filepath.Join(root,"out")}
func states()(string,string){
 base:=map[string]any{"services":map[string]any{"web":map[string]any{"image":"nginx:old","networks":[]string{"front"},"labels":map[string]string{"traefik.enable":"true"}}},"networks":map[string]any{"front":map[string]any{"external":true}}}
 next:=map[string]any{"services":map[string]any{"web":map[string]any{"image":"nginx:new","networks":[]string{"front"},"labels":map[string]string{"traefik.enable":"true"}}},"networks":map[string]any{"front":map[string]any{"external":true}}}
 a,_:=json.Marshal(base);b,_:=json.Marshal(next);return string(a),string(b)
}
func TestPrepareAndScopedApply(t *testing.T){p,roots,out:=registration(t);a,b:=states();f:=&fake{before:a,after:b};path,e:=p.Prepare(context.Background(),f,roots,"web","nginx:new",out);if e!=nil{t.Fatal(e)};if _,e=os.Stat(path);e!=nil{t.Fatal(e)}
 if _,e=p.Apply(context.Background(),f,roots,"web","nginx:new",out,false);e==nil{t.Fatal("must require confirmation")}
 if _,e=p.Apply(context.Background(),f,roots,"web","nginx:new",out,true);e!=nil{t.Fatal(e)}
 if len(f.calls)!=6{t.Fatalf("unexpected call count: %d",len(f.calls))}
 if !strings.Contains(strings.Join(f.calls[5]," "),"up -d --no-deps --wait --wait-timeout 120 web"){t.Fatal("wrong deployment target")}
}
func TestRejectOtherService(t *testing.T){p,roots,out:=registration(t);f:=&fake{};if _,e:=p.Prepare(context.Background(),f,roots,"database","nginx:new",out);e==nil||len(f.calls)!=0{t.Fatal("unapproved service allowed")}}
func TestRejectMutation(t *testing.T){p,roots,out:=registration(t);a,b:=states();b=strings.Replace(b,"traefik.enable","unexpected.label",1);f:=&fake{before:a,after:b};if _,e:=p.Prepare(context.Background(),f,roots,"web","nginx:new",out);e==nil{t.Fatal("accepted non-image change")}}
func TestRejectSymlinkEscape(t *testing.T){p,roots,_:=registration(t);outside:=filepath.Join(t.TempDir(),"compose.yaml");_ =os.WriteFile(outside,[]byte("hello"),0600);_ =os.Remove(p.Files[0]);_ =os.Symlink(outside,p.Files[0]);if e:=p.Validate(roots);e==nil{t.Fatal("accepted symlink escape")}}
func TestDiscover(t *testing.T){f:=&fake{};x,e:=Discover(context.Background(),f);if e!=nil||len(x)!=1||x[0].ImageID!="sha256:abc"{t.Fatalf("%+v %v",x,e)}}
