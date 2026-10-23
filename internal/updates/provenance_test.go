package updates

import(
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/store"
)
func TestRepoDigestEvidence(t *testing.T){
 a:="sha256:"+strings.Repeat("a",64)
 b:="sha256:"+strings.Repeat("b",64)
 c:="sha256:"+strings.Repeat("c",64)
 policy:=store.MonitorPolicy{Project:"app",Service:"web",Image:"ghcr.io/company/web:latest",Platform:"linux/amd64"}
 projects:=[]store.Registration{{Name:"app",Services:[]string{"web"}}}
 checks:=[]store.RegistryCheck{{Image:policy.Image,Platform:policy.Platform,IndexDigest:b,PlatformDigest:a}}
 cases:=[]struct{name string;container store.Container;state string}{
  {"verified tagged current",store.Container{Image:"ghcr.io/company/web:latest",RepoDigests:[]string{"ghcr.io/company/web@"+a},ImageOS:"linux",ImageArchitecture:"amd64"},"current"},
  {"verified tagged different",store.Container{Image:"ghcr.io/company/web:latest",RepoDigests:[]string{"ghcr.io/company/web@"+c},ImageOS:"linux",ImageArchitecture:"amd64"},"different"},
  {"index digest match",store.Container{Image:"ghcr.io/company/web:latest",RepoDigests:[]string{"ghcr.io/company/web@"+b},ImageOS:"linux",ImageArchitecture:"amd64"},"current"},
  {"mismatched platform",store.Container{Image:"ghcr.io/company/web:latest",RepoDigests:[]string{"ghcr.io/company/web@"+a},ImageOS:"linux",ImageArchitecture:"arm64"},"unknown"},
  {"other repository",store.Container{Image:"ghcr.io/company/web:latest",RepoDigests:[]string{"ghcr.io/company/other@"+a},ImageOS:"linux",ImageArchitecture:"amd64"},"unknown"},
  {"multiple digests",store.Container{Image:"ghcr.io/company/web:latest",RepoDigests:[]string{"ghcr.io/company/web@"+a,"ghcr.io/company/web@"+c},ImageOS:"linux",ImageArchitecture:"amd64"},"unknown"},
  {"no image provenance",store.Container{Image:"ghcr.io/company/web:latest"},"unknown"},
  {"pinned mismatch provenance",store.Container{Image:"ghcr.io/company/web@"+a,RepoDigests:[]string{"ghcr.io/company/web@"+c},ImageOS:"linux",ImageArchitecture:"amd64"},"unknown"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   sample:=tc.container;sample.Project="app";sample.Service="web";sample.State="running"
   got:=Assess(projects,[]store.Container{sample},[]store.MonitorPolicy{policy},checks)
   if len(got)!=1||got[0].State!=tc.state{t.Fatalf("%+v",got)}
  })
 }
}
