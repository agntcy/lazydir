// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

// Package localdaemon wraps `dirctl daemon start|status` and owns the child
// process lazydir starts. Stopping signals that child directly.
package localdaemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// ErrDirctlNotFound is returned when the dirctl binary cannot be resolved.
var ErrDirctlNotFound = errors.New("dirctl not found")

// Options configures a Daemon.
type Options struct {
	DirctlPath string
	DataDir    string
	LogPath    string
	// Env is appended to os.Environ() for `start`.
	Env []string
}

// Daemon manages a dirctl daemon process.
type Daemon struct {
	bin     string
	dataDir string
	logPath string
	env     []string

	mu     sync.Mutex
	cmd    *exec.Cmd
	exited chan error
	owned  bool
}

// New resolves dirctl and returns a Daemon.
func New(o Options) (*Daemon, error) {
	bin := o.DirctlPath
	if bin == "" {
		bin = "dirctl"
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrDirctlNotFound, bin, err)
	}
	logPath := o.LogPath
	if logPath == "" {
		logPath = DefaultLogPath()
	}
	return &Daemon{bin: resolved, dataDir: o.DataDir, logPath: logPath, env: o.Env}, nil
}

func (d *Daemon) args(sub string) []string {
	a := []string{"daemon", sub}
	if d.dataDir != "" {
		a = append(a, "--data-dir", d.dataDir)
	}
	return a
}

// Running reports whether a daemon is running, whoever started it.
func (d *Daemon) Running(ctx context.Context) (bool, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, d.bin, d.args("status")...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return false, err
	}
	return strings.Contains(out.String(), "is running"), nil
}

// Start launches `dirctl daemon start` as an owned child process.
func (d *Daemon) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.owned {
		return errors.New("daemon already started by lazydir")
	}
	if err := os.MkdirAll(filepath.Dir(d.logPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(d.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(d.bin, d.args("start")...)
	cmd.Env = append(os.Environ(), d.env...)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = sysProcAttr()
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close() // the child holds its own descriptor
	ch := make(chan error, 1)
	d.cmd, d.exited, d.owned = cmd, ch, true
	go func() {
		err := cmd.Wait()
		d.mu.Lock()
		if d.cmd == cmd {
			d.owned = false
		}
		d.mu.Unlock()
		ch <- err
		close(ch)
	}()
	return nil
}

// Exited returns the exit channel of the current owned process, or nil.
func (d *Daemon) Exited() <-chan error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.exited == nil {
		return nil
	}
	return d.exited
}

// Owned is true from a successful Start until the process exits.
func (d *Daemon) Owned() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.owned
}

// Stop stops a daemon lazydir started: it asks the owned child process to
// terminate (SIGTERM on Unix, a hard kill on Windows), waits up to timeout
// for it to exit, then kills its process group and waits up to one more
// second. It never runs `dirctl daemon stop`: that acts on whatever pid
// dirctl's pid file names, which is not written until the daemon is ready
// and may belong to a daemon lazydir did not start. A daemon lazydir does
// not own is never touched.
func (d *Daemon) Stop(timeout time.Duration) error {
	d.mu.Lock()
	owned, cmd, ch := d.owned, d.cmd, d.exited
	d.mu.Unlock()
	if !owned || cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = terminate(cmd.Process)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
		return nil
	case <-timer.C:
	}
	_ = killGroup(cmd.Process)
	select {
	case <-ch:
		return nil
	case <-time.After(time.Second):
		return errors.New("daemon did not exit after kill")
	}
}

// LogPath returns the daemon log file path.
func (d *Daemon) LogPath() string { return d.logPath }

// IsLoopback reports whether addr (host or host:port) points at this machine.
func IsLoopback(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		h = addr
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// DefaultLogPath is <user cache dir>/lazydir/daemon.log.
func DefaultLogPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "lazydir", "daemon.log")
}

// tailChunk is how much TailLog reads per step from the end of the file.
const tailChunk = 4096

// TailLog returns the last n lines of the file at path. It reads backwards
// from the end in chunks, so a large log is never loaded whole.
func TailLog(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if n <= 0 {
		return "", nil
	}
	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	var buf []byte
	for off := info.Size(); off > 0; {
		size := min(int64(tailChunk), off)
		off -= size
		chunk := make([]byte, size)
		if _, err := f.ReadAt(chunk, off); err != nil {
			return "", err
		}
		buf = slices.Concat(chunk, buf)
		// n lines need n newlines after trimming the trailing one, unless
		// the start of the file is reached first.
		if bytes.Count(bytes.TrimRight(buf, "\n"), []byte("\n")) >= n {
			break
		}
	}

	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}
