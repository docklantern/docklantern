package executorjournal

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	HostJournalDirectory = "/var/lib/docklantern/executor/journal"
	HostJournalFilename = "operations.sqlite"
)

// OpenHost opens only the fixed, independently protected executor journal.
// The host administrator must provision the parent directory separately;
// no directory, mount, privilege, Docker socket, or monitoring volume is
// created or modified by Phase 3a. Neither CLI nor web calls this function.
func OpenHost() (*Store, error) {
	if os.Geteuid() != 0 { return nil, ErrUnavailable }
	return openProtectedJournal(HostJournalDirectory)
}

func openProtectedJournal(directory string) (*Store, error) {
	if os.Geteuid() != 0 || !filepath.IsAbs(directory) || directory == "/" ||
		filepath.Clean(directory) != directory || len(directory) > 512 {
		return nil, ErrUnavailable
	}
	parts := strings.Split(strings.TrimPrefix(directory, "/"), "/")
	if len(parts) < 2 || len(parts) > 32 { return nil, ErrUnavailable }
	fd, err := syscall.Open("/",syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC,0)
	if err != nil { return nil, ErrUnavailable }
	defer func() { _ = syscall.Close(fd) }()
	if !trustedJournalDir(fd,false) { return nil, ErrUnavailable }
	for i, component := range parts {
		if component == "" || component == "." || component == ".." { return nil, ErrUnavailable }
		next, err := syscall.Openat(fd,component,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
		if err != nil { return nil, ErrUnavailable }
		syscall.Close(fd)
		fd=next
		if !trustedJournalDir(fd,i==len(parts)-1) { return nil, ErrUnavailable }
	}
	// Only a protected 0600 singly-linked root-owned regular file is
	// accepted. Creation is exclusive; no symlink/hardlink/truncate path.
	created := false
	filefd, err := syscall.Openat(fd,HostJournalFilename,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0o600)
	if err == nil {
		created = true
	} else if err == syscall.EEXIST {
		filefd, err = syscall.Openat(fd,HostJournalFilename,
			syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
	}
	if err != nil { return nil, ErrUnavailable }
	defer syscall.Close(filefd)
	if !trustedJournalFile(filefd) { return nil, ErrUnavailable }
	if created {
		if err:=syscall.Fsync(filefd);err!=nil {return nil,ErrUnavailable}
		if err:=syscall.Fsync(fd);err!=nil {return nil,ErrUnavailable}
	}
	// WAL and SHM, if present after a previous run, must also be owned by
	// root, private, regular and singly linked; never follow link names.
	for _, sidecar:=range []string{HostJournalFilename+"-wal",HostJournalFilename+"-shm"} {
		sidefd, err := syscall.Openat(fd,sidecar,
			syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
		if err == syscall.ENOENT { continue }
		if err != nil { return nil, ErrUnavailable }
		ok:=trustedJournalFile(sidefd)
		syscall.Close(sidefd)
		if !ok { return nil, ErrUnavailable }
	}
	if !trustedJournalDir(fd,true) { return nil, ErrUnavailable }
	store, err := openSQLite(filepath.Join(directory,HostJournalFilename))
	if err != nil { return nil, err }
	// Confirm the database that SQLite opened retains trusted inode,
	// ownership and permissions. All directory components are root owned,
	// so nonroot monitoring cannot replace them or their children.
	check, err := syscall.Openat(fd,HostJournalFilename,
		syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
	if err != nil { store.Close();return nil,ErrUnavailable }
	ok:=trustedJournalFile(check)
	var before,after syscall.Stat_t
	_ = syscall.Fstat(filefd,&before)
	_ = syscall.Fstat(check,&after)
	syscall.Close(check)
	if !ok || before.Dev!=after.Dev || before.Ino!=after.Ino {
		store.Close()
		return nil,ErrUnavailable
	}
	return store,nil
}

func trustedJournalDir(fd int, last bool) bool {
	var st syscall.Stat_t
	if syscall.Fstat(fd,&st)!=nil || st.Uid!=0 ||
		st.Mode&syscall.S_IFMT!=syscall.S_IFDIR ||
		st.Mode&0o022!=0 {return false}
	return !last || st.Mode&0o077==0 && st.Mode&0o700==0o700
}

func trustedJournalFile(fd int) bool {
	var st syscall.Stat_t
	return syscall.Fstat(fd,&st)==nil && st.Uid==0 &&
		st.Mode&syscall.S_IFMT==syscall.S_IFREG && st.Mode&0o7777==0o600 &&
		st.Nlink==1
}
