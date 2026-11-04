package executorpreflight

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorhost"
)

func TestHostComposeVersionPolicy(t *testing.T){
 cases:=[]struct{name,output string;allow bool}{
  {"official_tested_v5","5.5.1\n",true},
  {"v2_observes_env_file_differently","2.38.2\n",false},
  {"future_untested","5.5.2\n",false},
  {"additional_output","5.5.1\nUNTRUSTED\n",false},
  {"malformed","\n",false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  runner:=&hostFixtureRunner{response:func(c hostReadCommand)([]byte,error){
   if len(c.args)!=5||c.args[0]!="--host"||c.args[1]!=executorhost.LocalSocket||
    c.args[2]!="compose"||c.args[3]!="version"||c.args[4]!="--short" {
    t.Fatalf("unexpected host version command: %#v",c.args)
   }
   return []byte(tc.output),nil
  }}
  adapter:=HostReadOnlyAdapter{runner:runner,clock:time.Now}
  err:=adapter.RequireSupportedCompose(context.Background())
  if (err==nil)!=tc.allow{t.Fatalf("unexpected runtime qualification: %q %v",tc.output,err)}
  if len(runner.calls)!=1{t.Fatalf("unexpected command calls: %d",len(runner.calls))}
 })}
}
func TestHostComposeVersionRejectsUnavailableAndMutatingCommands(t *testing.T){
 runner:=&hostFixtureRunner{response:func(hostReadCommand)([]byte,error){
  return nil,errors.New("untrusted child process error")
 }}
 adapter:=HostReadOnlyAdapter{runner:runner,clock:time.Now}
 if adapter.RequireSupportedCompose(context.Background())==nil{
  t.Fatal("failed version command accepted")
 }
 for _,args:=range [][]string{
  {"--host",executorhost.LocalSocket,"compose","up","-d"},
  {"--host",executorhost.LocalSocket,"compose","version","--short","--plugin","other"},
  {"--host","tcp://other:2375","compose","version","--short"},
 }{
  if approvedHostRead(args){t.Fatalf("unsafe version command accepted: %#v",args)}
 }
}
