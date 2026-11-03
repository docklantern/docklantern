package executorjournal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
	"golang.org/x/sys/unix"
)

const HostOverridesDirectory = "/var/lib/docklantern/executor/overrides"

// StageOverrideGeneration writes only an immutable, private metadata manifest,
// NEVER a Compose override YAML, committed desired state or Docker update.
// It requires a project lock and an in-session durable STAGE phase intent.
// The caller must separately provision the protected project directory.
func (s *Store) StageOverrideGeneration(ctx context.Context, lock *ProjectLock, m OverrideManifest, at time.Time) (string,error) {
	return s.stageOverrideAt(ctx,lock,HostOverridesDirectory,m,at,nil)
}

// stageOverrideAt allows disposable root fixture directories and injected
// crash points. No caller-provided filesystem path reaches the exported API.
func (s *Store) stageOverrideAt(ctx context.Context, lock *ProjectLock, root string,
	m OverrideManifest, at time.Time, checkpoint func(string)error) (string,error) {
	if ctx==nil || ctx.Err()!=nil || os.Geteuid()!=0 || lock==nil {
		return "",ErrOverrideStage
	}
	b,err:=m.bytes()
	if err!=nil || !validAt(at) {return "",ErrOverrideStage}
	h:=sha256.Sum256(b)
	hash:="sha256:"+hex.EncodeToString(h[:])
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.released || lock.fd<0 || lock.projectID!=m.ProjectID ||
		!trustedJournalFile(lock.fd) {return "",ErrOverrideStage}
	dirfd,err:=openOverrideProjectDir(root,m.ProjectID)
	if err!=nil{return "",ErrOverrideStage}
	defer syscall.Close(dirfd)
	if err:=s.appendStageIntent(ctx,m,hash,at);err!=nil{return "",ErrOverrideStage}
	if checkpoint!=nil && checkpoint("after_stage_intent")!=nil{return "",ErrOverrideStage}
	if err:=publishImmutableManifest(dirfd,m.Generation,b);err!=nil{return "",ErrOverrideStage}
	if checkpoint!=nil && checkpoint("after_manifest_sync")!=nil{return "",ErrOverrideStage}
	if err:=verifyManifest(dirfd,m.Generation,hash);err!=nil{return "",ErrOverrideStage}
	if err:=s.appendStaged(ctx,m,hash,at.Add(time.Nanosecond));err!=nil{return "",ErrOverrideStage}
	if checkpoint!=nil && checkpoint("after_staged_event")!=nil{return "",ErrOverrideStage}
	return hash,nil
}

// RecordOverridePromotionIntent records ONLY journal intent. It does not
// rename, copy or promote any metadata/Compose file, write to Docker,
// mark the operation successful, or publish an authorized receipt.
// A subsequent Phase 4/5 design must validate real state and independently
// approve filesystem promotion + terminal success recording.
func (s *Store) RecordOverridePromotionIntent(ctx context.Context, lock *ProjectLock,
	m OverrideManifest, expectedSHA256 string, at time.Time) error {
	return s.promotionIntentAt(ctx,lock,HostOverridesDirectory,m,expectedSHA256,at)
}

func (s *Store) promotionIntentAt(ctx context.Context, lock *ProjectLock, root string,
	m OverrideManifest, hash string, at time.Time) error {
	if ctx==nil || ctx.Err()!=nil || os.Geteuid()!=0 || lock==nil || !validHash(hash) ||
		!validAt(at) {return ErrOverrideStage}
	b,err:=m.bytes()
	if err!=nil {return ErrOverrideStage}
	h:=sha256.Sum256(b)
	if hash!="sha256:"+hex.EncodeToString(h[:]) {return ErrOverrideStage}
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.released || lock.fd<0 || lock.projectID!=m.ProjectID ||
		!trustedJournalFile(lock.fd) {return ErrOverrideStage}
	dirfd,err:=openOverrideProjectDir(root,m.ProjectID)
	if err!=nil{return ErrOverrideStage}
	defer syscall.Close(dirfd)
	if err:=verifyManifest(dirfd,m.Generation,hash);err!=nil{return ErrOverrideStage}
	if err:=s.appendPromotionIntent(ctx,m,hash,at);err!=nil{return ErrOverrideStage}
	return nil
}

func openOverrideProjectDir(root,projectID string) (int,error) {
	if os.Geteuid()!=0 || !executorregistration.ValidID(projectID) ||
		!filepath.IsAbs(root) || root=="/" || filepath.Clean(root)!=root ||
		len(root)>512 {return -1,ErrOverrideStage}
	parts:=strings.Split(strings.TrimPrefix(root,"/"),"/")
	if len(parts)<2 || len(parts)>30 {return -1,ErrOverrideStage}
	parts=append(parts,projectID)
	fd,err:=syscall.Open("/",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
	if err!=nil{return -1,ErrOverrideStage}
	if !trustedJournalDir(fd,false) {syscall.Close(fd);return -1,ErrOverrideStage}
	for i,part:=range parts {
		if part=="" || part=="." || part==".." {syscall.Close(fd);return -1,ErrOverrideStage}
		next,e:=syscall.Openat(fd,part,syscall.O_RDONLY|syscall.O_DIRECTORY|
			syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
		syscall.Close(fd)
		if e!=nil{return -1,ErrOverrideStage}
		fd=next
		if !trustedJournalDir(fd,i>=len(parts)-2) {
			syscall.Close(fd)
			return -1,ErrOverrideStage
		}
	}
	return fd,nil
}

func overrideFilename(generation int64) string {
	return fmt.Sprintf("g%018d.json",generation)
}

func publishImmutableManifest(dirfd int,gen int64,data []byte) error {
	if gen<1 || gen>999999999999999999 || len(data)==0 || len(data)>1024 ||
		!trustedJournalDir(dirfd,true) {return ErrOverrideStage}
	nonce:=make([]byte,16)
	if _,err:=rand.Read(nonce);err!=nil{return ErrOverrideStage}
	tmp:=".staged-"+hex.EncodeToString(nonce)+".tmp"
	fd,err:=syscall.Openat(dirfd,tmp,syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|
		syscall.O_CLOEXEC|syscall.O_NOFOLLOW,0o600)
	if err!=nil{return ErrOverrideStage}
	// Own unguessable temp only: never unlink a final generation.
	defer syscall.Unlinkat(dirfd,tmp)
	file:=os.NewFile(uintptr(fd),"override-manifest-temp")
	n,writeErr:=file.Write(data)
	if writeErr!=nil || n!=len(data) {file.Close();return ErrOverrideStage}
	if !trustedJournalFile(fd) {file.Close();return ErrOverrideStage}
	if err:=file.Sync();err!=nil {file.Close();return ErrOverrideStage}
	if err:=file.Close();err!=nil{return ErrOverrideStage}
	if !trustedJournalDir(dirfd,true) {return ErrOverrideStage}
	if err:=unix.Renameat2(dirfd,tmp,dirfd,overrideFilename(gen),unix.RENAME_NOREPLACE);err!=nil {
		return ErrOverrideStage
	}
	// An error after the rename is ambiguous, never auto-remove.
	if err:=syscall.Fsync(dirfd);err!=nil{return ErrOverrideStage}
	return nil
}

func verifyManifest(dirfd int,gen int64,hash string) error {
	if !trustedJournalDir(dirfd,true) || !validHash(hash) {return ErrOverrideStage}
	fd,err:=syscall.Openat(dirfd,overrideFilename(gen),syscall.O_RDONLY|
		syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC,0)
	if err!=nil{return ErrOverrideStage}
	file:=os.NewFile(uintptr(fd),"override-manifest-verify")
	defer file.Close()
	var before,after syscall.Stat_t
	if syscall.Fstat(fd,&before)!=nil || !trustedJournalFile(fd) ||
		before.Size<1 || before.Size>1024 {return ErrOverrideStage}
	data,readErr:=io.ReadAll(io.LimitReader(file,1025))
	if readErr!=nil || int64(len(data))!=before.Size ||
		syscall.Fstat(fd,&after)!=nil || before.Dev!=after.Dev ||
		before.Ino!=after.Ino || before.Size!=after.Size ||
		before.Mtim!=after.Mtim || before.Ctim!=after.Ctim {return ErrOverrideStage}
	h:=sha256.Sum256(data)
	if hash!="sha256:"+hex.EncodeToString(h[:]) {return ErrOverrideStage}
	return nil
}
