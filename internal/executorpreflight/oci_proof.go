package executorpreflight

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "io"
)

// OCIManifestEvidence checks an independently obtained OCI image index and
// selected platform manifest against their cryptographic content digests.
// The caller must separately authenticate the registry source, bind this to
// the registered repository and perform a fresh host Docker state inspection.
// This function has no network, Docker, CLI or execution access.
type OCIManifestEvidence struct {
 IndexDigest string
 PlatformManifestDigest string
 OS string
 Architecture string
}

const MaxOCIIndexBytes = 256 << 10
const MaxOCIManifestBytes = 1 << 20
const ociIndexMediaType = "application/vnd.oci.image.index.v1+json"
const dockerIndexMediaType = "application/vnd.docker.distribution.manifest.list.v2+json"
const ociManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
const dockerManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"

type ociPlatform struct {
 OS string `json:"os"`
 Architecture string `json:"architecture"`
 Variant string `json:"variant,omitempty"`
}
type ociDescriptor struct {
 MediaType string `json:"mediaType"`
 Digest string `json:"digest"`
 Size int64 `json:"size"`
 Platform ociPlatform `json:"platform"`
}
type ociIndex struct {
 SchemaVersion int `json:"schemaVersion"`
 MediaType string `json:"mediaType"`
 Manifests []ociDescriptor `json:"manifests"`
}
type ociManifest struct {
 SchemaVersion int `json:"schemaVersion"`
 MediaType string `json:"mediaType"`
 Config struct {MediaType string `json:"mediaType"`;Digest string `json:"digest"`;Size int64 `json:"size"`} `json:"config"`
 Layers []struct {MediaType string `json:"mediaType"`;Digest string `json:"digest"`;Size int64 `json:"size"`} `json:"layers"`
}

// VerifyOCIPlatformProof accepts only a strict OCI/Docker v2 multi-platform
// index with exactly one linux/amd64 target descriptor. It requires the
// selected manifest bytes and their expected size/digest. This is evidence
// validation, not trusted registry retrieval or deployment authorization.
func VerifyOCIPlatformProof(indexBytes,manifestBytes []byte,expectedIndexDigest string)(OCIManifestEvidence,error){
 var zero OCIManifestEvidence
 if !validDigest(expectedIndexDigest)||len(indexBytes)==0||len(indexBytes)>MaxOCIIndexBytes||
  len(manifestBytes)==0||len(manifestBytes)>MaxOCIManifestBytes||
  digestFor(indexBytes)!=expectedIndexDigest{return zero,ErrNotImageOnly}
 var index ociIndex
 if decodeOne(indexBytes,&index)!=nil||index.SchemaVersion!=2||
  (index.MediaType!=ociIndexMediaType&&index.MediaType!=dockerIndexMediaType)||
  len(index.Manifests)==0||len(index.Manifests)>128{return zero,ErrNotImageOnly}
 var selected *ociDescriptor
 for i:=range index.Manifests {
  d:=&index.Manifests[i]
  if !validDigest(d.Digest)||d.Size<=0||d.Size>MaxOCIManifestBytes ||
   (d.MediaType!=ociManifestMediaType&&d.MediaType!=dockerManifestMediaType){return zero,ErrNotImageOnly}
  if d.Platform.OS=="linux"&&d.Platform.Architecture=="amd64"&&d.Platform.Variant==""{
   if selected!=nil{return zero,ErrNotImageOnly}
   selected=d
  }
 }
 if selected==nil||selected.Digest!=digestFor(manifestBytes)||selected.Size!=int64(len(manifestBytes)){
  return zero,ErrNotImageOnly
 }
 var manifest ociManifest
 if decodeOne(manifestBytes,&manifest)!=nil||manifest.SchemaVersion!=2||
  manifest.MediaType!=selected.MediaType||!validDigest(manifest.Config.Digest)||
  manifest.Config.Size<=0||len(manifest.Layers)==0||len(manifest.Layers)>512{return zero,ErrNotImageOnly}
 for _,l:=range manifest.Layers{if !validDigest(l.Digest)||l.Size<0{return zero,ErrNotImageOnly}}
 return OCIManifestEvidence{IndexDigest:expectedIndexDigest,PlatformManifestDigest:selected.Digest,
  OS:"linux",Architecture:"amd64"},nil
}

func digestFor(data []byte)string{
 h:=sha256.Sum256(data)
 return "sha256:"+hex.EncodeToString(h[:])
}
func decodeOne(data []byte,dest any)error{
 dec:=json.NewDecoder(bytes.NewReader(data))
 if err:=dec.Decode(dest);err!=nil{return ErrNotImageOnly}
 var trailing any
 if err:=dec.Decode(&trailing);err!=io.EOF{return ErrNotImageOnly}
 return nil
}
