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

import "sync"

// clusterCache is a small concurrency-safe map used by the cloud stores to
// remember clusters discovered during the search, so GetKubeconfigForPath and
// the preview can avoid a second API round-trip.
//
// It must be concurrency-safe: StartSearch may still be populating it (possibly
// from goroutines) while the TUI computes a preview or fetches the kubeconfig of
// a selected cluster. A plain map accessed from these paths is a data race and
// crashes the process — see the Azure store, which this type generalises.
type clusterCache[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func newClusterCache[K comparable, V any]() *clusterCache[K, V] {
	return &clusterCache[K, V]{m: make(map[K]V)}
}

// Get returns the cached value for key and whether it was present.
func (c *clusterCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[key]
	return v, ok
}

// Set stores value under key.
func (c *clusterCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = value
}

// Values returns a snapshot copy of the cached values, safe to range over
// without holding the lock. Used by stores that look a cluster up by a
// non-key attribute (e.g. its name) as a fallback.
func (c *clusterCache[K, V]) Values() []V {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]V, 0, len(c.m))
	for _, v := range c.m {
		out = append(out, v)
	}
	return out
}

// Keys returns a snapshot copy of the cached keys, safe to range over without
// holding the lock.
func (c *clusterCache[K, V]) Keys() []K {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]K, 0, len(c.m))
	for k := range c.m {
		out = append(out, k)
	}
	return out
}

// Reset empties the cache.
func (c *clusterCache[K, V]) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = make(map[K]V)
}
