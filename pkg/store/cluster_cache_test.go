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
	"strconv"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestClusterCache_GetSet(t *testing.T) {
	c := newClusterCache[string, int]()

	if _, ok := c.Get("missing"); ok {
		t.Error("expected a miss for an absent key")
	}

	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Errorf("Get(a) = %d, %v; want 1, true", v, ok)
	}
}

// TestClusterCache_ConcurrentAccess exercises the cache from many goroutines so
// that `go test -race` fails loudly if the locking ever regresses. This mirrors
// the real usage: StartSearch writing while previews/fetches read.
func TestClusterCache_ConcurrentAccess(t *testing.T) {
	defer goleak.VerifyNone(t)

	c := newClusterCache[string, int]()
	const n = 200

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.Set(strconv.Itoa(i), i)
		}(i)
		go func(i int) {
			defer wg.Done()
			_, _ = c.Get(strconv.Itoa(i))
			_ = c.Values()
		}(i)
	}
	wg.Wait()

	if got := len(c.Values()); got != n {
		t.Errorf("Values() length = %d, want %d", got, n)
	}
}
