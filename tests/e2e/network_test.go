// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/agntcy/lazydir/internal/dirclient"
)

// networkSearchLimit is generous so other peers announcing the same skill
// cannot push the seeded record out of the results.
const networkSearchLimit = 1000

// requirePeerDaemon returns the address of the second, peered daemon, or
// skips the test when DIR_PEER_SERVER is unset.
func requirePeerDaemon(t *testing.T) string {
	t.Helper()

	addr := os.Getenv("DIR_PEER_SERVER")
	if addr == "" {
		t.Skip("DIR_PEER_SERVER not set: run task test:e2e:network")
	}

	return addr
}

// poll calls check every second until it reports done or the deadline passes.
func poll(t *testing.T, timeout time.Duration, what string, check func() (bool, error)) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		done, err := check()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s (last error: %v)", timeout, what, err)
		}
		time.Sleep(time.Second)
	}
}

func TestNetwork_SearchPreviewSync(t *testing.T) {
	peerAddr := requirePeerDaemon(t)
	addr := requireDaemon(t)
	dirctl := dirctlBin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	cid, name := seedRecord(t, ctx, peerAddr, dirctl)
	out, err := exec.CommandContext(ctx, dirctl,
		"routing", "publish", cid, "--server-addr", peerAddr).CombinedOutput()
	if err != nil {
		t.Fatalf("dirctl routing publish failed: %v\noutput: %s", err, out)
	}
	t.Logf("published %s on peer %s", cid, peerAddr)

	c, err := dirclient.Connect(ctx, dirclient.Config{ServerAddress: addr, AuthMode: "insecure"})
	if err != nil {
		t.Fatalf("Connect(%s): %v", addr, err)
	}
	defer c.Close()

	queries := []dirclient.NetworkQuery{{
		Category: dirclient.FilterSkill,
		Value:    seedSkill,
	}}

	var found dirclient.NetworkResult
	poll(t, 60*time.Second, "routing search to return "+cid, func() (bool, error) {
		results, err := c.SearchNetwork(ctx, queries, 1, networkSearchLimit)
		for _, r := range results {
			if r.CID == cid {
				found = r
				return true, nil
			}
		}
		return false, err
	})
	if found.DirAddr != peerAddr {
		t.Fatalf("DirAddr = %q, want %q", found.DirAddr, peerAddr)
	}

	peer, err := dirclient.ConnectPeer(dirclient.Config{ServerAddress: addr, AuthMode: "insecure"}, found.DirAddr)
	if err != nil {
		t.Fatalf("ConnectPeer(%s): %v", found.DirAddr, err)
	}
	defer peer.Close()

	jsonStr, err := peer.PullJSON(ctx, cid)
	if err != nil {
		t.Fatalf("peer PullJSON(%s): %v", cid, err)
	}
	if !strings.Contains(jsonStr, name) {
		t.Fatalf("peer PullJSON(%s) missing record name %q", cid, name)
	}

	local, err := c.LocalCIDs(ctx, queries)
	if err != nil {
		t.Fatalf("LocalCIDs before sync: %v", err)
	}
	if local[cid] {
		t.Fatalf("LocalCIDs reports %s present before sync", cid)
	}

	syncID, err := c.CreateSync(ctx, "http://"+found.DirAddr, []string{cid})
	if err != nil {
		t.Fatalf("CreateSync: %v", err)
	}
	t.Logf("created sync %s", syncID)

	poll(t, 90*time.Second, "sync "+syncID+" to complete", func() (bool, error) {
		info, err := c.GetSyncInfo(ctx, syncID)
		if err != nil {
			return false, err
		}
		if info.Status == dirclient.SyncFailed {
			t.Fatalf("sync %s failed", syncID)
		}
		return info.Status == dirclient.SyncCompleted, nil
	})

	poll(t, 90*time.Second, cid+" to be indexed locally", func() (bool, error) {
		local, err := c.LocalCIDs(ctx, queries)
		return local[cid], err
	})
}
