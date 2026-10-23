package store
import(
 "context"
 "os"
 "path/filepath"
 "testing"
)
func TestBackupSnapshotAndNoOverwrite(t *testing.T){
 root:=t.TempDir()
 db,err:=Open(filepath.Join(root,"live","docklantern.db"));if err!=nil{t.Fatal(err)};defer db.Close()
 ctx:=context.Background()
 if err=db.Register(ctx,Registration{Name:"demo",ProjectDir:"/tmp/demo",Files:[]string{"/tmp/demo/compose.yaml"},Services:[]string{"web"}});err!=nil{t.Fatal(err)}
 dest:=filepath.Join(root,"backups","snapshot.sqlite")
 if err=db.Backup(ctx,dest);err!=nil{t.Fatal(err)}
 info,err:=os.Stat(dest);if err!=nil{t.Fatal(err)}
 if info.Mode().Perm()!=0600{t.Fatalf("backup permissions %v",info.Mode())}
 if err=db.Backup(ctx,dest);err==nil{t.Fatal("backup overwrote existing destination")}
 saved,err:=Open(dest);if err!=nil{t.Fatal(err)};defer saved.Close()
 projects,err:=saved.Projects(ctx);if err!=nil||len(projects)!=1||projects[0].Name!="demo"{t.Fatalf("backup data %+v %v",projects,err)}
 if _,err=saved.NotificationSettings(ctx);err!=nil{t.Fatal(err)}
}
