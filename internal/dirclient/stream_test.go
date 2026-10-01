// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package dirclient

import (
	"errors"
	"testing"
	"time"
)

// fakeStream mirrors the dir client's stream result: unbuffered result and
// error channels that are never closed, and a done channel closed when the
// producer exits.
type fakeStream struct {
	resCh  chan *string
	errCh  chan error
	doneCh chan struct{}
}

func (f *fakeStream) ResCh() <-chan *string   { return f.resCh }
func (f *fakeStream) ErrCh() <-chan error     { return f.errCh }
func (f *fakeStream) DoneCh() <-chan struct{} { return f.doneCh }

func TestDrainStream_UnblocksProducer(t *testing.T) {
	t.Parallel()

	f := &fakeStream{resCh: make(chan *string), errCh: make(chan error), doneCh: make(chan struct{})}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer close(f.doneCh)
		for range 3 {
			s := "rec"
			f.resCh <- &s // blocks forever unless someone drains
		}
		f.errCh <- errors.New("context canceled")
	}()

	drainStream[string](f)

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("producer still blocked: drainStream did not consume the stream")
	}
}
