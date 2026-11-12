// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agntcy/lazydir/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testLocalAddr        = "localhost:8888"
	testCustomDaemonAddr = "localhost:9999"
)

func TestLocalDaemonEligible(t *testing.T) {
	t.Parallel()

	def := config.DefaultLocalDaemonAddress
	tests := []struct {
		name       string
		mode       string
		addr       string
		authMode   string
		daemonAddr string
		want       bool
	}{
		{name: "ask localhost no auth", mode: config.LocalDaemonAsk, addr: testLocalAddr, daemonAddr: def, want: true},
		{name: "never", mode: config.LocalDaemonNever, addr: testLocalAddr, daemonAddr: def, want: false},
		{name: "tls", mode: config.LocalDaemonAsk, addr: testLocalAddr, authMode: "tls", daemonAddr: def, want: false},
		{name: "remote host", mode: config.LocalDaemonAsk, addr: "dir.example.com:443", authMode: "insecure", daemonAddr: def, want: false},
		{name: "other loopback port", mode: config.LocalDaemonAsk, addr: "localhost:8080", daemonAddr: def, want: false},
		{name: "127.0.0.1 default port", mode: config.LocalDaemonAsk, addr: "127.0.0.1:8888", daemonAddr: def, want: true},
		{name: "ipv6 loopback default port", mode: config.LocalDaemonAsk, addr: "[::1]:8888", daemonAddr: def, want: true},
		{name: "custom daemon address", mode: config.LocalDaemonAsk, addr: testCustomDaemonAddr, daemonAddr: testCustomDaemonAddr, want: true},
		{name: "custom daemon address other port", mode: config.LocalDaemonAsk, addr: testLocalAddr, daemonAddr: testCustomDaemonAddr, want: false},
		{name: "daemon address not loopback", mode: config.LocalDaemonAsk, addr: testLocalAddr, daemonAddr: "10.0.0.1:8888", want: false},
		{name: "no port", mode: config.LocalDaemonAsk, addr: "localhost", daemonAddr: def, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := localDaemonEligible(tt.mode, tt.addr, tt.authMode, tt.daemonAddr); got != tt.want {
				t.Errorf("localDaemonEligible(%q, %q, %q, %q) = %v, want %v", tt.mode, tt.addr, tt.authMode, tt.daemonAddr, got, tt.want)
			}
		})
	}
}

func TestDecideLocalDaemon(t *testing.T) {
	t.Parallel()

	unavailable := status.Error(codes.Unavailable, "down")
	deadline := status.Error(codes.DeadlineExceeded, "slow")
	local := daemonDecision{Mode: config.LocalDaemonAsk, Addr: testLocalAddr, AuthMode: "insecure", DaemonAddr: config.DefaultLocalDaemonAddress, PingErr: unavailable, HaveDirctl: true}

	with := func(f func(*daemonDecision)) daemonDecision {
		d := local
		f(&d)
		return d
	}

	tests := []struct {
		name string
		d    daemonDecision
		want daemonAction
	}{
		{name: "remote host", d: with(func(d *daemonDecision) { d.Addr = "dir.example.com:443" }), want: daemonNone},
		{name: "other loopback port", d: with(func(d *daemonDecision) { d.Addr = "localhost:8080" }), want: daemonNone},
		{name: "ping ok", d: with(func(d *daemonDecision) { d.PingErr = nil }), want: daemonNone},
		{name: "deadline exceeded", d: with(func(d *daemonDecision) { d.PingErr = deadline }), want: daemonNone},
		{name: "no dirctl", d: with(func(d *daemonDecision) { d.HaveDirctl = false }), want: daemonNone},
		{name: "already running", d: with(func(d *daemonDecision) { d.Running = true }), want: daemonNone},
		{name: "owned", d: with(func(d *daemonDecision) { d.Owned = true; d.Running = true }), want: daemonWaitOwned},
		{name: "ask", d: local, want: daemonAsk},
		{name: "auto", d: with(func(d *daemonDecision) { d.Mode = config.LocalDaemonAuto }), want: daemonAuto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := decideLocalDaemon(tt.d); got != tt.want {
				t.Errorf("decideLocalDaemon(%+v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

func TestAwaitDaemonStart_Ready(t *testing.T) {
	t.Parallel()

	exited := make(chan error, 1)
	ready := make(chan struct{})
	close(ready)
	got, err := awaitDaemonStart(exited, ready, 5*time.Second)
	if got != startReady || err != nil {
		t.Fatalf("awaitDaemonStart = (%v, %v), want (startReady, nil)", got, err)
	}
}

func TestAwaitDaemonStart_EarlyExit(t *testing.T) {
	t.Parallel()

	exitErr := errors.New("exit 1")
	exited := make(chan error, 1)
	exited <- exitErr
	ready := make(chan struct{})

	start := time.Now()
	got, err := awaitDaemonStart(exited, ready, 5*time.Second)
	if got != startExited || !errors.Is(err, exitErr) {
		t.Fatalf("awaitDaemonStart = (%v, %v), want (startExited, %v)", got, err, exitErr)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("awaitDaemonStart took %v, want well under the 5s timeout", elapsed)
	}
}

func TestAwaitDaemonStart_ExitedClosed(t *testing.T) {
	t.Parallel()

	// A second receiver of Exited() sees the closed channel (nil): that is
	// still an exit, never "still running".
	exited := make(chan error)
	close(exited)
	got, _ := awaitDaemonStart(exited, make(chan struct{}), 5*time.Second)
	if got != startExited {
		t.Fatalf("awaitDaemonStart = %v, want startExited", got)
	}
}

func TestAwaitDaemonStart_Timeout(t *testing.T) {
	t.Parallel()

	got, err := awaitDaemonStart(make(chan error), make(chan struct{}), 50*time.Millisecond)
	if got != startTimeout || err != nil {
		t.Fatalf("awaitDaemonStart = (%v, %v), want (startTimeout, nil)", got, err)
	}
}

func TestDaemonFailureReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome startOutcome
		err     error
		want    string
	}{
		{name: "exit error", outcome: startExited, err: errors.New("exit status 1"), want: "daemon exited: exit status 1"},
		{name: "exit nil", outcome: startExited, err: nil, want: "daemon exited"},
		{name: "timeout", outcome: startTimeout, want: "daemon did not become reachable within 30s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := daemonFailureReason(tt.outcome, tt.err); got != tt.want {
				t.Errorf("daemonFailureReason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkDaemonReady(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	ready := make(chan struct{})
	app.state.daemonStarting = true
	app.state.daemonReady = ready

	app.markDaemonReady()

	select {
	case <-ready:
	default:
		t.Fatal("daemonReady not closed")
	}
	if app.state.daemonStarting {
		t.Fatal("daemonStarting still true")
	}
	if app.state.daemonReady != nil {
		t.Fatal("daemonReady not cleared")
	}

	app.markDaemonReady() // must not panic
}

func TestDaemonStartTimeout_IgnoredAfterSwitch(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	ready := make(chan struct{})
	app.state.daemonStarting = true
	app.state.daemonReady = ready

	if !app.daemonStartStillPending(ready) {
		t.Fatal("pending start not recognised")
	}
	if app.daemonStartStillPending(make(chan struct{})) {
		t.Fatal("a different channel must not count as pending")
	}

	// What connectToDirectory does on a server switch.
	app.state.daemonStarting = false
	app.state.daemonReady = nil

	if app.daemonStartStillPending(ready) {
		t.Fatal("late timeout must be ignored after a switch")
	}
}

func TestArmDaemonStart(t *testing.T) {
	t.Parallel()

	app := &Gui{}
	exited := make(chan error) // never fires: the daemon keeps running
	ready := app.armDaemonStart(&fakeDaemon{exited: exited})

	if !app.state.daemonStarting {
		t.Fatal("daemonStarting not set")
	}
	if ready == nil || app.state.daemonReady != ready {
		t.Fatal("daemonReady not armed with the returned channel")
	}
	if !app.daemonStartStillPending(ready) {
		t.Fatal("armed start not pending")
	}

	// Re-arming (e.g. daemonWaitOwned after a switch back) replaces the
	// channel, so the earlier watcher's guard no longer matches.
	again := app.armDaemonStart(&fakeDaemon{exited: exited})
	if again == ready || app.daemonStartStillPending(ready) {
		t.Fatal("re-arm must install a fresh channel")
	}

	// Records landing closes the current channel, ending the start; its
	// watcher sees ready and exits without touching app.g. Close the
	// superseded channel too so the first watcher exits instead of waiting
	// out the 30s timeout (which would call the nil app.g).
	app.markDaemonReady()
	if app.state.daemonStarting {
		t.Fatal("daemonStarting still set after markDaemonReady")
	}
	close(ready)
}

// fakeDaemon satisfies localDaemon in tests and records Stop calls.
type fakeDaemon struct {
	exited chan error
	owned  bool
	stops  []time.Duration
}

func (*fakeDaemon) Running(context.Context) (bool, error) { return false, nil }
func (*fakeDaemon) Start() error                          { return nil }
func (f *fakeDaemon) Exited() <-chan error                { return f.exited }
func (f *fakeDaemon) Owned() bool                         { return f.owned }
func (f *fakeDaemon) Stop(d time.Duration) error {
	f.stops = append(f.stops, d)
	return nil
}
func (*fakeDaemon) LogPath() string { return "" }

var _ localDaemon = (*fakeDaemon)(nil)

func TestStopOwnedDaemon_Owned(t *testing.T) {
	t.Parallel()
	fd := &fakeDaemon{owned: true}
	app := &Gui{daemon: fd}
	app.stopOwnedDaemon()
	if len(fd.stops) != 1 || fd.stops[0] != daemonStopTimeout {
		t.Errorf("stops = %v, want one call with %v", fd.stops, daemonStopTimeout)
	}
}

func TestStopOwnedDaemon_NotOwned(t *testing.T) {
	t.Parallel()
	fd := &fakeDaemon{}
	app := &Gui{daemon: fd}
	app.stopOwnedDaemon()
	if len(fd.stops) != 0 {
		t.Errorf("Stop called %d times for a daemon we do not own", len(fd.stops))
	}
}

func TestStopOwnedDaemon_NilDaemon(t *testing.T) {
	t.Parallel()
	(&Gui{}).stopOwnedDaemon()
}
