// docklantern-registry resolves OCI image identity without pulling/deploying.
// Credentials are accepted ONLY from environment, never command-line arguments.
package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "runtime"
 "time"
 "github.com/docklantern/docklantern/internal/registry"
)
func main(){
 image:=flag.String("image","","OCI image reference (tag, not digest)")
 platform:=flag.String("platform",runtime.GOOS+"/"+runtime.GOARCH,"target os/architecture[/variant]")
 flag.Parse()
 if *image==""{fmt.Fprintln(os.Stderr,"-image required");os.Exit(2)}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 user:=os.Getenv("DOCKLANTERN_REGISTRY_USERNAME")
 token:=os.Getenv("DOCKLANTERN_REGISTRY_TOKEN")
 if token!=""&&user==""{fmt.Fprintln(os.Stderr,"registry username required with token");os.Exit(2)}
 result,err:=(registry.Client{Credentials:registry.Credentials{Username:user,Password:token}}).Check(ctx,*image,*platform)
 if err!=nil{fmt.Fprintln(os.Stderr,"registry check failed:",err);os.Exit(1)}
 if err=json.NewEncoder(os.Stdout).Encode(result);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
