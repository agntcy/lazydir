// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"sync"
	"time"
)

// indexSnapshot shares one view of the CIDs this node has indexed among the
// network syncs waiting for their records, so concurrent peer batches read
// the full index once per poll instead of once each. The zero value is ready
// to use and safe for concurrent use.
type indexSnapshot struct {
	mu       sync.Mutex
	key      any             // what the snapshot was read from, e.g. the client
	at       time.Time       // when it was read
	cids     map[string]bool // nil until a read succeeds
	inflight chan struct{}   // closed when the read in flight ends
}

// get returns the indexed CIDs of key, reading them with fetch unless a
// snapshot of key younger than maxAge exists. Callers arriving while a read
// is in flight wait for it instead of starting their own.
func (s *indexSnapshot) get(ctx context.Context, key any, maxAge time.Duration, fetch func(context.Context) ([]string, error)) (map[string]bool, error) {
	for {
		s.mu.Lock()
		if s.key == key && s.cids != nil && time.Since(s.at) < maxAge {
			cids := s.cids
			s.mu.Unlock()
			return cids, nil
		}
		if ch := s.inflight; ch != nil {
			s.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ch := make(chan struct{})
		s.inflight = ch
		s.mu.Unlock()

		list, err := fetch(ctx)
		var cids map[string]bool
		if err == nil {
			cids = make(map[string]bool, len(list))
			for _, c := range list {
				cids[c] = true
			}
		}

		s.mu.Lock()
		s.inflight = nil
		if err == nil {
			s.key, s.at, s.cids = key, time.Now(), cids
		}
		s.mu.Unlock()
		close(ch)
		return cids, err
	}
}
