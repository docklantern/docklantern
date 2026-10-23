// Package auth protects the local monitoring UI with HTTP Basic authentication.
// Credentials must only be used over HTTPS (reverse proxy) or local loopback.
package auth

import (
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "errors"
 "net/http"
 "net/url"
 "strings"
)

type Guard struct {
 username string
 password [32]byte
 csrfKey [32]byte
 publicOrigin string
}
func New(username,password string)(*Guard,error){
 if strings.TrimSpace(username)==""||len(password)<20{return nil,errors.New("authentication requires a username and password of at least 20 characters")}
 g:=&Guard{username:username,password:sha256.Sum256([]byte(password))}
 if _,err:=rand.Read(g.csrfKey[:]);err!=nil{return nil,err}
 return g,nil
}
// SetPublicOrigin configures a single explicit external HTTPS origin.
// Forwarded headers are never used to choose the expected origin.
func(g *Guard) SetPublicOrigin(raw string)error{
 u,err:=url.Parse(raw)
 if err!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.Path!=""||u.RawQuery!=""||u.Fragment!="" {
  return errors.New("public origin must be an HTTPS origin with no path or query")
 }
 if u.Hostname()=="" {return errors.New("invalid public origin hostname")}
 g.publicOrigin=u.Scheme+"://"+u.Host
 return nil
}
func(g *Guard) PublicOrigin()string{return g.publicOrigin}
func(g *Guard) Token()string{
 m:=hmac.New(sha256.New,g.csrfKey[:]);_,_=m.Write([]byte("docklantern-monitoring-settings-v1"))
 return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func(g *Guard) ValidToken(s string)bool{
 expected:=g.Token()
 return len(s)==len(expected)&&subtle.ConstantTimeCompare([]byte(s),[]byte(expected))==1
}
func(g *Guard) CheckCSRF(r *http.Request)bool{
 // Protect even authenticated browsers against cross-site form requests.
 if r.Header.Get("Sec-Fetch-Site")=="cross-site" {return false}
 if r.Header.Get("Origin")!=""{
  // A configured external origin is not implicitly trusted; reverse proxy must
  // provide the trusted public origin as an explicit setting in a future version.
  // For now only same-host Origin passes.
  if !g.sameOrigin(r){return false}
 }
 if err:=r.ParseForm();err!=nil{return false}
 return g.ValidToken(r.PostFormValue("csrf_token"))
}
func(g *Guard) sameOrigin(r *http.Request)bool{
 origin:=r.Header.Get("Origin")
 if g.publicOrigin!="" {return origin==g.publicOrigin}
 return origin=="http://"+r.Host||origin=="https://"+r.Host
}
func(g *Guard) Wrap(next http.Handler)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if g.publicOrigin!=""{
   // Exact Host and proxy protocol expectations; do not trust arbitrary
   // X-Forwarded-Host values to grant access to another origin.
   originURL,_:=url.Parse(g.publicOrigin)
   if !strings.EqualFold(r.Host,originURL.Host)||r.Header.Get("X-Forwarded-Proto")!="https"{
    http.Error(w,"Invalid gateway origin",http.StatusForbidden)
    return
   }
  }
  // Readiness contains no private data and is accessible without credentials.
  if r.URL.Path=="/healthz" && r.Method==http.MethodGet {next.ServeHTTP(w,r);return}
  user,pass,ok:=r.BasicAuth()
  sum:=sha256.Sum256([]byte(pass))
  valid:=ok&&subtle.ConstantTimeCompare([]byte(user),[]byte(g.username))==1&&subtle.ConstantTimeCompare(sum[:],g.password[:])==1
  if !valid {
   w.Header().Set("WWW-Authenticate",`Basic realm="DockLantern", charset="UTF-8"`)
   w.Header().Set("Cache-Control","no-store")
   http.Error(w,"Authentication required",http.StatusUnauthorized)
   return
  }
  w.Header().Set("Cache-Control","no-store")
  next.ServeHTTP(w,r)
 })
}
