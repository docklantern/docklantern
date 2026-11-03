package executorjournal

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "time"

 "golang.org/x/sys/unix"
)

// Phase 3c-c deliberately has no exported backup/restore API or CLI wiring.
// All path-taking helpers are private for disposable root-owned CI fixtures.
// No function can replace the active operations.sqlite or authorize work.
var errBackup = errors.New("protected executor backup unavailable")

const maxBackupBytes int64 = 256 << 20

type backupManifest struct {
 Version int `json:"version"`
 ID string `json:"id"`
 SHA256 string `json:"sha256"`
 Bytes int64 `json:"bytes"`
 Schema int `json:"schema"`
 ApplicationID int `json:"application_id"`
 CreatedNS int64 `json:"created_ns"`
}

func backupID() (string,error) {
 var b [16]byte
 if _,err:=rand.Read(b[:]);err!=nil{return "",err}
 return hex.EncodeToString(b[:]),nil
}

func backupNameValid(s string) bool {
 if len(s)!=32{return false}
 for _,c:=range s {if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}}
 return true
}

// openPrivateBackupDir enforces root-owned, nofollow, non-writable ancestors
// and an exactly 0700 preprovisioned final directory.
func openPrivateBackupDir(dir string) (int,error) {
 if os.Geteuid()!=0 || !filepath.IsAbs(dir) || filepath.Clean(dir)!=dir || dir=="/" || len(dir)>512 {
  return -1,errBackup
 }
 parts:=strings.Split(strings.TrimPrefix(dir,"/"),"/")
 if len(parts)<2 || len(parts)>32{return -1,errBackup}
 fd,e:=syscall.Open("/",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
 if e!=nil{return -1,errBackup}
 if !trustedJournalDir(fd,false){syscall.Close(fd);return -1,errBackup}
 for i,p:=range parts {
  if p==""||p=="."||p==".."{syscall.Close(fd);return -1,errBackup}
  next,e:=syscall.Openat(fd,p,syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
  syscall.Close(fd)
  if e!=nil{return -1,errBackup}
  fd=next
  if !trustedJournalDir(fd,i==len(parts)-1) {syscall.Close(fd);return -1,errBackup}
 }
 return fd,nil
}

func backupFileInfo(fd int) (int64,error) {
 var st syscall.Stat_t
 if syscall.Fstat(fd,&st)!=nil || st.Uid!=0 || st.Nlink!=1 ||
  st.Mode&syscall.S_IFMT!=syscall.S_IFREG || st.Mode&0o7777!=0o600 ||
  st.Size<=0 || st.Size>maxBackupBytes{return 0,errBackup}
 return st.Size,nil
}

func hashBackupFile(dirfd int,name string) (string,int64,error) {
 fd,e:=syscall.Openat(dirfd,name,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK,0)
 if e!=nil{return "",0,errBackup}
 f:=os.NewFile(uintptr(fd),"protected-backup")
 defer f.Close()
 size,e:=backupFileInfo(fd)
 if e!=nil{return "",0,e}
 h:=sha256.New()
 n,e:=io.Copy(h,io.LimitReader(f,maxBackupBytes+1))
 if e!=nil || n!=size{return "",0,errBackup}
 if _,e=backupFileInfo(fd);e!=nil{return "",0,errBackup}
 return "sha256:"+hex.EncodeToString(h.Sum(nil)),size,nil
}

func verifyBackupDB(ctx context.Context,path string) error {
 db,e:=sql.Open("sqlite","file:"+path+"?mode=ro&immutable=1")
 if e!=nil{return errBackup}
 defer db.Close()
 db.SetMaxOpenConns(1)
 var quick string
 if e=db.QueryRowContext(ctx,"PRAGMA integrity_check").Scan(&quick);e!=nil||quick!="ok"{return errBackup}
 var app,version int
 if e=db.QueryRowContext(ctx,"PRAGMA application_id").Scan(&app);e!=nil||app!=journalApplicationID{return errBackup}
 if e=db.QueryRowContext(ctx,"PRAGMA user_version").Scan(&version);e!=nil||version!=SchemaVersion{return errBackup}
 if !requiredSchemaObjects(ctx,db,true){return errBackup}
 var foreign int
 if e=db.QueryRowContext(ctx,"PRAGMA foreign_key_check").Scan(&foreign);e!=sql.ErrNoRows{return errBackup}
 return nil
}

func writeBackupManifest(dirfd int,name string,body []byte) error {
 if len(body)==0||len(body)>1024{return errBackup}
 fd,e:=syscall.Openat(dirfd,name,syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0o600)
 if e!=nil{return errBackup}
 f:=os.NewFile(uintptr(fd),"backup-manifest")
 n,e:=f.Write(body)
 if e!=nil||n!=len(body){f.Close();return errBackup}
 if e=f.Sync();e!=nil{f.Close();return errBackup}
 if e=f.Close();e!=nil{return errBackup}
 return syscall.Fsync(dirfd)
}

// backupJournalAt snapshots the active WAL database through SQLite VACUUM
// INTO, which includes committed WAL transactions without copying live
// SQLite/WAL/SHM files. A Store-local mutex serializes its own writers.
// Host-wide executor quiescence still requires an independently approved
// coordination mechanism before enabling any production backup command.
func (s *Store) backupJournalAt(ctx context.Context,dir string,at time.Time, checkpoint func(string)error) (string,error) {
 if s==nil||ctx==nil||ctx.Err()!=nil||!validAt(at)||os.Geteuid()!=0{return "",errBackup}
 dirfd,e:=openPrivateBackupDir(dir)
 if e!=nil{return "",errBackup}
 defer syscall.Close(dirfd)
 id,e:=backupID()
 if e!=nil{return "",errBackup}
 tmp:=".snapshot-"+id+".tmp"
 final:="snapshot-"+id+".sqlite"
 manifestName:="snapshot-"+id+".json"
 // A failed or interrupted snapshot is kept as an untrusted orphan for
 // deliberate offline inspection, never treated as a valid backup.
 s.mu.Lock()
 if s.db==nil{s.mu.Unlock();return "",errBackup}
 // The destination is a random new name within trusted root-only storage.
 // VACUUM INTO makes a transactionally consistent independent SQLite DB.
 _,e=s.db.ExecContext(ctx,"VACUUM INTO '"+filepath.Join(dir,tmp)+"'")
 s.mu.Unlock()
 if e!=nil{return "",errBackup}
 if checkpoint!=nil&&checkpoint("after_snapshot")!=nil{return "",errBackup}
 chmodFD,chErr:=syscall.Openat(dirfd,tmp,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if chErr!=nil{return "",errBackup}
 chErr=syscall.Fchmod(chmodFD,0o600)
 syscall.Close(chmodFD)
 if chErr!=nil{return "",errBackup}
 hash,size,e:=hashBackupFile(dirfd,tmp)
 if e!=nil{return "",errBackup}
 if e=verifyBackupDB(ctx,filepath.Join(dir,tmp));e!=nil{return "",errBackup}
 fd,e:=syscall.Openat(dirfd,tmp,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if e!=nil{return "",errBackup}
 e=syscall.Fsync(fd)
 syscall.Close(fd)
 if e!=nil{return "",errBackup}
 if checkpoint!=nil&&checkpoint("after_snapshot_sync")!=nil{return "",errBackup}
 if e=unix.Renameat2(dirfd,tmp,dirfd,final,unix.RENAME_NOREPLACE);e!=nil{return "",errBackup}
 if e=syscall.Fsync(dirfd);e!=nil{return "",errBackup}
 if checkpoint!=nil&&checkpoint("after_backup_rename")!=nil{return "",errBackup}
 m:=backupManifest{Version:1,ID:id,SHA256:hash,Bytes:size,Schema:SchemaVersion,ApplicationID:journalApplicationID,CreatedNS:at.UnixNano()}
 body,e:=json.Marshal(m)
 if e!=nil{return "",errBackup}
 if e=writeBackupManifest(dirfd,manifestName,body);e!=nil{return "",errBackup}
 if checkpoint!=nil&&checkpoint("after_manifest_sync")!=nil{return "",errBackup}
 if e=verifyBackupBundle(ctx,dirfd,dir,id);e!=nil{return "",errBackup}
 return id,nil
}

func verifyBackupBundle(ctx context.Context,fd int,dir,id string) error {
 if !backupNameValid(id){return errBackup}
 name:="snapshot-"+id+".json"
 manifestFD,e:=syscall.Openat(fd,name,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK,0)
 if e!=nil{return errBackup}
 f:=os.NewFile(uintptr(manifestFD),"backup-manifest")
 if _,e=backupFileInfo(manifestFD);e!=nil{f.Close();return errBackup}
 body,e:=io.ReadAll(io.LimitReader(f,1025))
 f.Close()
 if e!=nil||len(body)>1024{return errBackup}
 var m backupManifest
 if e=json.Unmarshal(body,&m);e!=nil{return errBackup}
 canonical,_:=json.Marshal(m)
 if string(canonical)!=string(body)||m.ID!=id||m.Version!=1||
  m.Schema!=SchemaVersion||m.ApplicationID!=journalApplicationID||
  !validHash(m.SHA256)||m.CreatedNS<=0{return errBackup}
 h,n,e:=hashBackupFile(fd,"snapshot-"+id+".sqlite")
 if e!=nil||h!=m.SHA256||n!=m.Bytes{return errBackup}
 return verifyBackupDB(ctx,filepath.Join(dir,"snapshot-"+id+".sqlite"))
}

// stageRestoreAt verifies an immutable bundle and stages a separate sealed
// database in an EMPTY root-private quarantine directory. Crucially, it
// NEVER replaces an active journal, removes WAL, changes project outcomes,
// or invokes Docker. Activation needs a later reviewed offline restore gate.
func stageRestoreAt(ctx context.Context,backupDir,quarantineDir,id string,checkpoint func(string)error) error {
 if ctx==nil||ctx.Err()!=nil||os.Geteuid()!=0||!backupNameValid(id){return errBackup}
 source,e:=openPrivateBackupDir(backupDir)
 if e!=nil{return errBackup}
 defer syscall.Close(source)
 target,e:=openPrivateBackupDir(quarantineDir)
 if e!=nil{return errBackup}
 defer syscall.Close(target)
 if backupDir==quarantineDir||!trustedJournalDir(target,true){return errBackup}
 if e=verifyBackupBundle(ctx,source,backupDir,id);e!=nil{return errBackup}
 // Refuse nonempty restore quarantine; no wildcard cleanup is performed.
 dup,e:=syscall.Openat(target,".",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
 if e!=nil{return errBackup}
 directoryFile:=os.NewFile(uintptr(dup),"restore-quarantine")
 entries,e:=directoryFile.ReadDir(1)
 _ = directoryFile.Close()
 if e!=nil && !errors.Is(e,io.EOF){return errBackup}
 if len(entries)!=0{return errBackup}
 in,e:=syscall.Openat(source,"snapshot-"+id+".sqlite",syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if e!=nil{return errBackup}
 input:=os.NewFile(uintptr(in),"verified-backup-source")
 defer input.Close()
 size,e:=backupFileInfo(in)
 if e!=nil{return errBackup}
 out,e:=syscall.Openat(target,".restore-"+id+".tmp",syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0o600)
 if e!=nil{return errBackup}
 output:=os.NewFile(uintptr(out),"quarantined-restore")
 n,e:=io.Copy(output,io.LimitReader(input,maxBackupBytes+1))
 if e!=nil||n!=size{output.Close();return errBackup}
 if e=output.Sync();e!=nil{output.Close();return errBackup}
 if e=output.Close();e!=nil{return errBackup}
 if checkpoint!=nil&&checkpoint("after_restore_sync")!=nil{return errBackup}
 restore:="restored-"+id+".sqlite"
 if e=unix.Renameat2(target,".restore-"+id+".tmp",target,restore,unix.RENAME_NOREPLACE);e!=nil{return errBackup}
 if e=syscall.Fsync(target);e!=nil{return errBackup}
 if checkpoint!=nil&&checkpoint("after_restore_rename")!=nil{return errBackup}
 h,n,e:=hashBackupFile(source,"snapshot-"+id+".sqlite")
 if e!=nil||n!=size{return errBackup}
 copied,copiedSize,e:=hashBackupFile(target,restore)
 if e!=nil||copied!=h||copiedSize!=size{return errBackup}
 return verifyBackupDB(ctx,filepath.Join(quarantineDir,restore))
}
