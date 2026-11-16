// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/localdaemon"
)

// Alternate ports keep this test away from a developer's own daemon on the
// default localhost:8888 and ~/.agntcy/dir.
const (
	localDaemonAPIAddr = "localhost:8858"
	localDaemonZotAddr = "localhost:5558"
	localDaemonP2PAddr = "/ip4/127.0.0.1/tcp/8859"
	localDaemonGWAddr  = "localhost:8857"
)

func TestLocalDaemon_StartStop(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "daemon.log")

	d, err := localdaemon.New(localdaemon.Options{
		DirctlPath: dirctlBin(t),
		DataDir:    t.TempDir(),
		LogPath:    logPath,
		Env: []string{
			"DIRECTORY_DAEMON_SERVER_LISTEN_ADDRESS=" + localDaemonAPIAddr,
			"DIRECTORY_DAEMON_SERVER_STORE_OCI_REGISTRY_ADDRESS=" + localDaemonZotAddr,
			"DIRECTORY_DAEMON_RECONCILER_LOCAL_REGISTRY_REGISTRY_ADDRESS=" + localDaemonZotAddr,
			"DIRECTORY_DAEMON_SERVER_ROUTING_LISTEN_ADDRESS=" + localDaemonP2PAddr,
			"DIRECTORY_DAEMON_SERVER_ROUTING_DIRECTORY_API_ADDRESS=" + localDaemonAPIAddr,
			"DIRECTORY_DAEMON_SERVER_ROUTING_BOOTSTRAP_PEERS=",
			"DIRECTORY_DAEMON_SERVER_HTTP_GATEWAY_LISTEN_ADDRESS=" + localDaemonGWAddr,
			"DIRECTORY_DAEMON_SERVER_HTTP_GATEWAY_PUBLIC_URL=http://" + localDaemonGWAddr,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = d.Stop(5 * time.Second) })

	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !d.Owned() {
		t.Error("Owned() = false after Start")
	}

	deadline := time.Now().Add(60 * time.Second)

	for {
		select {
		case err := <-d.Exited():
			t.Fatalf("daemon exited before becoming reachable: %v (log: %s)", err, d.LogPath())
		default:
		}

		if pingLocalDaemon() == nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("daemon not reachable at %s within 60s: %v (log: %s)", localDaemonAPIAddr, pingLocalDaemon(), d.LogPath())
		}

		time.Sleep(500 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	running, err := d.Running(ctx)
	if err != nil {
		t.Fatalf("Running: %v", err)
	}

	if !running {
		t.Error("Running() = false while daemon is reachable")
	}

	if err := d.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Stop may use its whole grace period, so check with a fresh timeout.
	afterCtx, afterCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer afterCancel()

	running, err = d.Running(afterCtx)
	if err != nil {
		t.Fatalf("Running after Stop: %v", err)
	}

	if running {
		t.Error("Running() = true after Stop")
	}

	if d.Owned() {
		t.Error("Owned() = true after Stop")
	}
}

func pingLocalDaemon() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, err := dirclient.Connect(ctx, dirclient.Config{ServerAddress: localDaemonAPIAddr, AuthMode: "insecure"})
	if err != nil {
		return err
	}
	defer c.Close()

	return c.Ping(ctx)
}
