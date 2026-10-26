package store

import (
 "context"
 "path/filepath"
 "strings"
 "testing"
)

func TestImageEventHistoryIsExactAndCursorBounded(t *testing.T) {
 db,err:=Open(filepath.Join(t.TempDir(),"image-history.db"))
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 ctx:=context.Background()
 a:="registry-1.docker.io/library/nginx:latest"
 b:="ghcr.io/example/other:latest"
 amd,arm:="linux/amd64","linux/arm64"
 d1:="sha256:"+strings.Repeat("a",64)
 d2:="sha256:"+strings.Repeat("b",64)
 d3:="sha256:"+strings.Repeat("c",64)
 pairs:=[]struct{image,platform,digest string}{
  {a,amd,d1},{a,amd,d2},{a,amd,d2},
  {b,amd,d1},{b,amd,d3},
  {a,arm,d1},{a,arm,d3},
  {a,amd,d3},
 }
 for _,p:=range pairs{
  if _,err:=db.RecordRegistryCheck(ctx,p.image,p.platform,p.digest,p.digest);err!=nil{t.Fatal(err)}
 }
 events,err:=db.ImageEvents(ctx,a,amd,0,100)
 if err!=nil{t.Fatal(err)}
 if len(events)!=2{t.Fatalf("expected only two digest movements for image/platform, got %d",len(events))}
 for _,event:=range events{
  if event.Image!=a || event.Platform!=amd || event.Kind!="remote_digest_changed"{t.Errorf("leaked unrelated event: %+v",event)}
 }
 if events[0].ID<=events[1].ID{t.Fatal("history must be latest first")}
 first,err:=db.ImageEvents(ctx,a,amd,0,1)
 if err!=nil || len(first)!=1{t.Fatalf("first page: %+v, %v",first,err)}
 older,err:=db.ImageEvents(ctx,a,amd,first[0].ID,1)
 if err!=nil || len(older)!=1 || older[0].ID!=events[1].ID{t.Fatalf("older page: %+v, %v",older,err)}
 end,err:=db.ImageEvents(ctx,a,amd,older[0].ID,1)
 if err!=nil || len(end)!=0{t.Fatalf("unexpected next page: %+v, %v",end,err)}
 changes,err:=db.LatestImageChangeEvents(ctx)
 if err!=nil{t.Fatal(err)}
 if len(changes)!=3{t.Fatalf("expected three distinct image/platform latest movements, got %+v",changes)}
 seen:=map[string]int64{}
 for _,event:=range changes{seen[event.Image+"|"+event.Platform]=event.ID}
 if seen[a+"|"+amd]!=first[0].ID{t.Fatal("latest change for shared image was not retained")}
 if seen[a+"|"+arm]==0 || seen[b+"|"+amd]==0{t.Fatal("missing independently scoped history")}
 if got,err:=db.ImageEvents(ctx,a+"' OR 1=1 --",amd,0,100);err!=nil || len(got)!=0 {
  t.Fatalf("unsafe search semantics: %+v %v",got,err)
 }
}
func TestImageEventHistoryRejectsInvalidQueries(t *testing.T) {
 db,err:=Open(filepath.Join(t.TempDir(),"invalid-history.db"))
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 ctx:=context.Background()
 for _,q:=range []struct{image,platform string;before int64;limit int}{
  {"","linux/amd64",0,20},{"nginx:latest","",0,20},
  {"nginx:latest","linux/amd64",-1,20},
  {"nginx:latest","linux/amd64",0,0},
  {"nginx:latest","linux/amd64",0,101},
 }{
  if _,err:=db.ImageEvents(ctx,q.image,q.platform,q.before,q.limit);err==nil{t.Errorf("accepted invalid query %+v",q)}
 }
}
