package executorpreflight

import (
 "errors"
 "strings"
 "testing"
)

const allowedRawCompose = `services:
  web:
    image: docker.io/example/hello-go:stable
    pull_policy: always
    labels:
      traefik.enable: "true"
    environment:
      SAMPLE: "${SAMPLE_VALUE}"
    networks:
      - web
  sibling:
    image: docker.io/example/sibling:stable
networks:
  web:
    external: true
`

func TestRawComposeSourceAcceptsNarrowProfile(t *testing.T){
 for _,s:=range []string{
  allowedRawCompose,
  "# documented Compose file\n"+allowedRawCompose,
  strings.Replace(allowedRawCompose,"    labels:","    # labels\n    labels:",1),
 }{
  if e:=ValidateRawComposeSource([]byte(s));e!=nil{t.Fatalf("valid source rejected: %v",e)}
 }
}

func TestRawComposeSourceRejectsImplicitOrAmbiguousInputs(t *testing.T){
 cases:=map[string]string{
  "include":"include:\n  - other.yaml\n",
  "quoted_include":"\"include\":\n  - other.yaml\n",
  "inline_include":"  web: {include: other.yaml}\n",
  "extends":"    extends:\n      file: ../../other.yaml\n",
  "quoted_extends":"    'extends':\n      file: ../../other.yaml\n",
  "env_file":"    env_file:\n      - ./secrets.env\n",
  "quoted_env_file":"    \"env_file\": secrets.env\n",
  "service_secrets":"    secrets:\n      - certificate\n",
  "service_configs":"    configs:\n      - metadata\n",
  "build":"    build: .\n",
  "yaml_alias":"    image: *remote_image\n",
  "yaml_anchor":"    image: &remote_image busybox\n",
  "yaml_tag":"    image: !include ./other.yml\n",
  "merge_key":"    <<: *defaults\n",
  "flow_mapping":"    labels: {owner: external}\n",
  "flow_sequence":"    ports: [8080]\n",
  "block_scalar":"    command: |\n      include: other.yml\n",
  "explicit_complex_key":"    ? include\n    : other.yaml\n",
  "escaped_key":"    \"\\u0069nclude\": other.yaml\n",
  "multi_document":"---\n",
  "missing_services":"name: something\n",
  "invalid_utf8":string([]byte{0xff}),
  "nul":"services:\n"+string([]byte{0}),
  "crlf":"services:\r\n",
 }
 for name,s:=range cases{t.Run(name,func(t *testing.T){
  input:=allowedRawCompose+"\n"+s
  if name=="missing_services"||name=="invalid_utf8"||name=="nul"||name=="crlf"{input=s}
  if err:=ValidateRawComposeSource([]byte(input));!errors.Is(err,ErrUntrustedComposeSyntax){
   t.Fatalf("unsafe raw Compose source accepted (%s)",name)
  }
 })}
}
