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

const defaultDirAddr = "localhost:8888"

func dirAddr(t *testing.T) string {
	t.Helper()

	if addr := os.Getenv("DIR_SERVER"); addr != "" {
		return addr
	}

	return defaultDirAddr
}

func dirctlBin(t *testing.T) string {
	t.Helper()

	if bin := os.Getenv("DIRCTL_BIN"); bin != "" {
		return bin
	}

	path, err := exec.LookPath("dirctl")
	if err != nil {
		t.Fatal("dirctl not found: set DIRCTL_BIN or add dirctl to PATH")
	}

	return path
}

func requireDaemon(t *testing.T) string {
	t.Helper()

	addr := dirAddr(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := dirclient.Connect(ctx, dirclient.Config{ServerAddress: addr})
	if err != nil {
		t.Fatalf("cannot connect to dir daemon at %s: %v", addr, err)
	}

	if err := c.Ping(ctx); err != nil {
		c.Close()
		t.Fatalf("dir daemon at %s not reachable: %v", addr, err)
	}

	c.Close()

	return addr
}

func TestDir_Ping(t *testing.T) {
	addr := requireDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := dirclient.Connect(ctx, dirclient.Config{ServerAddress: addr})
	if err != nil {
		t.Fatalf("Connect(%s): %v", addr, err)
	}
	defer c.Close()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestDir_Stream_NoFilters(t *testing.T) {
	addr := requireDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := dirclient.Connect(ctx, dirclient.Config{ServerAddress: addr})
	if err != nil {
		t.Fatalf("Connect(%s): %v", addr, err)
	}
	defer c.Close()

	var (
		firstPage []*dirclient.RecordSummary
		batches   [][]*dirclient.RecordSummary
		streamErr error
		done      = make(chan struct{})
	)

	go func() {
		c.Stream(ctx, nil, dirclient.StreamCallbacks{
			OnFirstPage: func(s []*dirclient.RecordSummary) { firstPage = s },
			OnBatch:     func(s []*dirclient.RecordSummary) { batches = append(batches, s) },
			OnDone:      func(err error) { streamErr = err; close(done) },
		})
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("stream timed out")
	}

	if streamErr != nil {
		t.Fatalf("Stream error: %v", streamErr)
	}

	total := len(firstPage)
	for _, b := range batches {
		total += len(b)
	}

	t.Logf("streamed %d records (first page: %d, batches: %d)", total, len(firstPage), len(batches))
}

func TestDir_PushAndPull(t *testing.T) {
	addr := requireDaemon(t)
	dirctl := dirctlBin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	record := `{
		"name": "e2e-test-agent",
		"version": "0.0.1-test",
		"schema_version": "1.0.0",
		"description": "lazydir e2e test record",
		"authors": ["lazydir-e2e"],
		"skills": [{"name": "natural_language_processing/analytical_reasoning"}],
		"created_at": "2025-01-01T00:00:00Z"
	}`

	cmd := exec.CommandContext(ctx, dirctl,
		"push", "--server-addr", addr, "--stdin", "--output", "raw")
	cmd.Stdin = strings.NewReader(record)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dirctl push failed: %v\noutput: %s", err, out)
	}

	cid := strings.TrimSpace(string(out))
	if cid == "" {
		t.Fatal("dirctl push returned empty CID")
	}

	t.Logf("pushed record, CID: %s", cid)

	c, err := dirclient.Connect(ctx, dirclient.Config{ServerAddress: addr})
	if err != nil {
		t.Fatalf("Connect(%s): %v", addr, err)
	}
	defer c.Close()

	jsonStr, err := c.PullJSON(ctx, cid)
	if err != nil {
		t.Fatalf("PullJSON(%s): %v", cid, err)
	}

	if jsonStr == "" || jsonStr == "{}" {
		t.Errorf("PullJSON returned empty record for CID %s", cid)
	}

	t.Logf("pulled record JSON (first 200 chars): %.200s", jsonStr)
}
