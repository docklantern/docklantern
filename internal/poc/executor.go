// Package poc contains experimental Docker Compose execution routines.
// Not production safe: no durable journal, immutable digest pinning or rollback.
package poc

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
)
type Runner interface { Run(context.Context,string,...string)([]byte,error) }
type Shell struct{}
func (Shell) Run(ctx context.Context,name string,args ...string)([]byte,error){
 out,err:=exec.CommandContext(ctx,name,args...).CombinedOutput()
 if err!=nil{return nil,fmt.Errorf("%s: %w (output redacted)",name,err)}
 return out,nil
}
type Registration struct {
 Name string `json:"name"`
 ProjectDir string `json:"project_dir"`
 Files []string `json:"files"`
 EnvFiles []string `json:"env_files"`
 Services []string `json:"services"`
 Profiles []string `json:"profiles"`
}
var identifier=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
func inside(root,p string)bool{r,e:=filepath.Rel(root,p);return e==nil&&r!=".."&&!strings.HasPrefix(r,".."+string(os.PathSeparator))}
func (p Registration) Validate(roots []string)error{
 if !identifier.MatchString(p.Name)||len(p.Files)==0||len(p.Services)==0||!filepath.IsAbs(p.ProjectDir){return errors.New("invalid registration")}
 for _,s:=range append(append([]string{},p.Services...),p.Profiles...){if !identifier.MatchString(s){return errors.New("invalid service/profile")}}
 paths:=append(append([]string{p.ProjectDir},p.Files...),p.EnvFiles...)
 for i,path:=range paths {
  if !filepath.IsAbs(path){return errors.New("path must be absolute")}
  real,e:=filepath.EvalSymlinks(path);if e!=nil{return e}
  allowed:=false
  for _,root:=range roots{rr,e:=filepath.EvalSymlinks(root);if e==nil&&inside(rr,real){allowed=true;break}}
  if !allowed{return fmt.Errorf("path outside allowed roots: %s",path)}
  st,e:=os.Stat(path);if e!=nil{return e};if i==0&&!st.IsDir(){return errors.New("project dir is not directory")};if i>0&&!st.Mode().IsRegular(){return errors.New("Compose input is not a file")}
 }
 if !inside(p.ProjectDir,p.Files[0]){return errors.New("primary Compose file must be within project dir")}
 return nil
}
func (p Registration) permitted(s string)bool{for _,x:=range p.Services{if x==s{return true}};return false}
func (p Registration) args(override string)[]string{
 a:=[]string{"compose","--project-directory",p.ProjectDir,"-p",p.Name}
 for _,e:=range p.EnvFiles{a=append(a,"--env-file",e)}
 for _,f:=range p.Files{a=append(a,"-f",f)}
 if override!=""{a=append(a,"-f",override)}
 for _,profile:=range p.Profiles{a=append(a,"--profile",profile)}
 return a
}
type config map[string]json.RawMessage
func load(ctx context.Context,r Runner,p Registration,override string)(config,error){
 a:=append(p.args(override),"config","--format","json")
 b,e:=r.Run(ctx,"docker",a...);if e!=nil{return nil,e}
 var c config;if e=json.Unmarshal(b,&c);e!=nil{return nil,e}
 if len(c)==0{return nil,errors.New("empty Compose configuration")};return c,nil
}
func onlyImageChanged(old,new config,service,image string)error{
 if len(old)!=len(new){return errors.New("changed top-level config fields")}
 for key,value:=range old{
  next,ok:=new[key];if !ok{return fmt.Errorf("removed config section %s",key)}
  if key!="services" {
   var a,b any;_ =json.Unmarshal(value,&a);_ =json.Unmarshal(next,&b);x,_:=json.Marshal(a);y,_:=json.Marshal(b)
   if string(x)!=string(y){return fmt.Errorf("modified top-level %s",key)}
   continue
  }
  var before,after map[string]map[string]json.RawMessage
  if json.Unmarshal(value,&before)!=nil||json.Unmarshal(next,&after)!=nil{return errors.New("invalid service data")}
  if len(before)!=len(after){return errors.New("service set changed")}
  for name,fields:=range before{
   newFields,ok:=after[name];if !ok||len(fields)!=len(newFields){return errors.New("service fields changed")}
   for field,original:=range fields{
    got,ok:=newFields[field];if !ok{return errors.New("service field removed")}
    if name==service&&field=="image"{var s string;_ =json.Unmarshal(got,&s);if s!=image{return errors.New("wrong image override")};continue}
    var a,b any;_ =json.Unmarshal(original,&a);_ =json.Unmarshal(got,&b);x,_:=json.Marshal(a);y,_:=json.Marshal(b)
    if string(x)!=string(y){return fmt.Errorf("unexpected mutation: %s.%s",name,field)}
   }
  }
 }
 return nil
}
var imagePattern=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]*$`)
func imageValid(s string)bool{return len(s)<=512&&imagePattern.MatchString(s)&&!strings.Contains(s,"..")&&strings.ContainsAny(s,":@")}
func (p Registration) Prepare(ctx context.Context,r Runner,roots []string,service,image,outDir string)(string,error){
 if e:=p.Validate(roots);e!=nil{return "",e}
 if !p.permitted(service)||!identifier.MatchString(service)||!imageValid(image){return "",errors.New("unapproved service or invalid image")}
 if !filepath.IsAbs(outDir){return "",errors.New("output directory must be absolute")}
 orig,e:=load(ctx,r,p,"");if e!=nil{return "",e}
 var services map[string]any
 if e=json.Unmarshal(orig["services"],&services);e!=nil{return "",e}
 if _,ok:=services[service];!ok{return "",errors.New("service not found in effective config")}
 payload,_:=json.Marshal(map[string]any{"services":map[string]any{service:map[string]string{"image":image}}})
 if e=os.MkdirAll(outDir,0700);e!=nil{return "",e}
 f,e:=os.CreateTemp(outDir,".image-*.yaml");if e!=nil{return "",e}
 path:=f.Name();defer func(){if path!=""{os.Remove(path)}}()
 _ =f.Chmod(0600)
 if _,e=f.Write(payload);e!=nil{f.Close();return "",e};if e=f.Close();e!=nil{return "",e}
 changed,e:=load(ctx,r,p,path);if e!=nil{return "",e}
 if e=onlyImageChanged(orig,changed,service,image);e!=nil{return "",e}
 result:=path+".prepared";if e=os.Rename(path,result);e!=nil{return "",e};path="";return result,nil
}
func (p Registration) Apply(ctx context.Context,r Runner,roots []string,service,image,outDir string,confirm bool)(string,error){
 if !confirm{return "",errors.New("explicit disposable-fixture confirmation required")}
 prepared,e:=p.Prepare(ctx,r,roots,service,image,outDir);if e!=nil{return "",e}
 if _,e=r.Run(ctx,"docker",append(p.args(prepared),"pull",service)...);e!=nil{return prepared,e}
 _,e=r.Run(ctx,"docker",append(p.args(prepared),"up","-d","--no-deps","--wait","--wait-timeout","120",service)...)
 return prepared,e
}
type Found struct {Project string `json:"project"`;Service string `json:"service"`;Container string `json:"container"`;Image string `json:"image"`;ImageID string `json:"image_id"`}
func Discover(ctx context.Context,r Runner)([]Found,error){
 ids,e:=r.Run(ctx,"docker","ps","-aq");if e!=nil{return nil,e};if len(strings.Fields(string(ids)))==0{return []Found{},nil}
 result,e:=r.Run(ctx,"docker",append([]string{"inspect","--type=container"},strings.Fields(string(ids))...)...);if e!=nil{return nil,e}
 var raw []struct{Name string `json:"Name"`;Image string `json:"Image"`;Config struct{Image string `json:"Image"`;Labels map[string]string `json:"Labels"`}`}
 if e=json.Unmarshal(result,&raw);e!=nil{return nil,e}
 found:=[]Found{}
 for _,c:=range raw{pr,s:=c.Config.Labels["com.docker.compose.project"],c.Config.Labels["com.docker.compose.service"];if identifier.MatchString(pr)&&identifier.MatchString(s){found=append(found,Found{pr,s,strings.TrimPrefix(c.Name,"/"),c.Config.Image,c.Image})}}
 return found,nil
}
