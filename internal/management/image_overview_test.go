package management

import (
 "context"
 "net/http"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

func TestBeta4OverviewSharesImageIdentityAndActivityWithImagesWorkspace(t *testing.T){
 h,db,_,_:=setup(t)
 ctx:=context.Background()
 image:="ghcr.io/example/shared:latest"
 discovered:=[]store.Container{}
 for _,service:=range []string{"web","worker"}{
  discovered=append(discovered,store.Container{
   ID:"demo-"+service,Project:"demo",Service:service,Image:image,
   ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
   MonitorLabels:map[string]string{"com.docklantern.monitor":"true"},
  })
 }
 if err:=db.Snapshot(ctx,discovered);err!=nil{t.Fatal(err)}
 policies,err:=db.MonitorPolicies(ctx)
 if err!=nil || len(policies)!=2{t.Fatalf("expected two monitored Compose services: %+v %v",policies,err)}
 ref,platform:=policies[0].Image,policies[0].Platform
 if _,err:=db.RecordRegistryCheck(ctx,ref,platform,"sha256:index","sha256:first");err!=nil{t.Fatal(err)}
 if _,err:=db.RecordRegistryCheck(ctx,ref,platform,"sha256:index","sha256:changed");err!=nil{t.Fatal(err)}
 // The latest registry result becomes unchanged, but the prior image-scoped
 // change event must still appear once in the 24h Overview summary.
 if _,err:=db.RecordRegistryCheck(ctx,ref,platform,"sha256:index","sha256:changed");err!=nil{t.Fatal(err)}
 overview:=request(h,http.MethodGet,"/","",true)
 if overview.Code!=http.StatusOK{t.Fatalf("overview status %d: %s",overview.Code,overview.Body.String())}
 body:=overview.Body.String()
 for _,expected:=range []string{
  `class="stat-label">Watched Images</div><strong>1</strong>`,
  `class="stat-label">Remote Changes · 24h</div><strong>1</strong>`,
  "Images to Review","Recent Image Activity","Remote registry digest changed",
  "Remote digest changed in the last 24 hours",
 }{
  if !strings.Contains(body,expected){t.Errorf("overview missing %q",expected)}
 }
 id:=presentation.ImageID(presentation.ImageIdentity{Reference:ref,Platform:platform})
 if count:=strings.Count(body,`>View image</a>`);count!=1{
  t.Errorf("shared image appears %d times in review",count)
 }
 if !strings.Contains(body,`href="/images/`+id+`"`){t.Error("review has no real image-details link")}
 inventory:=request(h,http.MethodGet,"/images","",true)
 if inventory.Code!=200||strings.Count(inventory.Body.String(),`>View image</a>`)!=1{
  t.Fatalf("Images workspace not grouped consistently: %d",inventory.Code)
 }
 // The overview must not reintroduce unrelated full tables from beta.3.
 for _,obsolete:=range []string{"Compose projects with registered sources","Service schedules","Image status assessments","Latest registry checks"}{
  if strings.Contains(body,obsolete){t.Errorf("obsolete section %q returned",obsolete)}
 }
 if got:=request(h,http.MethodGet,"/","",false).Code;got!=http.StatusUnauthorized{
  t.Errorf("Overview no longer requires authentication: %d",got)
 }
}
