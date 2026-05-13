// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
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

func seedRecord(t *testing.T, ctx context.Context, addr, dirctl string) (string, string) {
	t.Helper()

	name := fmt.Sprintf("e2e-test-agent-%d", time.Now().UnixNano())
	record := map[string]any{
		"name":           name,
		"version":        "0.0.1-test",
		"schema_version": "1.0.0",
		"description":    "lazydir e2e test record",
		"authors":        []string{"lazydir-e2e"},
		"skills": []map[string]string{
			{"name": "natural_language_processing/analytical_reasoning"},
		},
		"created_at": "2025-01-01T00:00:00Z",
	}

	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json marshal record: %v", err)
	}

	cmd := exec.CommandContext(ctx, dirctl,
		"push", "--server-addr", addr, "--stdin", "--output", "raw")
	cmd.Stdin = strings.NewReader(string(payload))

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dirctl push failed: %v\noutput: %s", err, out)
	}

	cid := strings.TrimSpace(string(out))
	if cid == "" {
		t.Fatal("dirctl push returned empty CID")
	}

	return cid, name
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
	dirctl := dirctlBin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cid, _ := seedRecord(t, ctx, addr, dirctl)

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
	containsSeedCID := false
	for _, r := range firstPage {
		if r.CID == cid {
			containsSeedCID = true
			break
		}
	}
	for _, b := range batches {
		total += len(b)
		if containsSeedCID {
			continue
		}
		for _, r := range b {
			if r.CID == cid {
				containsSeedCID = true
				break
			}
		}
	}

	if total == 0 {
		t.Fatal("expected at least one streamed record")
	}
	if !containsSeedCID {
		t.Fatalf("seeded record CID %s not found in stream", cid)
	}

	t.Logf("streamed %d records (first page: %d, batches: %d)", total, len(firstPage), len(batches))
}

func TestDir_PushAndPull(t *testing.T) {
	addr := requireDaemon(t)
	dirctl := dirctlBin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cid, _ := seedRecord(t, ctx, addr, dirctl)

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
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		t.Fatalf("PullJSON(%s) returned invalid JSON: %v", cid, err)
	}
	if _, ok := payload["name"]; !ok {
		t.Fatalf("PullJSON(%s) missing expected name field", cid)
	}

	t.Logf("pulled record JSON (first 200 chars): %.200s", jsonStr)
}
