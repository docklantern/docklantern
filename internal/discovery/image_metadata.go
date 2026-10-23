package discovery

import (
 "errors"
 "strings"
)

// ImageMetadata holds only the immutable identity and relevant manifest metadata.
// Do not persist or expose the full Docker image inspection payload.
type ImageMetadata struct {
 ID string `json:"Id"`
 RepoDigests []string `json:"RepoDigests"`
 OS string `json:"Os"`
 Architecture string `json:"Architecture"`
 Variant string `json:"Variant"`
}

// validID rejects names and tags in Docker API URL paths; these can contain
// separators or otherwise change the requested image.
func validID(id string) bool {
 if len(id)!=71||!strings.HasPrefix(id,"sha256:"){return false}
 for _,r:=range id[7:] {if !strings.ContainsRune("0123456789abcdef",r){return false}}
 return true
}

func verifiedImageMetadata(want string, image ImageMetadata)(ImageMetadata,error){
 if !validID(want)||image.ID!=want {return ImageMetadata{},errors.New("Docker image configuration ID mismatch")}
 if image.OS==""||image.Architecture=="" {return ImageMetadata{},errors.New("Docker image platform unavailable")}
 // RepoDigests may contain manifests from other registries or tags; their
 // association with the exact local config ID is all we can establish here.
 if len(image.RepoDigests)>100{return ImageMetadata{},errors.New("excessive image digest count")}
 return image,nil
}
