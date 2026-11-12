// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agntcy/lazydir/internal/config"
	"github.com/agntcy/lazydir/internal/localdaemon"
	"github.com/jesseduffield/gocui"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// localDaemon is the part of *localdaemon.Daemon the GUI uses; tests use a
// fake.
type localDaemon interface {
	Running(ctx context.Context) (bool, error)
	Start() error
	Exited() <-chan error
	Owned() bool
	Stop(timeout time.Duration) error
	LogPath() string
}

var _ localDaemon = (*localdaemon.Daemon)(nil)

// daemonStartTimeout bounds how long a started daemon may take to answer.
const daemonStartTimeout = 30 * time.Second

// daemonStopTimeout bounds how long an owned daemon gets to stop gracefully
// before its process group is killed.
const daemonStopTimeout = 5 * time.Second

// stopOwnedDaemon stops the daemon lazydir started, if any. A daemon lazydir
// did not start is never touched.
func (app *Gui) stopOwnedDaemon() {
	if app.daemon == nil || !app.daemon.Owned() {
		return
	}
	_ = app.daemon.Stop(daemonStopTimeout)
}

// daemonAction is what connect does about a local directory that did not
// answer its ping.
type daemonAction int

const (
	daemonNone      daemonAction = iota // nothing to offer
	daemonAsk                           // offer to start a daemon
	daemonAuto                          // start a daemon without asking
	daemonWaitOwned                     // lazydir's own daemon is still coming up
)

// daemonDecision collects the inputs of decideLocalDaemon.
type daemonDecision struct {
	Mode, Addr, AuthMode string
	PingErr              error
	HaveDirctl           bool
	Running              bool
	Owned                bool
}

// newLocalDaemon builds the daemon manager, or returns nil when the feature is
// off or dirctl cannot be resolved.
func newLocalDaemon(cfg config.LocalDaemonConfig) localDaemon {
	if cfg.EffectiveMode() == config.LocalDaemonNever {
		return nil
	}
	d, err := localdaemon.New(localdaemon.Options{DirctlPath: cfg.DirctlPath, DataDir: cfg.DataDir})
	if err != nil {
		return nil // ErrDirctlNotFound or anything else: no daemon support
	}
	return d
}

// localDaemonEligible reports whether a server could be a local daemon that
// lazydir may start: the feature is on, the host is loopback and the
// connection is plain insecure gRPC.
func localDaemonEligible(mode, addr, authMode string) bool {
	if mode == config.LocalDaemonNever {
		return false
	}
	if !localdaemon.IsLoopback(addr) {
		return false
	}
	return authMode == "" || authMode == authModeInsecure
}

// decideLocalDaemon picks what to do after the connect ping.
func decideLocalDaemon(d daemonDecision) daemonAction {
	if !localDaemonEligible(d.Mode, d.Addr, d.AuthMode) || status.Code(d.PingErr) != codes.Unavailable {
		return daemonNone
	}
	if d.Owned {
		return daemonWaitOwned
	}
	if !d.HaveDirctl || d.Running {
		return daemonNone
	}
	if d.Mode == config.LocalDaemonAuto {
		return daemonAuto
	}
	return daemonAsk
}

// startOutcome is how waiting for a started daemon ended.
type startOutcome int

const (
	startReady   startOutcome = iota // the directory answered
	startExited                      // the daemon process exited
	startTimeout                     // neither happened in time
)

// awaitDaemonStart waits until ready is closed, the daemon exits or timeout
// passes. Any receive on exited counts as an exit: only one receiver gets the
// real error, others see the closed channel's nil.
func awaitDaemonStart(exited <-chan error, ready <-chan struct{}, timeout time.Duration) (startOutcome, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ready:
		return startReady, nil
	case err := <-exited:
		return startExited, err
	case <-timer.C:
		return startTimeout, nil
	}
}

// daemonFailureReason is the first line of the failure popup.
func daemonFailureReason(outcome startOutcome, err error) string {
	if outcome == startExited {
		if err == nil {
			return "daemon exited"
		}
		return "daemon exited: " + err.Error()
	}
	return fmt.Sprintf("daemon did not become reachable within %s", daemonStartTimeout)
}

// offerLocalDaemon asks whether to start a daemon for addr. Must be called on
// the GUI goroutine.
func (app *Gui) offerLocalDaemon(g *gocui.Gui, addr string) {
	body := fmt.Sprintf("Nothing is listening on %s. Start a local daemon? It stops when lazydir quits.", addr)
	app.openConfirmPopup(g, "No local directory", body, func() { app.startLocalDaemon(g) })
	showError := func() { app.openInfoPopup(g, viewDirectory) }
	if len(app.state.menu.options) > 1 {
		app.state.menu.options[1].action = showError
	}
	app.state.menu.onClose = showError
}

// startLocalDaemon launches the daemon and waits in the background for it to
// answer. Must be called on the GUI goroutine.
func (app *Gui) startLocalDaemon(g *gocui.Gui) {
	d := app.daemon
	if d == nil {
		return
	}
	if err := d.Start(); err != nil {
		app.showDaemonFailure(g, err.Error())
		return
	}
	app.armDaemonStart(d)
	app.renderDirectory(g)
}

// armDaemonStart enters the starting state for d with a fresh daemonReady
// channel and spawns the watcher that reports an early exit or a timeout.
// Shared by startLocalDaemon and connect's daemonWaitOwned branch so a
// daemon lazydir already owns cannot leave the starting state forever. An
// already exited daemon (closed Exited channel) fails immediately. Must be
// called on the GUI goroutine.
func (app *Gui) armDaemonStart(d localDaemon) chan struct{} {
	ready := make(chan struct{})
	app.state.daemonStarting = true
	app.state.daemonReady = ready

	exited := d.Exited()
	go func() {
		outcome, err := awaitDaemonStart(exited, ready, daemonStartTimeout)
		if outcome == startReady {
			return
		}
		app.g.Update(func(g *gocui.Gui) error {
			if !app.daemonStartStillPending(ready) {
				return nil // records loaded, or the user switched servers
			}
			app.state.daemonStarting = false
			app.state.daemonReady = nil
			app.renderDirectory(g)
			app.showDaemonFailure(g, daemonFailureReason(outcome, err))
			return nil
		})
	}()
	return ready
}

// daemonStartStillPending reports whether the start that owns ready is still
// the one being waited for.
func (app *Gui) daemonStartStillPending(ready chan struct{}) bool {
	return app.state.daemonStarting && app.state.daemonReady == ready
}

// markDaemonReady ends the starting state once the directory has answered.
// Must be called on the GUI goroutine.
func (app *Gui) markDaemonReady() {
	if !app.state.daemonStarting {
		return
	}
	if app.state.daemonReady != nil {
		close(app.state.daemonReady)
		app.state.daemonReady = nil
	}
	app.state.daemonStarting = false
}

// showDaemonFailure shows why the daemon did not come up, with the log tail.
func (app *Gui) showDaemonFailure(g *gocui.Gui, reason string) {
	var sb strings.Builder
	sb.WriteString(reason)
	if app.daemon != nil {
		path := app.daemon.LogPath()
		fmt.Fprintf(&sb, "\n\nLog: %s", path)
		if tail, err := localdaemon.TailLog(path, 10); err == nil && tail != "" {
			sb.WriteString("\n\n")
			sb.WriteString(strings.TrimRight(tail, "\n"))
		}
	}
	app.openConfirmPopup(g, "Local daemon failed", sb.String(), nil)
}

// daemonLogPath is the daemon log path, or "" without daemon support.
func (app *Gui) daemonLogPath() string {
	if app.daemon == nil {
		return ""
	}
	return app.daemon.LogPath()
}
