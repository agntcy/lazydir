// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package localdaemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeDirctl writes a fake dirctl shell script and returns its path. The pid
// file lives next to it as "pid", and every invocation's subcommand is
// appended to "calls". Its `start` runs in the foreground like the real
// daemon and exits on SIGTERM; with the "ignore-term" option it ignores
// SIGTERM so only a kill stops it.
func fakeDirctl(t *testing.T, opts ...string) string {
	t.Helper()
	dir := t.TempDir()
	trap := `trap 'kill "$SLEEP" 2>/dev/null; exit 0' TERM`
	for _, o := range opts {
		if o == "ignore-term" {
			trap = `trap '' TERM`
		}
	}
	script := `#!/bin/sh
DIR="$(dirname "$0")"
PIDFILE="$DIR/pid"
echo "$2" >> "$DIR/calls"
[ "$1" = daemon ] || exit 2
case "$2" in
start)
  if [ "$FAKE_START_FAIL" = 1 ]; then echo boom; exit 1; fi
  ` + trap + `
  echo $$ > "$PIDFILE"
  echo starting
  sleep 60 &
  SLEEP=$!
  wait "$SLEEP"
  ;;
status)
  if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
    echo "Daemon is running"
  else
    echo "Daemon is not running"
  fi
  ;;
esac
exit 0
`
	p := filepath.Join(dir, "dirctl")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newFake(t *testing.T, env []string, opts ...string) *Daemon {
	t.Helper()
	bin := fakeDirctl(t, opts...)
	d, err := New(Options{
		DirctlPath: bin,
		LogPath:    filepath.Join(t.TempDir(), "sub", "daemon.log"),
		Env:        env,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "pid"))
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				_ = syscall.Kill(-pid, syscall.SIGKILL) // the fake and its sleep
			}
		}
	})
	return d
}

func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"localhost:8888":      true,
		"LOCALHOST:1":         true,
		"127.0.0.1:8888":      true,
		"[::1]:8888":          true,
		"localhost":           true,
		"127.0.0.2:8888":      true,
		"dir.example.com:443": false,
		"10.0.0.1:8888":       false,
		"":                    false,
	}
	for addr, want := range cases {
		if got := IsLoopback(addr); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestNew_DirctlNotFound(t *testing.T) {
	_, err := New(Options{DirctlPath: "/nonexistent/dirctl"})
	if !errors.Is(err, ErrDirctlNotFound) {
		t.Fatalf("err = %v, want ErrDirctlNotFound", err)
	}
}

func TestDaemon_StartRunningStop(t *testing.T) {
	d := newFake(t, nil)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if !d.Owned() {
		t.Fatal("expected owned")
	}
	if !waitFor(t, func() bool {
		ok, _ := d.Running(context.Background())
		return ok
	}) {
		t.Fatal("daemon never reported running")
	}
	if err := d.Stop(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if d.Owned() {
		t.Error("still owned after Stop")
	}
	if ok, _ := d.Running(context.Background()); ok {
		t.Error("still running after Stop")
	}
	out, err := TailLog(d.LogPath(), 10)
	if err != nil || !strings.Contains(out, "starting") {
		t.Errorf("log = %q, err = %v", out, err)
	}
}

func TestDaemon_OwnedFalseAfterExit(t *testing.T) {
	d := newFake(t, nil)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(filepath.Dir(d.bin), "pid")
	if !waitFor(t, func() bool { _, err := os.Stat(pidFile); return err == nil }) {
		t.Fatal("no pid file")
	}
	b, _ := os.ReadFile(pidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	_ = syscall.Kill(pid, syscall.SIGKILL)
	select {
	case <-d.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("Exited did not yield")
	}
	if !waitFor(t, func() bool { return !d.Owned() }) {
		t.Error("still owned after exit")
	}
	if err := d.Stop(time.Second); err != nil {
		t.Errorf("Stop = %v", err)
	}
}

func TestDaemon_StartFailWritesLog(t *testing.T) {
	d := newFake(t, []string{"FAKE_START_FAIL=1"})
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-d.Exited():
		if err == nil {
			t.Fatal("expected non-nil exit error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Exited did not yield")
	}
	out, err := TailLog(d.LogPath(), 10)
	if err != nil || !strings.Contains(out, "boom") {
		t.Errorf("log = %q, err = %v", out, err)
	}
}

// waitPidFile waits until the fake has written its pid file, i.e. has
// installed its TERM trap.
func waitPidFile(t *testing.T, d *Daemon) {
	t.Helper()
	pidFile := filepath.Join(filepath.Dir(d.bin), "pid")
	if !waitFor(t, func() bool { _, err := os.Stat(pidFile); return err == nil }) {
		t.Fatal("no pid file")
	}
}

func TestDaemon_StopTerminatesChildWithoutStopCommand(t *testing.T) {
	d := newFake(t, nil)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	ex := d.Exited()
	waitPidFile(t, d)

	start := time.Now()
	if err := d.Stop(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Stop took %v, want well under the 5s timeout", elapsed)
	}
	select {
	case <-ex: // Stop consumed the exit; the channel is closed
	case <-time.After(time.Second):
		t.Fatal("child still running after Stop")
	}
	if d.Owned() {
		t.Error("still owned after Stop")
	}
	calls, err := os.ReadFile(filepath.Join(filepath.Dir(d.bin), "calls"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		if c == "stop" {
			t.Fatalf("Stop invoked `dirctl daemon stop`; calls = %q", calls)
		}
	}
}

func TestDaemon_StopNotOwnedIsNoop(t *testing.T) {
	d := newFake(t, nil)
	if err := d.Stop(time.Second); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(d.bin), "calls")); err == nil {
		t.Error("Stop on an unowned daemon ran dirctl")
	}
}

func TestDaemon_StopKillsAfterTimeout(t *testing.T) {
	d := newFake(t, nil, "ignore-term")
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	ex := d.Exited()
	waitPidFile(t, d)
	start := time.Now()
	_ = d.Stop(200 * time.Millisecond)
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Errorf("Stop returned after %v: the TERM-ignoring fake should need the kill", elapsed)
	}
	select {
	case <-ex:
	case <-time.After(2 * time.Second):
		t.Fatal("process not killed")
	}
}

func TestDaemon_StartTwiceFails(t *testing.T) {
	d := newFake(t, nil)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if err := d.Start(); err == nil {
		t.Error("second Start should fail")
	}
	_ = d.Stop(5 * time.Second)
}

func TestTailLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	var sb strings.Builder
	for i := 1; i <= 15; i++ {
		sb.WriteString("line" + strconv.Itoa(i) + "\n")
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := TailLog(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 10 || lines[0] != "line6" || lines[9] != "line15" {
		t.Errorf("got %q", got)
	}
}
