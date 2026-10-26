package settings

import (
 "net/http/httptest"
 "reflect"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
)

func TestServiceFiltersAndProjectGrouping(t *testing.T){
 services:=[]presentation.ServiceSummary{
  {ServiceIdentity:presentation.ServiceIdentity{Project:"zeta",Service:"web"},Image:"nginx:latest",Monitoring:"monitoring",Assessment:"current",HasPolicy:true,Enrollment:"Docker label"},
  {ServiceIdentity:presentation.ServiceIdentity{Project:"alpha",Service:"cache"},Image:"redis:7",Monitoring:"not_enrolled",Assessment:"unknown"},
  {ServiceIdentity:presentation.ServiceIdentity{Project:"alpha",Service:"worker"},Image:"ghcr.io/org/worker:v1",Monitoring:"check_failed",Assessment:"different",HasPolicy:true,Enrollment:"GUI",LastError:"timeout"},
  {ServiceIdentity:presentation.ServiceIdentity{Project:"alpha",Service:"blocked"},Image:"alpine:latest",Monitoring:"disabled_by_label",Assessment:"unknown",HasPolicy:false},
  {ServiceIdentity:presentation.ServiceIdentity{Project:"zeta",Service:"jobs"},Image:"alpine:3",Monitoring:"paused",Assessment:"unknown",HasPolicy:true,Enrollment:"Docker label"},
 }
 cases:=[]struct{name string;filter serviceFilters;want []string}{
  {"all",serviceFilters{},[]string{"alpha/blocked","alpha/cache","alpha/worker","zeta/jobs","zeta/web"}},
  {"search image",serviceFilters{Query:"REDIS"},[]string{"alpha/cache"}},
  {"search project",serviceFilters{Query:"ZETA"},[]string{"zeta/jobs","zeta/web"}},
  {"active",serviceFilters{Monitoring:"active"},[]string{"alpha/worker","zeta/web"}},
  {"paused",serviceFilters{Monitoring:"paused"},[]string{"zeta/jobs"}},
  {"unmonitored",serviceFilters{Monitoring:"not_enrolled"},[]string{"alpha/cache"}},
  {"label disabled",serviceFilters{Monitoring:"disabled_by_label"},[]string{"alpha/blocked"}},
  {"attention",serviceFilters{Monitoring:"needs_attention"},[]string{"alpha/worker"}},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   chosen:=filterServices(services,tc.filter)
   found:=[]string{}
   for _,s:=range chosen{found=append(found,s.Project+"/"+s.Service)}
   if !reflect.DeepEqual(found,tc.want){t.Errorf("got %v, want %v",found,tc.want)}
  })
 }
 grouped:=groupServices(filterServices(services,serviceFilters{}))
 if len(grouped)!=2 || grouped[0].Project!="alpha" || len(grouped[0].Services)!=3 || grouped[1].Project!="zeta" || len(grouped[1].Services)!=2{
  t.Fatalf("unexpected Compose project groups: %+v",grouped)
 }
 if !serviceEnrolled(services[0]) || serviceEnrolled(services[1]) || serviceEnrolled(services[3]){
  t.Fatal("wrong enrollment count semantics")
 }
}

func TestServiceFilterRejectsUnexpectedInputs(t *testing.T){
 tests:=[]struct{query string;valid bool}{
  {"",true},
  {"?q=web&monitoring=active",true},
  {"?monitoring=needs_attention",true},
  {"?monitoring=update_now",false},
  {"?q="+strings.Repeat("x",129),false},
  {"?q=line%0Abreak",false},
 }
 for _,tc:=range tests{
  r:=httptest.NewRequest("GET","/services"+tc.query,nil)
  _,err:=parseServiceFilters(r)
  if (err==nil)!=tc.valid{t.Errorf("query %q validity mismatch: %v",tc.query,err)}
 }
}
