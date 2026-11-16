package executorregistration

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestStandingUpdateRequiresProtectedPersistentOptIn(t *testing.T) {
 r:=fixture()
 r.AutomaticUpdates=&AutomaticUpdatePolicy{Enabled:true,Tag:"latest"}
 if r.Validate()==nil{t.Fatal("accepted unattended authorization without persistent root source")}
 root:=uint32(0)
 r.TrustedSourceUID=&root
 r.PersistentOverride=true
 r.ComposeFiles=[]string{r.ProjectDirectory+"/compose.yaml"}
 if err:=r.Validate();err!=nil{t.Fatal(err)}
 data,e:=json.Marshal(r)
 if e!=nil{t.Fatal(e)}
 parsed,e:=Parse(data)
 if e!=nil||parsed.AutomaticUpdates==nil||parsed.AutomaticUpdates.Tag!="latest" {
  t.Fatalf("standing authority did not round-trip: %#v %v",parsed,e)
 }
 r.AutomaticUpdates.Enabled=false
 if r.Validate()==nil{t.Fatal("accepted disabled but present standing authority")}
 r.AutomaticUpdates.Enabled=true
 for _,tag:=range []string{"","/latest","latest/evil","..","a@sha256:","https://host","tag space",strings.Repeat("a",129)}{
  r.AutomaticUpdates.Tag=tag
  if r.Validate()==nil{t.Fatalf("accepted unsafe tag: %q",tag)}
 }
 r.AutomaticUpdates=nil
 if e:=r.Validate();e!=nil{t.Fatal("absent automatic policy broke manual persistence")}
}
