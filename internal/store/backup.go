package store

import(
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
)
// Backup creates a consistent SQLite snapshot using VACUUM INTO. The source
// remains online and WAL frames are included by SQLite. Backups never replace
// an existing destination and are restricted to private file permissions.
func(s *Store)Backup(ctx context.Context,destination string)error{
 if destination==""{return errors.New("backup path required")}
 dest,err:=filepath.Abs(destination);if err!=nil{return err}
 if strings.HasSuffix(dest,string(os.PathSeparator)){return errors.New("backup destination must be a file")}
 if _,err=os.Lstat(dest);err==nil{return errors.New("backup destination already exists")}else if !os.IsNotExist(err){return err}
 dir:=filepath.Dir(dest)
 if err=os.MkdirAll(dir,0700);err!=nil{return err}
 key:=make([]byte,16);if _,err=rand.Read(key);err!=nil{return err}
 temp:=filepath.Join(dir,".docklantern-backup-"+hex.EncodeToString(key)+".sqlite")
 defer os.Remove(temp)
 if _,err=s.db.ExecContext(ctx,"VACUUM INTO ?",temp);err!=nil{return fmt.Errorf("SQLite backup failed: %w",err)}
 if err=os.Chmod(temp,0600);err!=nil{return err}
 file,err:=os.OpenFile(temp,os.O_RDONLY,0);if err!=nil{return err}
 if err=file.Sync();err!=nil{file.Close();return err}
 if err=file.Close();err!=nil{return err}
 // Hard link offers atomic no-overwrite publication on the same filesystem.
 if err=os.Link(temp,dest);err!=nil{return fmt.Errorf("publish backup without overwrite: %w",err)}
 return nil
}
