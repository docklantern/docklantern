package store

import (
 "context"
 "errors"
)

// LatestImageChangeEvents returns only the latest *retained* change event for
// each image/platform identity. This avoids a fixed-size global event window
// hiding historical changes when unrelated images are noisy. Event deletion
// can remove prior changes, so callers must label these as recorded history.
func (s *Store) LatestImageChangeEvents(ctx context.Context)([]Event,error) {
 rows,err:=s.db.QueryContext(ctx,`SELECT `+eventColumns+` FROM monitor_events
  WHERE kind='remote_digest_changed' AND id IN (
   SELECT MAX(id) FROM monitor_events WHERE kind='remote_digest_changed'
   GROUP BY image,platform)
  ORDER BY id DESC`)
 if err!=nil{return nil,err}
 defer rows.Close()
 events:=[]Event{}
 for rows.Next(){
  event,err:=scanEvent(rows);if err!=nil{return nil,err}
  events=append(events,event)
 }
 return events,rows.Err()
}

// ImageEvents reads precisely one image/platform's recorded history. Filtering
// in SQL (rather than after a global pagination limit) avoids missing shared
// image events and prevents cross-image attribution on detail pages.
func (s *Store) ImageEvents(ctx context.Context,image,platform string,before int64,limit int)([]Event,error) {
 if image=="" || platform=="" || len(image)>4096 || len(platform)>128 || before<0 || limit<1 || limit>100 {
  return nil,errors.New("invalid image event query")
 }
 query:="SELECT "+eventColumns+" FROM monitor_events WHERE image=? AND platform=?"
 args:=[]any{image,platform}
 if before>0{
  query+=" AND id<?"
  args=append(args,before)
 }
 query+=" ORDER BY id DESC LIMIT ?"
 args=append(args,limit)
 rows,err:=s.db.QueryContext(ctx,query,args...)
 if err!=nil{return nil,err}
 defer rows.Close()
 events:=[]Event{}
 for rows.Next(){
  event,err:=scanEvent(rows);if err!=nil{return nil,err}
  events=append(events,event)
 }
 return events,rows.Err()
}
