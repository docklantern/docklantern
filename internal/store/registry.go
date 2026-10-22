package store

import (
 "context"
 "database/sql"
 "errors"
 "time"
)
// RegistryCheck compares successive remote resolutions, NOT a running container.
type RegistryCheck struct{
 Image string `json:"image"`
 Platform string `json:"platform"`
 IndexDigest string `json:"index_digest"`
 PlatformDigest string `json:"platform_digest"`
 PreviousDigest string `json:"previous_digest,omitempty"`
 Status string `json:"status"` // first_seen, unchanged, changed
 CheckedAt string `json:"checked_at"`
}
func(s *Store) RecordRegistryCheck(ctx context.Context,image,platform,indexDigest,platformDigest string)(RegistryCheck,error){
 if image==""||platform==""||indexDigest==""||platformDigest==""{return RegistryCheck{},errors.New("missing registry check identity")}
 tx,e:=s.db.BeginTx(ctx,nil);if e!=nil{return RegistryCheck{},e};defer tx.Rollback()
 var prev string
 e=tx.QueryRowContext(ctx,"SELECT platform_digest FROM registry_checks WHERE image=? AND platform=?",image,platform).Scan(&prev)
 if e!=nil&&e!=sql.ErrNoRows{return RegistryCheck{},e}
 status:="first_seen"
 if e==nil {status="unchanged";if prev!=platformDigest{status="changed"}}
 now:=time.Now().UTC().Format(time.RFC3339)
 _,e=tx.ExecContext(ctx,`INSERT INTO registry_checks(image,platform,index_digest,platform_digest,previous_digest,status,checked_at)
 VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(image,platform) DO UPDATE SET
 index_digest=excluded.index_digest,platform_digest=excluded.platform_digest,
 previous_digest=excluded.previous_digest,status=excluded.status,checked_at=excluded.checked_at`,
 image,platform,indexDigest,platformDigest,prev,status,now)
 if e!=nil{return RegistryCheck{},e}
 if e=tx.Commit();e!=nil{return RegistryCheck{},e}
 return RegistryCheck{Image:image,Platform:platform,IndexDigest:indexDigest,PlatformDigest:platformDigest,PreviousDigest:prev,Status:status,CheckedAt:now},nil
}
func(s *Store)RegistryChecks(ctx context.Context)([]RegistryCheck,error){
 rows,e:=s.db.QueryContext(ctx,"SELECT image,platform,index_digest,platform_digest,previous_digest,status,checked_at FROM registry_checks ORDER BY image,platform")
 if e!=nil{return nil,e};defer rows.Close()
 out:=[]RegistryCheck{}
 for rows.Next(){var x RegistryCheck
  if e=rows.Scan(&x.Image,&x.Platform,&x.IndexDigest,&x.PlatformDigest,&x.PreviousDigest,&x.Status,&x.CheckedAt);e!=nil{return nil,e}
  out=append(out,x)
 }
 return out,rows.Err()
}
