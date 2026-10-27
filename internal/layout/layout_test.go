package layout

import (
 "html/template"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/buildinfo"
)

func TestSharedShellExposesBakedBuildVersion(t *testing.T){
 page:=template.Must(WithShell(template.Must(template.New("test").Parse(`{{template "shell-start" .}}Welcome{{template "shell-end" .}}`))))
 var html strings.Builder
 if err:=page.Execute(&html,struct{
  ActivePage string
  MainClass string
 }{ActivePage:"overview"});err!=nil{t.Fatal(err)}
 expected:="Running app: "+buildinfo.Version
 if !strings.Contains(html.String(),expected){
  t.Fatalf("shell missing actual linked build version %q",expected)
 }
 if !strings.Contains(html.String(),`href="/images"`){
  t.Fatal("build label removed shared navigation")
 }
}
