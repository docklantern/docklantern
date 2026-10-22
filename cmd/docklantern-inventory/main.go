// docklantern-inventory is a monitoring-only CLI/HTTP prototype.
// Do not expose its unauthenticated HTTP listener to the public internet.
package main

import (
 "context"
 "runtime"
 "encoding/json"
 "flag"
 "fmt"
 "log"
 "net/http"
 "os"
 "strings"
 "time"
 "github.com/docklantern/docklantern/internal/api"
 "github.com/docklantern/docklantern/internal/discovery"
 "github.com/docklantern/docklantern/internal/poc"
 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)
func main(){
 mode:=flag.String("mode","serve","serve, discover, scan, register, projects, check-image")
 dbPath:=flag.String("db","/data/docklantern.db","SQLite file")
 addr:=flag.String("listen","127.0.0.1:8787","local listener only")
 registration:=flag.String("registration","","JSON registration file")
 image:=flag.String("image","","tagged OCI image for manual registry check")
 platform:=flag.String("platform",runtime.GOOS+"/"+runtime.GOARCH,"target platform")
 roots:=flag.String("roots","","comma-separated allowlisted Compose source roots")
 flag.Parse()
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 if *mode=="discover"{x,e:=discovery.Scan(ctx,discovery.Shell{});must(e);must(json.NewEncoder(os.Stdout).Encode(x));return}
 s,e:=store.Open(*dbPath);must(e);defer s.Close()
 switch *mode{
 case "register":
  b,e:=os.ReadFile(*registration);must(e);var p store.Registration;must(json.Unmarshal(b,&p))
  validate:=poc.Registration{Name:p.Name,ProjectDir:p.ProjectDir,Files:p.Files,EnvFiles:p.EnvFiles,Services:p.Services,Profiles:p.Profiles}
  if *roots==""{log.Fatal("registration requires explicit -roots")}
  must(validate.Validate(strings.Split(*roots,",")))
  p.Enabled=false // Monitoring registration never authorizes updates.
  must(s.Register(ctx,p));fmt.Println("registered for monitoring:",p.Name)
 case "scan":
  x,e:=discovery.Scan(ctx,discovery.Shell{});must(e)
  items:=make([]store.Container,0,len(x))
  for _,c:=range x{items=append(items,store.Container{ID:c.ID,Name:c.Name,Project:c.Project,Service:c.Service,Image:c.Image,ImageID:c.ImageID,State:c.State,Health:c.Health})}
  must(s.Snapshot(ctx,items));fmt.Printf("recorded %d containers\n",len(items))
 case "check-image":
  if *image==""{log.Fatal("-image is required for check-image")}
  user:=os.Getenv("DOCKLANTERN_REGISTRY_USERNAME")
  token:=os.Getenv("DOCKLANTERN_REGISTRY_TOKEN")
  if token!=""&&user==""{log.Fatal("registry username required with token")}
  resolution,e:=(registry.Client{Credentials:registry.Credentials{Username:user,Password:token}}).Check(ctx,*image,*platform);must(e)
  check,e:=s.RecordRegistryCheck(ctx,resolution.Image,resolution.Platform,resolution.IndexDigest,resolution.PlatformDigest);must(e)
  must(json.NewEncoder(os.Stdout).Encode(check))
 case "projects":
  x,e:=s.Projects(ctx);must(e);must(json.NewEncoder(os.Stdout).Encode(x))
 case "serve":
  // The API is unauthenticated and intentionally read-only. Bind loopback only.
  if !strings.HasPrefix(*addr,"127.0.0.1:")&&!strings.HasPrefix(*addr,"localhost:"){log.Fatal("POC HTTP API must bind loopback")}
  log.Printf("inventory API on %s",*addr)
  server:=&http.Server{Addr:*addr,Handler:api.Handler(s),ReadHeaderTimeout:5*time.Second}
  log.Fatal(server.ListenAndServe())
 default:log.Fatal("unknown mode")
 }
}
func must(err error){if err!=nil{log.Fatal(err)}}
