package executorpreflight

import (
 "encoding/json"
 "strings"
 "testing"
)

func cloneConfig(v map[string]any)map[string]any{
 data,_:=json.Marshal(v)
 var result map[string]any
 _=json.Unmarshal(data,&result)
 return result
}

func TestNormalizedFeatureSubsetPermitsPrebuiltExternalNetwork(t *testing.T){
 before,_,_:=configs()
 if err:=ValidateNormalizedFeatureSubset(before);err!=nil{
  t.Fatalf("narrow approved prebuilt config denied: %v",err)
 }
 // Compose also adds an explicit effective external network name.
 network:=before["networks"].(map[string]any)["web"].(map[string]any)
 network["name"]="web"
 if err:=ValidateNormalizedFeatureSubset(before);err!=nil{
  t.Fatalf("resolved external network denied: %v",err)
 }
}

func TestNormalizedFeatureSubsetFailsClosedOnNestedOrMutableInputs(t *testing.T){
 fixtures:=[]struct{
  name string
  mutate func(map[string]any)
 }{
  {"top_level_include",func(a map[string]any){a["include"]=[]any{"extra.yml"}}},
  {"top_level_secrets",func(a map[string]any){a["secrets"]=map[string]any{"api":map[string]any{"file":"/tmp/key"}}}},
  {"top_level_configs",func(a map[string]any){a["configs"]=map[string]any{"api":map[string]any{"file":"./config"}}}},
  {"unknown_top_level",func(a map[string]any){a["x-extension"]=map[string]any{"unsafe":true}}},
  {"implicit_network",func(a map[string]any){a["networks"]=map[string]any{"default":map[string]any{"name":"hello-default","driver":"bridge"}}}},
  {"new_project_volume",func(a map[string]any){a["volumes"]=map[string]any{"data":map[string]any{"name":"hello_data"}}}},
  {"service_env_file",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["env_file"]=[]any{"./untrusted.env"}}},
  {"sibling_env_file",func(a map[string]any){a["services"].(map[string]any)["worker"].(map[string]any)["env_file"]=[]any{"./untrusted.env"}}},
  {"service_secret",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["secrets"]=[]any{"api"}}},
  {"service_config",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["configs"]=[]any{"api"}}},
  {"service_volumes",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["volumes"]=[]any{"data:/app/data"}}},
  {"sibling_volumes",func(a map[string]any){a["services"].(map[string]any)["worker"].(map[string]any)["volumes"]=[]any{"data:/app/data"}}},
  {"unknown_service_field",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["mystery"]=true}},
  {"unknown_sibling_field",func(a map[string]any){a["services"].(map[string]any)["worker"].(map[string]any)["mystery"]=true}},
  {"unknown_platform",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["platform"]="linux/arm64"}},
  {"pull_credentials",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["credential_spec"]="file:secret"}},
  {"post_start",func(a map[string]any){a["services"].(map[string]any)["web"].(map[string]any)["post_start"]=[]any{map[string]any{"command":"echo hi"}}}},
 }
 for _,tc:=range fixtures{t.Run(tc.name,func(t *testing.T){
  before,after,digest:=configs()
  tc.mutate(before)
  tc.mutate(after)
  if err:=ValidateNormalizedFeatureSubset(before);err==nil{
   t.Fatal("unsupported Compose input classified as safe")
  }
  if err:=CheckImageOnly(raw(before),raw(after),fixtureRegistration(),"web","docker.io/example/hello-go",digest);err==nil{
   t.Fatal("unchanged dangerous Compose feature bypassed image-only checks")
  }
 })}
}

func TestNormalizedFeatureSubsetRequiresExactImagePlatform(t *testing.T){
 before,_,_:=configs()
 before["services"].(map[string]any)["web"].(map[string]any)["platform"]="linux/amd64"
 if err:=ValidateNormalizedFeatureSubset(before);err!=nil{
  t.Fatalf("explicit compatible platform rejected: %v",err)
 }
 before["services"].(map[string]any)["web"].(map[string]any)["environment"]=map[string]any{"FOO":strings.Repeat("x",60)}
 if err:=ValidateNormalizedFeatureSubset(before);err!=nil{
  t.Fatalf("ordinary environment data rejected: %v",err)
 }
}
