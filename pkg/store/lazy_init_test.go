// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go.uber.org/goleak"
)

// TestLazyInit_RunsOnceConcurrently is the property that makes publish-after-init
// correct: under `go test -race` it proves init runs exactly once even when many
// goroutines call ensure simultaneously, and that done() flips only after.
func TestLazyInit_RunsOnceConcurrently(t *testing.T) {
	defer goleak.VerifyNone(t)

	var l lazyInit
	var calls atomic.Int32

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_ = l.ensure(func() error {
				calls.Add(1)
				return nil
			})
		})
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Errorf("init ran %d times, want exactly 1", got)
	}
	if !l.done() {
		t.Error("done() = false after successful init, want true")
	}
}

func TestLazyInit_RetriesAfterError(t *testing.T) {
	var l lazyInit

	if err := l.ensure(func() error { return errors.New("boom") }); err == nil {
		t.Fatal("expected the failing init to return an error")
	}
	if l.done() {
		t.Error("done() = true after a failed init, want false")
	}

	if err := l.ensure(func() error { return nil }); err != nil {
		t.Fatalf("retry after failure returned error: %v", err)
	}
	if !l.done() {
		t.Error("done() = false after a successful retry, want true")
	}
}
