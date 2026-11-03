package executorreview

import (
	"bufio"
	"io"
	"context"
	"os"
	"strings"
	"sync"
	"syscall"
)

// OpenLocalOperatorTerminal requires an actual host-root process with a
// controlling terminal. No stdin/pipe fallback, no web call, and no --yes.
// The binary is NOT wired to this adapter while Docker baseline verification
// and a trusted monitor UID configuration remain unavailable.
func OpenLocalOperatorTerminal() (*LocalTerminal, error) {
	if os.Geteuid() != 0 {
		return nil, ErrReviewDenied
	}
	file, err := os.OpenFile("/dev/tty", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrReviewDenied
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		file.Close()
		return nil, ErrReviewDenied
	}
	return &LocalTerminal{file: file}, nil
}

type LocalTerminal struct {
	file *os.File
	mu sync.Mutex
}

func (t *LocalTerminal) Interactive() bool { return t != nil && t.file != nil }
func (t *LocalTerminal) Close() error {
	if t == nil || t.file == nil { return nil }
	return t.file.Close()
}
func (t *LocalTerminal) Print(s string) error {
	if !t.Interactive() { return ErrReviewDenied }
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.file.WriteString(s)
	return err
}

func (t *LocalTerminal) ReadLine(ctx context.Context) (string, error) {
	if !t.Interactive() || ctx == nil {
		return "", ErrReviewDenied
	}
	type answer struct { text string; err error }
	done := make(chan answer, 1)
	go func() {
		// 96 bytes is enough for REVIEW + 12-hex token; reject anything
		// larger instead of truncating and accepting a crafted prefix.
		reader := bufio.NewReaderSize(io.LimitReader(t.file, 97), 128)
		line, err := reader.ReadString('\n')
		if len(line) > 96 { err = ErrReviewDenied }
		done <- answer{text: strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), err: err}
	}()
	select {
	case <-ctx.Done():
		_ = t.Close() // interrupt local TTY read as best effort
		return "", ErrReviewDenied
	case response := <-done:
		if response.err != nil { return "", ErrReviewDenied }
		return response.text, nil
	}
}

// HostRoot is intentionally not injectable via CLI flags; only real root
// authority, distinct from membership in the monitoring Docker group.
func HostRoot() bool {
	return os.Geteuid() == 0
}

var _ Terminal = (*LocalTerminal)(nil)
