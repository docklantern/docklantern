package executorpreflight

import (
 "bytes"
 "errors"
 "regexp"
 "strings"
 "unicode/utf8"
)

// ErrUntrustedComposeSyntax is deliberately generic: source contents,
// interpolated values and paths must never enter host CLI diagnostics.
var ErrUntrustedComposeSyntax = errors.New("untrusted Compose syntax or implicit source")

// The beta.6 first-deployment profile accepts a deliberately small textual
// YAML subset before invoking Docker Compose. This lexical guard supplements
// (never replaces) the normalized whole-project feature allowlist.
//
// In particular, Compose may erase include/extends/env_file provenance while
// normalizing YAML. Reject these keywords in both plain and quoted mapping
// keys, including inline occurrences. Reject YAML tags/anchors/aliases,
// explicit complex keys, flow mappings and multiline scalars that could hide
// those directives from a line-oriented guard. Do not attempt to be a general
// YAML parser: unsupported syntax fails closed.
var implicitComposeKeys=regexp.MustCompile(`(?i)(^|[[:space:],])['"]?(include|extends|env_file|configs|secrets|build|develop|post_start|pre_stop|credential_spec|volumes_from)['"]?[[:space:]]*:`)
var yamlAlias=regexp.MustCompile(`(^|[[:space:][:punct:]])[&*][A-Za-z_][A-Za-z0-9_-]*`)
var yamlBlock=regexp.MustCompile(`:[[:space:]]*[|>][+-]?[[:space:]]*(#.*)?$`)

// ValidateRawComposeSource is invoked on descriptor-verified original
// *and* staged Compose file bytes before any read-only qualification.
// Empty, binary, noncanonical and unsupported YAML features are denied.
func ValidateRawComposeSource(data []byte) error {
 if len(data)==0||len(data)>maxSourceBytes||
  !utf8.Valid(data)||bytes.IndexByte(data,0)>=0||
  bytes.Contains(data,[]byte("\r"))||
  bytes.Contains(data,[]byte("\t")) {
  return ErrUntrustedComposeSyntax
 }
 foundServices:=false
 lines:=strings.Split(string(data),"\n")
 for _,line:=range lines {
  trimmed:=strings.TrimSpace(line)
  if trimmed==""||strings.HasPrefix(trimmed,"#"){continue}
  // Do not allow directives/markers that modify YAML document semantics.
  if strings.HasPrefix(trimmed,"%")||trimmed=="---"||trimmed=="..."||
   strings.HasPrefix(trimmed,"? ")||strings.HasPrefix(trimmed,"<<:")||
   strings.Contains(trimmed,"\\")||strings.Contains(trimmed,"!")||
   strings.Contains(trimmed,"{")&&!strings.Contains(trimmed,"${")||
   strings.Contains(trimmed,"}")&&!strings.Contains(trimmed,"${")||
   strings.Contains(trimmed,"[")||strings.Contains(trimmed,"]")||
   yamlBlock.MatchString(line)||yamlAlias.MatchString(line)||
   implicitComposeKeys.MatchString(line) {
   return ErrUntrustedComposeSyntax
  }
  // Disallow arbitrary indentation: no tabs (checked above), and no
  // continuation lines without an ordinary YAML mapping/sequence marker.
  if len(line)-len(strings.TrimLeft(line," "))>32{
   return ErrUntrustedComposeSyntax
  }
  if line=="services:"{foundServices=true}
 }
 if !foundServices||validateRawStructure(string(data))!=nil{return ErrUntrustedComposeSyntax}
 return nil
}
