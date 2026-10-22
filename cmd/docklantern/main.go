package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "strings"
 "time"
 "github.com/docklantern/docklantern/internal/poc"
)
func main(){
 mode:=flag.String("mode","discover","discover, validate, prepare, apply")
 reg:=flag.String("registration","","registration JSON")
 roots:=flag.String("roots","","comma-separated source roots")
 svc:=flag.String("service","","registered service")
 img:=flag.String("image","","image reference")
 dir:=flag.String("output-dir","","private override directory")
 confirm:=flag.Bool("confirm-unsafe-poc-deploy",false,"explicit approval for disposable fixtures only")
 flag.Parse()
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 r:=poc.Shell{}
 if *mode=="discover"{v,e:=poc.Discover(ctx,r);if e!=nil{fatal(e)};_ =json.NewEncoder(os.Stdout).Encode(v);return}
 if *reg==""||*roots==""{fatal(fmt.Errorf("registration and roots are required"))}
 b,e:=os.ReadFile(*reg);if e!=nil{fatal(e)}
 var p poc.Registration;if e=json.Unmarshal(b,&p);e!=nil{fatal(e)}
 allowed:=strings.Split(*roots,",")
 switch *mode {
 case "validate":if e=p.Validate(allowed);e!=nil{fatal(e)};fmt.Println("registered paths validated; effective Compose model not checked")
 case "prepare":v,e:=p.Prepare(ctx,r,allowed,*svc,*img,*dir);if e!=nil{fatal(e)};fmt.Println(v)
 case "apply":v,e:=p.Apply(ctx,r,allowed,*svc,*img,*dir,*confirm);if e!=nil{fatal(e)};fmt.Println("disposable fixture deployment:",v)
 default:fatal(fmt.Errorf("unknown mode"))
 }
}
func fatal(e error){fmt.Fprintln(os.Stderr,e);os.Exit(1)}
