package settings

import (
 "net/http/httptest"
 "reflect"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

func TestImageFiltersSearchStatusAndOrder(t *testing.T){
 now:=time.Date(2026,10,26,12,0,0,0,time.UTC)
 checked:=now.Add(-10*time.Minute).Format(time.RFC3339)
 older:=now.Add(-time.Hour).Format(time.RFC3339)
 images:=[]presentation.ImageSummary{
  {Reference:"registry-1.docker.io/library/nginx:latest",Platform:"linux/amd64",
   RegistryStatus:"unchanged",DeploymentStatus:"different",DeploymentCounts:presentation.DeploymentCounts{Different:1},Services:[]presentation.ServiceIdentity{{Project:"site",Service:"web"}},
   LastChangeAt:now.Add(-3*time.Hour).Format(time.RFC3339),
   LatestObservation:&store.RegistryCheck{CheckedAt:older}},
  {Reference:"ghcr.io/org/api:latest",Platform:"linux/amd64",
   RegistryStatus:"changed",DeploymentStatus:"mixed",DeploymentCounts:presentation.DeploymentCounts{Different:1,Current:1},
   Services:[]presentation.ServiceIdentity{{Project:"api",Service:"web"},{Project:"api",Service:"worker"}},
   LastChangeAt:now.Add(-time.Minute).Format(time.RFC3339),
   LatestObservation:&store.RegistryCheck{CheckedAt:checked}},
  {Reference:"registry-1.docker.io/library/redis:7",Platform:"linux/arm64",
   RegistryStatus:"check_failed",DeploymentStatus:"unknown",DeploymentCounts:presentation.DeploymentCounts{Unknown:1},
   Services:[]presentation.ServiceIdentity{{Project:"infra",Service:"cache"}}},
 }
 testCases:=[]struct{name string;filter imageFilters;want []string}{
  {"attention",imageFilters{Sort:"attention"},[]string{"redis:7","api:latest","nginx:latest"}},
  {"name",imageFilters{Sort:"name"},[]string{"api:latest","nginx:latest","redis:7"}},
  {"search service",imageFilters{Sort:"name",Query:"INFRA"},[]string{"redis:7"}},
  {"search registry",imageFilters{Sort:"name",Query:"ghcr.io"},[]string{"api:latest"}},
  {"registry changed",imageFilters{Sort:"attention",Registry:"changed"},[]string{"api:latest"}},
  {"recent recorded change",imageFilters{Sort:"attention",Registry:"recorded_change_24h"},[]string{"api:latest","nginx:latest"}},
  {"different deployment",imageFilters{Sort:"name",Deployment:"different"},[]string{"nginx:latest"}},
  {"most services",imageFilters{Sort:"services"},[]string{"api:latest","nginx:latest","redis:7"}},
  {"latest check",imageFilters{Sort:"last_check"},[]string{"api:latest","nginx:latest","redis:7"}},
  {"no matches",imageFilters{Query:"doesnotexist",Sort:"attention"},nil},
 }
 for _,tc:=range testCases{
  t.Run(tc.name,func(t *testing.T){
   got:=filterImages(images,tc.filter,now)
   found:=[]string{}
   for _,v:=range got{
    split:=strings.Split(v.Reference,"/")
    found=append(found,split[len(split)-1])
   }
   if len(found)==0{found=nil}
   if !reflect.DeepEqual(found,tc.want){t.Errorf("got %v want %v",found,tc.want)}
  })
 }
 if len(images)!=3{t.Fatal("input slice mutated")}
}

func TestImageQueryValidation(t *testing.T){
 cases:=[]struct{raw string;valid bool}{
  {"",true},
  {"?q=nginx&registry=changed&deployment=mixed&sort=services",true},
  {"?registry=recorded_change_24h",true},
  {"?sort=unknown",false},
  {"?registry=write_action",false},
  {"?deployment=upgrade_available",false},
  {"?q="+strings.Repeat("a",129),false},
  {"?q=a%0Ab",false},
 }
 for _,tc:=range cases{
  req:=httptest.NewRequest("GET","/images"+tc.raw,nil)
  _,err:=parseImageFilters(req)
  if (err==nil)!=tc.valid{t.Errorf("%q: valid=%v err=%v",tc.raw,tc.valid,err)}
 }
}

func TestRecentChangeRequiresFreshRetainedTimestamp(t *testing.T){
 now:=time.Date(2026,10,26,12,0,0,0,time.UTC)
 samples:=[]struct{raw string;expected bool}{
  {now.Add(-time.Hour).Format(time.RFC3339),true},
  {now.Add(-48*time.Hour).Format(time.RFC3339),false},
  {now.Add(time.Hour).Format(time.RFC3339),false},
  {"",false},
  {"bad-date",false},
 }
 for _,tc:=range samples{
  a:=presentation.ImageSummary{LastChangeAt:tc.raw}
  if got:=hasRecordedChangeInWindow(a,now,24*time.Hour);got!=tc.expected{t.Errorf("%q: %v",tc.raw,got)}
 }
}
