package executorjournal

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "time"

 "github.com/docklantern/docklantern/internal/executorregistration"
 "github.com/docklantern/docklantern/internal/executorreview"
)

var ErrLocalStatusDenied=errors.New("protected executor status unavailable")

// StatusLocal is an operator-only, read-only way to inspect an interrupted
// operation. It does not alter journal state, reconcile, replay, recover,
// promote an override or use the Docker socket.
func StatusLocal(ctx context.Context,id string)(Snapshot,error){
 var empty Snapshot
 if ctx==nil||ctx.Err()!=nil||os.Geteuid()!=0||
  !executorregistration.ValidID(id){return empty,ErrLocalStatusDenied}
 tty,err:=executorreview.OpenLocalOperatorTerminal()
 if err!=nil{return empty,ErrLocalStatusDenied}
 defer tty.Close()
 // Do not implicitly create a new journal during an informational query.
 info,err:=os.Lstat(filepath.Join(HostJournalDirectory,HostJournalFilename))
 if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()!=0o600{
  return empty,ErrLocalStatusDenied
 }
 store,err:=OpenHost()
 if err!=nil{return empty,ErrLocalStatusDenied}
 defer store.Close()
 deadline,cancel:=context.WithTimeout(ctx,5*time.Second)
 defer cancel()
 result,err:=store.Inspect(deadline,id)
 if err!=nil{return empty,ErrLocalStatusDenied}
 return result,nil
}
