// Package discovery contains read-only Docker Compose inventory clients.
package discovery

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "sort"
 "strings"
 "time"
)

const maxContainers = 500
const maxResponse = 4 << 20

// Proxy uses only the Docker Engine GET endpoints through a constrained
// HTTP socket proxy. It never sends a request capable of mutating Docker.
type Proxy struct {
 base string
 client *http.Client
}
func NewProxy(raw string)(*Proxy,error){
 u,err:=url.Parse(raw)
 if err!=nil||u.Scheme!="http"||u.Hostname()==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.Path!=""{
  return nil,errors.New("discovery proxy must be an internal http://host:port endpoint with no credentials or path")
 }
 if u.Hostname()=="0.0.0.0" {return nil,errors.New("invalid proxy destination")}
 return &Proxy{base:"http://"+u.Host,client:&http.Client{
  Timeout:12*time.Second,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
 }},nil
}
func (p *Proxy) get(ctx context.Context,path string,dst any)error{
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,p.base+path,nil)
 if err!=nil{return err}
 resp,err:=p.client.Do(req)
 if err!=nil{return errors.New("Docker proxy unavailable")}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return fmt.Errorf("Docker proxy HTTP %d",resp.StatusCode)}
 dec:=json.NewDecoder(io.LimitReader(resp.Body,maxResponse+1))
 if err:=dec.Decode(dst);err!=nil{return errors.New("invalid Docker proxy response")}
 return nil
}
func (p *Proxy) Scan(ctx context.Context)([]Container,error){
 var summaries []struct {
  ID string `json:"Id"`
  Labels map[string]string `json:"Labels"`
 }
 // Filter server-side to Compose-labeled containers, but never trust label
 // contents in follow-up URL construction.
 if err:=p.get(ctx,"/containers/json?all=1&filters=%7B%22label%22%3A%5B%22com.docker.compose.project%22%2C%22com.docker.compose.service%22%5D%7D",&summaries);err!=nil{return nil,err}
 if len(summaries)>maxContainers{return nil,errors.New("Docker inventory exceeds safe container limit")}
 found:=make([]Container,0,len(summaries))
 imageCache:=make(map[string]ImageMetadata)
 for _,item:=range summaries{
  if item.Labels["com.docker.compose.project"]==""||item.Labels["com.docker.compose.service"]==""{continue}
  // Engine IDs are strictly hex. Never pass labels or names into URL paths.
  if len(item.ID)<12||len(item.ID)>64{return nil,errors.New("invalid Docker container ID")}
  for _,r:=range item.ID{if !strings.ContainsRune("0123456789abcdef",r){return nil,errors.New("invalid Docker container ID")}}
  var detail struct{
   ID string `json:"Id"`
   Name string `json:"Name"`
   Image string `json:"Image"`
   Config struct{Image string `json:"Image"`;Labels map[string]string `json:"Labels"`} `json:"Config"`
   State struct{Status string `json:"Status"`;Health *struct{Status string `json:"Status"`} `json:"Health"`} `json:"State"`
  }
  if err:=p.get(ctx,"/containers/"+item.ID+"/json",&detail);err!=nil{return nil,err}
  if detail.ID!=item.ID{return nil,errors.New("Docker proxy container identity changed during scan")}
  project:=detail.Config.Labels["com.docker.compose.project"]
  service:=detail.Config.Labels["com.docker.compose.service"]
  if project==""||service==""{continue}
  out:=Container{ID:detail.ID,Name:strings.TrimPrefix(detail.Name,"/"),Project:project,Service:service,Image:detail.Config.Image,ImageID:detail.Image,State:detail.State.Status}
  if detail.State.Health!=nil{out.Health=detail.State.Health.Status}
  // Optional read-only image inspection, cached by local immutable config ID.
  // A denied IMAGES endpoint means provenance remains unknown, not a scan failure.
  if validID(detail.Image){
   image,ok:=imageCache[detail.Image]
   if !ok{
    if err:=p.get(ctx,"/images/"+detail.Image+"/json",&image);err==nil{
     if verified,err:=verifiedImageMetadata(detail.Image,image);err==nil{
      image=verified
      imageCache[detail.Image]=image
     }
    }
   }
   if image.ID==detail.Image{
    out.RepoDigests=image.RepoDigests
    out.ImageOS=image.OS;out.ImageArchitecture=image.Architecture;out.ImageVariant=image.Variant
   }
  }
  found=append(found,out)
 }
 sort.Slice(found,func(i,j int)bool{
  if found[i].Project!=found[j].Project{return found[i].Project<found[j].Project}
  if found[i].Service!=found[j].Service{return found[i].Service<found[j].Service}
  return found[i].Name<found[j].Name
 })
 return found,nil
}
