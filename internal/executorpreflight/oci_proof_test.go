package executorpreflight

import (
 "encoding/json"
 "strings"
 "testing"
)

func ociFixture()([]byte,[]byte,string){
 manifest:=map[string]any{
  "schemaVersion":2,"mediaType":ociManifestMediaType,
  "config":map[string]any{"mediaType":"application/vnd.oci.image.config.v1+json",
   "digest":"sha256:"+strings.Repeat("b",64),"size":256},
  "layers":[]any{map[string]any{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip",
   "digest":"sha256:"+strings.Repeat("c",64),"size":1024}},
 }
 mb,_:=json.Marshal(manifest)
 index:=map[string]any{"schemaVersion":2,"mediaType":ociIndexMediaType,
  "manifests":[]any{
   map[string]any{"mediaType":ociManifestMediaType,"digest":digestFor(mb),
    "size":len(mb),"platform":map[string]any{"os":"linux","architecture":"amd64"}},
  },
 }
 ib,_:=json.Marshal(index)
 return ib,mb,digestFor(ib)
}

func TestOCIProofValidPlatformManifest(t *testing.T){
 ib,mb,hash:=ociFixture()
 proof,err:=VerifyOCIPlatformProof(ib,mb,hash)
 if err!=nil||proof.IndexDigest!=hash||proof.PlatformManifestDigest!=digestFor(mb)||
  proof.OS!="linux"||proof.Architecture!="amd64"{
  t.Fatalf("correct OCI platform index rejected: %#v %v",proof,err)
 }
}

func TestOCIProofRejectsTamperAndUnsupportedPlatforms(t *testing.T){
 cases:=[]struct{name string;change func(*[]byte,*[]byte,*string)}{
  {"index_digest_not_matching",func(ib,mb *[]byte,h *string){*h="sha256:"+strings.Repeat("d",64)}},
  {"tampered_manifest",func(ib,mb *[]byte,h *string){*mb=append(*mb,' ')}},
  {"index_trailing_json",func(ib,mb *[]byte,h *string){*ib=append(*ib,[]byte("{}")...);*h=digestFor(*ib)}},
  {"wrong_platform",func(ib,mb *[]byte,h *string){
   var index map[string]any
   _=json.Unmarshal(*ib,&index)
   index["manifests"].([]any)[0].(map[string]any)["platform"].(map[string]any)["architecture"]="arm64"
   *ib,_=json.Marshal(index);*h=digestFor(*ib)
  }},
  {"duplicate_platform",func(ib,mb *[]byte,h *string){
   var index map[string]any
   _=json.Unmarshal(*ib,&index)
   list:=index["manifests"].([]any)
   index["manifests"]=append(list,list[0])
   *ib,_=json.Marshal(index);*h=digestFor(*ib)
  }},
  {"unsupported_index",func(ib,mb *[]byte,h *string){
   var index map[string]any
   _=json.Unmarshal(*ib,&index)
   index["mediaType"]="application/vnd.unknown"
   *ib,_=json.Marshal(index);*h=digestFor(*ib)
  }},
  {"wrong_manifest_size",func(ib,mb *[]byte,h *string){
   var index map[string]any
   _=json.Unmarshal(*ib,&index)
   index["manifests"].([]any)[0].(map[string]any)["size"]=999
   *ib,_=json.Marshal(index);*h=digestFor(*ib)
  }},
  {"oversize_index",func(ib,mb *[]byte,h *string){*ib=[]byte(strings.Repeat("a",MaxOCIIndexBytes+1));*h=digestFor(*ib)}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  ib,mb,h:=ociFixture()
  tc.change(&ib,&mb,&h)
  if _,e:=VerifyOCIPlatformProof(ib,mb,h);e==nil{t.Fatal("untrusted OCI evidence accepted")}
 })}
}
