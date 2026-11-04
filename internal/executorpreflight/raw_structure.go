package executorpreflight

import (
 "regexp"
 "strings"
)

// This is a deliberately bounded *subset* grammar, not an arbitrary YAML
// implementation. It is checked before the installed Compose CLI may see
// any of the registered source bytes. Ordinary mapping/sequence syntax must
// be unambiguous, with unique ASCII keys, exact two-space indent changes,
// no inline mappings and no unregistered implicit file mechanisms.
//
// It is intentionally narrower than Compose's full YAML grammar and is used
// together with the lexical denylist and effective-config comparison.
var rawMapping=regexp.MustCompile(`^([A-Za-z0-9_.-]+):(?: (.*))?$`)

type rawContainer struct {
 indent int
 kind byte // 'm' for mapping, 's' for sequence
 keys map[string]bool
}

func validateRawStructure(source string) error {
 stack:=make([]rawContainer,0,12)
 canNest:=false
 seenServices:=false
 serviceChildren:=false
 currentRoot:=""
 for _,raw:=range strings.Split(source,"\n"){
  if strings.TrimSpace(raw)==""{continue}
  spaces:=len(raw)-len(strings.TrimLeft(raw," "))
  line:=raw[spaces:]
  if strings.HasPrefix(line,"#"){continue}
  if spaces%2!=0||spaces>32||strings.HasSuffix(line," ") {
   return ErrUntrustedComposeSyntax
  }
  if len(stack)==0 {
   if spaces!=0{return ErrUntrustedComposeSyntax}
   stack=append(stack,rawContainer{indent:0,keys:map[string]bool{}})
  }else if spaces>stack[len(stack)-1].indent {
   if !canNest||spaces!=stack[len(stack)-1].indent+2 {
    return ErrUntrustedComposeSyntax
   }
   stack=append(stack,rawContainer{indent:spaces,keys:map[string]bool{}})
  }else if spaces<stack[len(stack)-1].indent{
   for len(stack)>0&&spaces<stack[len(stack)-1].indent{
    stack=stack[:len(stack)-1]
   }
   if len(stack)==0||stack[len(stack)-1].indent!=spaces{
    return ErrUntrustedComposeSyntax
   }
  }
  frame:=&stack[len(stack)-1]
  kind:=byte('m')
  if strings.HasPrefix(line,"- "){kind='s'}
  if frame.kind!=0&&frame.kind!=kind{return ErrUntrustedComposeSyntax}
  frame.kind=kind
  canNest=false
  if kind=='s'{
   item:=strings.TrimPrefix(line,"- ")
   if item=="" {canNest=true;continue}
   if !validRawScalar(item){return ErrUntrustedComposeSyntax}
   continue
  }
  m:=rawMapping.FindStringSubmatch(line)
  if m==nil{return ErrUntrustedComposeSyntax}
  key:=m[1]
  if frame.keys[key]{return ErrUntrustedComposeSyntax}
  frame.keys[key]=true
  if spaces==0{currentRoot=key;if key=="services"{seenServices=true}}
  if spaces==2&&currentRoot=="services"{
   // A root service name is not a recognized credential or command.
   // Effective-config checks independently validate all service fields.
   serviceChildren=true
  }
  value:=m[2]
  if value=="" {canNest=true;continue}
  if !validRawScalar(value){return ErrUntrustedComposeSyntax}
 }
 if !seenServices||!serviceChildren{return ErrUntrustedComposeSyntax}
 return nil
}

func validRawScalar(s string) bool {
 if s==""||strings.TrimSpace(s)!=s{return false}
 // Reject comments and YAML mapping syntax embedded in an unquoted scalar.
 // Quoted strings are permitted without escapes, and are parsed again by
 // Docker Compose in the read-only config validation.
 if strings.HasPrefix(s,"\"")&&strings.HasSuffix(s,"\"")&&len(s)>=2{
  interior:=s[1:len(s)-1]
  return !strings.ContainsAny(interior,"\\\n\r") && !strings.Contains(interior,"\"")
 }
 if strings.HasPrefix(s,"'")&&strings.HasSuffix(s,"'")&&len(s)>=2{
  interior:=s[1:len(s)-1]
  return !strings.ContainsAny(interior,"\n\r") && !strings.Contains(interior,"'")
 }
 if strings.Contains(s,": ")||strings.Contains(s," #")||
  strings.HasPrefix(s,"#")||strings.HasPrefix(s,"'")||
  strings.HasPrefix(s,"\"")||strings.HasPrefix(s,"- "){
  return false
 }
 return true
}
