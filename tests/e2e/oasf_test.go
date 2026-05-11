// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/agntcy/lazydir/internal/oasf"
)

func oasfServerAddr(t *testing.T) string {
	t.Helper()

	if addr := os.Getenv("OASF_SERVER"); addr != "" {
		return addr
	}

	return oasf.DefaultServerAddress
}

func TestOASF_Ping(t *testing.T) {
	addr := oasfServerAddr(t)

	c, err := oasf.NewClient(oasf.Config{ServerAddress: addr, Timeout: 15})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping(%s): %v", addr, err)
	}
}

func TestOASF_FetchAll_Skills(t *testing.T) {
	addr := oasfServerAddr(t)

	c, err := oasf.NewClient(oasf.Config{ServerAddress: addr, Timeout: 15})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entries, err := c.FetchAll(ctx, oasf.ClassTypeSkill, "")
	if err != nil {
		t.Fatalf("FetchAll(skills): %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least one skill entry, got 0")
	}

	for name, e := range entries {
		if e.Name == "" || e.ID == 0 {
			t.Errorf("skill %q has invalid entry: ID=%d Name=%q", name, e.ID, e.Name)
		}
	}

	t.Logf("fetched %d skill entries from %s", len(entries), addr)
}

func TestOASF_FetchAll_Domains(t *testing.T) {
	addr := oasfServerAddr(t)

	c, err := oasf.NewClient(oasf.Config{ServerAddress: addr, Timeout: 15})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entries, err := c.FetchAll(ctx, oasf.ClassTypeDomain, "")
	if err != nil {
		t.Fatalf("FetchAll(domains): %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least one domain entry, got 0")
	}

	t.Logf("fetched %d domain entries from %s", len(entries), addr)
}

func TestOASF_FetchAll_Modules(t *testing.T) {
	addr := oasfServerAddr(t)

	c, err := oasf.NewClient(oasf.Config{ServerAddress: addr, Timeout: 15})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entries, err := c.FetchAll(ctx, oasf.ClassTypeModule, "")
	if err != nil {
		t.Fatalf("FetchAll(modules): %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least one module entry, got 0")
	}

	t.Logf("fetched %d module entries from %s", len(entries), addr)
}

func TestOASF_Fetch_KnownSkill(t *testing.T) {
	addr := oasfServerAddr(t)

	c, err := oasf.NewClient(oasf.Config{ServerAddress: addr, Timeout: 15})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch the full skill list first and pick one to test Fetch().
	all, err := c.FetchAll(ctx, oasf.ClassTypeSkill, "")
	if err != nil {
		t.Fatalf("FetchAll(skills): %v", err)
	}

	if len(all) == 0 {
		t.Skip("no skills available to test Fetch()")
	}

	var firstName string
	for name := range all {
		firstName = name

		break
	}

	info, err := c.Fetch(ctx, oasf.ClassTypeSkill, firstName, "")
	if err != nil {
		t.Fatalf("Fetch(%q): %v", firstName, err)
	}

	if info.Name != firstName {
		t.Errorf("Fetch(%q).Name = %q", firstName, info.Name)
	}

	if info.ID == 0 {
		t.Errorf("Fetch(%q).ID = 0", firstName)
	}

	t.Logf("Fetch(%q): ID=%d Caption=%q Type=%s Ancestors=%d",
		firstName, info.ID, info.Caption, info.Type, len(info.Ancestors))
}
