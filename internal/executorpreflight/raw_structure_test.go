package executorpreflight

import (
 "errors"
 "strings"
 "testing"
)

func TestRawStructureRejectsDuplicateAndAmbiguousMappings(t *testing.T){
 tests:=map[string]string{
  "duplicate_service":strings.Replace(allowedRawCompose,"  sibling:","  web:\n    image: docker.io/example/other:stable\n  sibling:",1),
  "duplicate_image":strings.Replace(allowedRawCompose,"    pull_policy: always","    image: docker.io/example/other:stable\n    pull_policy: always",1),
  "duplicate_root":allowedRawCompose+"\nservices:\n  rogue:\n    image: busybox\n",
  "duplicate_nested_network":strings.Replace(allowedRawCompose,"    external: true","    external: true\n    external: false",1),
  "odd_indentation":strings.Replace(allowedRawCompose,"    image: docker.io/example/hello-go:stable","   image: docker.io/example/hello-go:stable",1),
  "skipped_indentation":strings.Replace(allowedRawCompose,"    image: docker.io/example/hello-go:stable","      image: docker.io/example/hello-go:stable",1),
  "unmatched_dedent":strings.Replace(allowedRawCompose,"    pull_policy: always"," pull_policy: always",1),
  "mixed_mapping_sequence":strings.Replace(allowedRawCompose,"      - web","      - web\n      rogue: value",1),
  "mapping_colon_without_space":strings.Replace(allowedRawCompose,"    pull_policy: always","    pull_policy:always",1),
  "quoted_key":strings.Replace(allowedRawCompose,"  web:","  \"web\":",1),
  "quoted_nested_key":strings.Replace(allowedRawCompose,"    image:","    \"image\":",1),
  "trailing_space":strings.Replace(allowedRawCompose,"  web:","  web: ",1),
  "colon_mapping_in_scalar":strings.Replace(allowedRawCompose,"    pull_policy: always","    pull_policy: always: unsafe",1),
  "trailing_comment":strings.Replace(allowedRawCompose,"    pull_policy: always","    pull_policy: always # ignored",1),
  "root_only": "services:\n",
  "services_scalar":"services: anything\n",
  "multi_root_nested":strings.Replace(allowedRawCompose,"networks:\n  web:","networks:\n    web:",1),
 }
 for name,data:=range tests{t.Run(name,func(t *testing.T){
  if err:=ValidateRawComposeSource([]byte(data));!errors.Is(err,ErrUntrustedComposeSyntax){
   t.Fatalf("structurally ambiguous Compose YAML accepted: %s",name)
  }
 })}
}

func TestRawStructurePreservesSimpleComposeWithMultipleSources(t *testing.T){
 valid:=[]string{
  allowedRawCompose,
  "services:\n  web:\n    image: docker.io/example/hello-go:stable\n    command: \"echo hello\"\n",
  "services:\n  web:\n    image: docker.io/example/hello-go:stable\n    expose:\n      - \"8080\"\n",
  "services:\n  web:\n    image: docker.io/example/hello-go:stable\n    labels:\n      traefik.enable: \"true\"\n",
 }
 for _,src:=range valid{
  if err:=ValidateRawComposeSource([]byte(src));err!=nil{
   t.Fatalf("valid constrained Compose structure denied: %v",err)
  }
 }
}
