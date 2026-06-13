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
	"sync"
	"sync/atomic"
)

// lazyInit serialises a fallible, one-time initialisation and publishes its
// completion safely. Stores that connect to a backing API lazily embed it so
// that several callers (StartSearch, GetKubeconfigForPath, a preview) can race
// to initialise without:
//   - running the (expensive, mutating) initialisation more than once, and
//   - exposing half-populated fields: done() only reports true once init has
//     fully and successfully completed, and the atomic flag establishes the
//     happens-before needed for the populated fields to be safely read.
//
// On failure the initialisation is retried on the next call (unlike sync.Once).
// The zero value is ready to use.
type lazyInit struct {
	mu    sync.Mutex
	ready atomic.Bool
}

// ensure runs init exactly once successfully. Concurrent callers block until
// the in-flight initialisation finishes; callers after success return nil
// without re-running it.
func (l *lazyInit) ensure(init func() error) error {
	if l.ready.Load() {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// re-check under the lock: another caller may have initialised meanwhile
	if l.ready.Load() {
		return nil
	}

	if err := init(); err != nil {
		return err
	}

	l.ready.Store(true)
	return nil
}

// done reports whether initialisation has fully completed.
func (l *lazyInit) done() bool {
	return l.ready.Load()
}
