// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndexSnapshot_SharesOneRead(t *testing.T) {
	var s indexSnapshot
	var reads atomic.Int32
	release := make(chan struct{})
	fetch := func(context.Context) ([]string, error) {
		reads.Add(1)
		<-release
		return []string{"a"}, nil
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			cids, err := s.get(context.Background(), "k", time.Minute, fetch)
			if err != nil || !cids["a"] {
				t.Errorf("get = %v, %v", cids, err)
			}
		})
	}
	time.Sleep(20 * time.Millisecond) // let the callers queue behind the read
	close(release)
	wg.Wait()

	if n := reads.Load(); n != 1 {
		t.Errorf("reads = %d, want 1 shared by all callers", n)
	}
}

func TestIndexSnapshot_RefreshesStaleAndOtherKey(t *testing.T) {
	var s indexSnapshot
	reads := 0
	fetch := func(context.Context) ([]string, error) {
		reads++
		return []string{"a"}, nil
	}
	ctx := context.Background()

	_, _ = s.get(ctx, "k1", time.Minute, fetch)
	_, _ = s.get(ctx, "k1", time.Minute, fetch) // fresh: cached
	_, _ = s.get(ctx, "k2", time.Minute, fetch) // other server: read
	_, _ = s.get(ctx, "k2", 0, fetch)           // stale: read
	if reads != 3 {
		t.Errorf("reads = %d, want 3", reads)
	}
}

func TestIndexSnapshot_ErrorNotCached(t *testing.T) {
	var s indexSnapshot
	boom := errors.New("index unavailable")
	reads := 0
	fetch := func(context.Context) ([]string, error) {
		reads++
		if reads == 1 {
			return nil, boom
		}
		return []string{"a"}, nil
	}
	if _, err := s.get(context.Background(), "k", time.Minute, fetch); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if cids, err := s.get(context.Background(), "k", time.Minute, fetch); err != nil || !cids["a"] {
		t.Errorf("retry = %v, %v; want a fresh read", cids, err)
	}
}
