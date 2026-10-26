// Package discovery inspects Docker Compose containers without altering them.
package discovery

import (
 "context"
 "encoding/json"
 "fmt"
 "os/exec"
 "sort"
 "strings"
)

type Container struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Project string `json:"project"`
 Service string `json:"service"`
 Image string `json:"image"`
 ImageID string `json:"image_id"`
 State string `json:"state"`
 Health string `json:"health,omitempty"`
 RepoDigests []string `json:"repo_digests,omitempty"`
 ImageOS string `json:"image_os,omitempty"`
 ImageArchitecture string `json:"image_architecture,omitempty"`
 ImageVariant string `json:"image_variant,omitempty"`
 // Only DockLantern's monitoring labels are passed to the persistence layer.
 MonitorLabels map[string]string `json:"monitor_labels,omitempty"`
}
type Runner interface { Run(context.Context,string,...string)([]byte,error) }
type Shell struct{}
func (Shell) Run(ctx context.Context, cmd string,args ...string)([]byte,error) {
 b,err:=exec.CommandContext(ctx,cmd,args...).Output()
 if err!=nil{return nil,fmt.Errorf("docker command failed: %w",err)}
 return b,nil
}
func Scan(ctx context.Context,r Runner)([]Container,error){
 ids,err:=r.Run(ctx,"docker","ps","-aq")
 if err!=nil{return nil,err}
 names:=strings.Fields(string(ids));if len(names)==0{return []Container{},nil}
 found:=make([]Container,0,len(names))
 // Inspect in bounded batches to avoid excessively long argument lists.
 for start:=0;start<len(names);start+=100 {
  end:=start+100;if end>len(names){end=len(names)}
  args:=append([]string{"inspect","--type=container"},names[start:end]...)
  b,e:=r.Run(ctx,"docker",args...);if e!=nil{return nil,e}
  var docs []struct {
   ID string `json:"Id"`
   Name string `json:"Name"`
   Image string `json:"Image"`
   Config struct{Image string `json:"Image"`;Labels map[string]string `json:"Labels"`} `json:"Config"`
   State struct{Status string `json:"Status"`;Health *struct{Status string `json:"Status"`} `json:"Health"`} `json:"State"`
  }
  if e=json.Unmarshal(b,&docs);e!=nil{return nil,fmt.Errorf("parse Docker inspect: %w",e)}
  for _,x:=range docs{
   project:=x.Config.Labels["com.docker.compose.project"]
   service:=x.Config.Labels["com.docker.compose.service"]
   if project==""||service==""{continue}
   item:=Container{ID:x.ID,Name:strings.TrimPrefix(x.Name,"/"),Project:project,Service:service,Image:x.Config.Image,ImageID:x.Image,State:x.State.Status}
   if x.State.Health!=nil{item.Health=x.State.Health.Status}
   item.MonitorLabels=monitorLabels(x.Config.Labels)
   if validID(x.Image){
    b,err:=r.Run(ctx,"docker","image","inspect",x.Image)
    if err==nil{
     var image []ImageMetadata
     if err=json.Unmarshal(b,&image);err==nil&&len(image)==1{
      if m,e:=verifiedImageMetadata(x.Image,image[0]);e==nil{
       item.RepoDigests=m.RepoDigests;item.ImageOS=m.OS;item.ImageArchitecture=m.Architecture;item.ImageVariant=m.Variant
      }
     }
    }
   }
   found=append(found,item)
  }
 }
 sort.Slice(found,func(i,j int)bool{if found[i].Project==found[j].Project{if found[i].Service==found[j].Service{return found[i].Name<found[j].Name};return found[i].Service<found[j].Service};return found[i].Project<found[j].Project})
 return found,nil
}
