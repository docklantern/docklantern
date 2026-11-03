package executorjournal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
	"golang.org/x/sys/unix"
)

// HostReceiptDirectory is an independently provisioned, root-owned outbox.
// The monitor may only see this directory through a separately approved
// READ-ONLY mount. Phase 3c-b does not configure any container mounts.
const HostReceiptDirectory = "/var/lib/docklantern/handoff/receipts"

const ReceiptFileMode = 0o644

// PublishReceipt is intentionally host-root only, journal-derived and fixed-
// directory. A successful publication means ONLY that a sanitized snapshot
// reached the outbox; it is never evidence of a successful Docker update.
func (s *Store) PublishReceipt(ctx context.Context, lock *ProjectLock, operationID string, at time.Time) (ReceiptPublication,error) {
	return s.publishReceiptAt(ctx,lock,HostReceiptDirectory,operationID,at,nil)
}

// publishReceiptAt is internal for isolated root-owned fixtures and crash
// injection. checkpoint callbacks are NEVER part of the production API.
func (s *Store) publishReceiptAt(ctx context.Context, lock *ProjectLock, dir, id string,
	at time.Time, checkpoint func(string) error) (ReceiptPublication,error) {
	var zero ReceiptPublication
	if s==nil || ctx==nil || ctx.Err()!=nil || !validAt(at) ||
		os.Geteuid()!=0 || lock==nil || !executorregistration.ValidID(id) {
		return zero,ErrReceiptUnavailable
	}
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.released || lock.fd<0 || lock.projectID=="" ||
		!trustedJournalFile(lock.fd) {return zero,ErrReceiptUnavailable}
	receipt,err:=s.receiptForOperation(ctx,id)
	if err!=nil || !executorregistration.ValidID(lock.projectID) {
		return zero,ErrReceiptUnavailable
	}
	// Bind the held project lock to the journal operation, never to a
	// monitor-provided project label or filename.
	var project string
	s.mu.Lock()
	if s.db!=nil {
		err=s.db.QueryRowContext(ctx,"SELECT project_id FROM operations WHERE operation_id=?",id).Scan(&project)
	}else{err=ErrUnavailable}
	s.mu.Unlock()
	if err!=nil || project!=lock.projectID {return zero,ErrReceiptUnavailable}
	data,err:=receipt.canonical()
	if err!=nil {return zero,ErrReceiptUnavailable}
	hash:=receiptHash(data)
	fd,err:=openReceiptOutbox(dir)
	if err!=nil{return zero,ErrReceiptUnavailable}
	defer syscall.Close(fd)
	previous,err:=s.reserveReceipt(ctx,id,hash,at)
	if err!=nil{return zero,ErrReceiptUnavailable}
	if checkpoint!=nil && checkpoint("after_pending")!=nil{return zero,ErrReceiptUnavailable}
	filename:=receipt.OperationID+"-v1.json"
	if err:=publishOrVerifyReceiptFile(fd,filename,data,hash);err!=nil{return zero,ErrReceiptUnavailable}
	if checkpoint!=nil && checkpoint("after_file_sync")!=nil{return zero,ErrReceiptUnavailable}
	if err:=s.markReceiptPublished(ctx,id,hash,at);err!=nil{return zero,ErrReceiptUnavailable}
	if checkpoint!=nil && checkpoint("after_published")!=nil{return zero,ErrReceiptUnavailable}
	return ReceiptPublication{ContentSHA256:hash,FilePublished:true,
		AlreadyPublished:previous,Outcome:receipt.Outcome},nil
}

func (s *Store) receiptForOperation(ctx context.Context,id string) (Receipt,error) {
	if s==nil || ctx==nil || ctx.Err()!=nil {return Receipt{},ErrReceiptUnavailable}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.journalReceiptSnapshot(ctx,id)
}

// openReceiptOutbox requires every ancestor to be root-owned with no
// group/world writable permission, and the leaf to be exactly 0755,
// root-owned. This permits bounded sanitized reads by a dedicated monitor
// process only if its separately approved mount is explicitly read-only.
func openReceiptOutbox(directory string) (int,error) {
	if os.Geteuid()!=0 || !filepath.IsAbs(directory) || directory=="/" ||
		len(directory)>512 || filepath.Clean(directory)!=directory {
		return -1,ErrReceiptUnavailable
	}
	parts:=strings.Split(strings.TrimPrefix(directory,"/"),"/")
	if len(parts)<2 || len(parts)>32{return -1,ErrReceiptUnavailable}
	fd,err:=syscall.Open("/",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
	if err!=nil{return -1,ErrReceiptUnavailable}
	if !trustedJournalDir(fd,false) {syscall.Close(fd);return -1,ErrReceiptUnavailable}
	for i,part:=range parts {
		if part==""||part=="."||part==".." {syscall.Close(fd);return -1,ErrReceiptUnavailable}
		next,e:=syscall.Openat(fd,part,syscall.O_RDONLY|syscall.O_DIRECTORY|
			syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
		syscall.Close(fd)
		if e!=nil{return -1,ErrReceiptUnavailable}
		fd=next
		if !trustedReceiptDirectory(fd,i==len(parts)-1) {
			syscall.Close(fd)
			return -1,ErrReceiptUnavailable
		}
	}
	return fd,nil
}

func trustedReceiptDirectory(fd int,final bool) bool {
	var st syscall.Stat_t
	if syscall.Fstat(fd,&st)!=nil || st.Mode&syscall.S_IFMT!=syscall.S_IFDIR ||
		st.Uid!=0 || st.Mode&0o022!=0 {return false}
	return !final || st.Mode&0o7777==0o755
}

func trustedReceiptFile(fd int) bool {
	var st syscall.Stat_t
	return syscall.Fstat(fd,&st)==nil && st.Mode&syscall.S_IFMT==syscall.S_IFREG &&
		st.Uid==0 && st.Nlink==1 && st.Mode&0o7777==ReceiptFileMode &&
		st.Size>0 && st.Size<=MaxReceiptBytes
}

func receiptFilenameAllowed(name string) (bool,bool) {
	if strings.HasPrefix(name,".receipt-") && strings.HasSuffix(name,".tmp") {
		if len(name)!=len(".receipt-")+32+len(".tmp"){return false,false}
		for _,ch:=range name[len(".receipt-"):len(".receipt-")+32] {
			if (ch<'0'||ch>'9')&&(ch<'a'||ch>'f'){return false,false}
		}
		return true,false
	}
	if !strings.HasSuffix(name,"-v1.json"){return false,false}
	id:=strings.TrimSuffix(name,"-v1.json")
	return executorregistration.ValidID(id),true
}

func boundedReceiptNames(dirfd int) (int,int,error) {
	dup,err:=syscall.Openat(dirfd,".",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
	if err!=nil{return 0,0,ErrReceiptUnavailable}
	d:=os.NewFile(uintptr(dup),"receipt-directory-scan")
	entries,err:=d.ReadDir(MaxReceiptEntries+1)
	_ = d.Close()
	if err!=nil || len(entries)>MaxReceiptEntries{return 0,0,ErrReceiptUnavailable}
	published:=0
	for _,e:=range entries {
		allowed,final:=receiptFilenameAllowed(e.Name())
		if !allowed {return 0,0,ErrReceiptUnavailable}
		if final {published++}
	}
	return published,len(entries),nil
}

func publishOrVerifyReceiptFile(dirfd int,name string,data []byte,hash string) error {
	if !trustedReceiptDirectory(dirfd,true) || len(data)==0 ||
		len(data)>MaxReceiptBytes || !validHash(hash) {
		return ErrReceiptUnavailable
	}
	allowed,final:=receiptFilenameAllowed(name)
	if !allowed || !final {return ErrReceiptUnavailable}
	// If the final name already exists, no overwrite or chmod is allowed:
	// only byte-for-byte exact, owner/mode/inode-verified replay succeeds.
	if err:=verifyReceiptFile(dirfd,name,hash);err==nil {return nil}
	// An existing untrusted/conflicting final file must fail; never replace.
	var st syscall.Stat_t
	if err:=syscall.Fstatat(dirfd,name,&st,syscall.AT_SYMLINK_NOFOLLOW);err==nil {
		return ErrReceiptUnavailable
	}else if !errors.Is(err,syscall.ENOENT) {return ErrReceiptUnavailable}
	count,total,err:=boundedReceiptNames(dirfd)
	if err!=nil || count>=MaxPublishedReceipts || total>=MaxReceiptEntries {
		return ErrReceiptUnavailable
	}
	nonce:=make([]byte,16)
	if _,err:=rand.Read(nonce);err!=nil {return ErrReceiptUnavailable}
	tmp:=".receipt-"+hex.EncodeToString(nonce)+".tmp"
	newfd,err:=syscall.Openat(dirfd,tmp,syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|
		syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0o600)
	if err!=nil{return ErrReceiptUnavailable}
	defer syscall.Unlinkat(dirfd,tmp)
	f:=os.NewFile(uintptr(newfd),"private-receipt-temp")
	n,err:=f.Write(data)
	if err!=nil || n!=len(data) {
		_ = f.Close()
		return ErrReceiptUnavailable
	}
	if err:=syscall.Fchmod(newfd,ReceiptFileMode);err!=nil {
		_ = f.Close()
		return ErrReceiptUnavailable
	}
	if !trustedReceiptFile(newfd) {
		_ = f.Close()
		return ErrReceiptUnavailable
	}
	if err:=f.Sync();err!=nil {
		_ = f.Close()
		return ErrReceiptUnavailable
	}
	if err:=f.Close();err!=nil{return ErrReceiptUnavailable}
	if !trustedReceiptDirectory(dirfd,true){return ErrReceiptUnavailable}
	if err:=unix.Renameat2(dirfd,tmp,dirfd,name,unix.RENAME_NOREPLACE);err!=nil {
		return ErrReceiptUnavailable
	}
	// A sync error after atomic rename is ambiguous, never unlink a final.
	if err:=syscall.Fsync(dirfd);err!=nil{return ErrReceiptUnavailable}
	return verifyReceiptFile(dirfd,name,hash)
}

func verifyReceiptFile(dirfd int,name,hash string) error {
	if !trustedReceiptDirectory(dirfd,true) || !validHash(hash) {
		return ErrReceiptUnavailable
	}
	fd,err:=syscall.Openat(dirfd,name,syscall.O_RDONLY|syscall.O_NONBLOCK|
		syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
	if err!=nil{return ErrReceiptUnavailable}
	f:=os.NewFile(uintptr(fd),"public-sanitized-receipt")
	defer f.Close()
	var before,after syscall.Stat_t
	if syscall.Fstat(fd,&before)!=nil || !trustedReceiptFile(fd) {
		return ErrReceiptUnavailable
	}
	data,err:=io.ReadAll(io.LimitReader(f,MaxReceiptBytes+1))
	if err!=nil || len(data)==0 || len(data)>MaxReceiptBytes ||
		int64(len(data))!=before.Size {return ErrReceiptUnavailable}
	if syscall.Fstat(fd,&after)!=nil || before.Dev!=after.Dev ||
		before.Ino!=after.Ino || before.Mode!=after.Mode ||
		before.Uid!=after.Uid || before.Nlink!=after.Nlink ||
		before.Size!=after.Size || before.Mtim!=after.Mtim ||
		before.Ctim!=after.Ctim {return ErrReceiptUnavailable}
	if receiptHash(data)!=hash{return ErrReceiptUnavailable}
	r,err:=ParseReceipt(data)
	if err!=nil || name!=r.OperationID+"-v1.json"{return ErrReceiptUnavailable}
	// Directory is root-owned and nonwritable by the monitor, so name
	// substitution cannot be triggered by an unprivileged monitor process.
	return nil
}
