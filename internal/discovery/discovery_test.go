package discovery
import (
 "context"
 "strings"
 "testing"
)
type fake struct{}
func(fake)Run(_ context.Context,_ string,args ...string)([]byte,error){
 if args[0]=="ps"{return []byte("a b"),nil}
 if args[0]=="inspect"{return []byte(`[{"Id":"id1","Name":"/web","Image":"sha256:123","Config":{"Image":"nginx:1","Labels":{"com.docker.compose.project":"demo","com.docker.compose.service":"web"}},"State":{"Status":"running","Health":{"Status":"healthy"}}},{"Id":"id2","Name":"/unrelated","Config":{"Image":"alpine"}}]`),nil}
 return nil,nil
}
func TestScan(t *testing.T){
 x,e:=Scan(context.Background(),fake{});if e!=nil{t.Fatal(e)}
 if len(x)!=1||x[0].Project!="demo"||x[0].Health!="healthy"||!strings.HasPrefix(x[0].ImageID,"sha256:"){t.Fatalf("%+v",x)}
}
