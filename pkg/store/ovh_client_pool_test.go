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
	"testing"

	"github.com/ovh/go-ovh/ovh"
)

// countingClientFactory builds OVH clients and records how many were built.
func countingClientFactory() (func() (*ovh.Client, error), *int64) {
	var (
		mutex sync.Mutex
		built int64
	)
	return func() (*ovh.Client, error) {
		mutex.Lock()
		built++
		mutex.Unlock()
		return ovh.NewClient("ovh-eu", "application-key", "application-secret", "consumer-key")
	}, &built
}

func TestOVHClientPool_Acquire(t *testing.T) {
	t.Parallel()

	t.Run("reuses a released client instead of building a new one", func(t *testing.T) {
		t.Parallel()

		newClient, built := countingClientFactory()
		pool := newOVHClientPool(newClient)

		first, err := pool.acquire()
		if err != nil {
			t.Fatalf("acquire failed: %v", err)
		}
		pool.release(first)

		second, err := pool.acquire()
		if err != nil {
			t.Fatalf("acquire failed: %v", err)
		}
		if first != second {
			t.Error("expected the released client to be handed out again")
		}
		if *built != 1 {
			t.Errorf("built %d clients, want 1", *built)
		}
	})

	t.Run("hands a distinct client to every concurrent borrower", func(t *testing.T) {
		t.Parallel()

		newClient, built := countingClientFactory()
		pool := newOVHClientPool(newClient)

		// go-ovh writes to the embedded http.Client while building a request, so two
		// borrowers must never hold the same client at the same time
		const borrowers = 8
		held := make([]*ovh.Client, borrowers)
		for i := range borrowers {
			client, err := pool.acquire()
			if err != nil {
				t.Fatalf("acquire failed: %v", err)
			}
			held[i] = client
		}

		seen := map[*ovh.Client]bool{}
		for _, client := range held {
			if seen[client] {
				t.Fatal("the same client was handed out twice while still borrowed")
			}
			seen[client] = true
		}
		if *built != borrowers {
			t.Errorf("built %d clients, want %d", *built, borrowers)
		}

		// once returned, the pool stops growing
		for _, client := range held {
			pool.release(client)
		}
		for range borrowers {
			client, err := pool.acquire()
			if err != nil {
				t.Fatalf("acquire failed: %v", err)
			}
			if !seen[client] {
				t.Error("expected a pooled client")
			}
		}
		if *built != borrowers {
			t.Errorf("built %d clients after the release, want %d", *built, borrowers)
		}
	})

	t.Run("propagates the error of the factory", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("no credentials")
		pool := newOVHClientPool(func() (*ovh.Client, error) { return nil, wantErr })

		if _, err := pool.acquire(); !errors.Is(err, wantErr) {
			t.Errorf("acquire error = %v, want %v", err, wantErr)
		}
		if err := pool.get("/cloud/project", &[]string{}); !errors.Is(err, wantErr) {
			t.Errorf("get error = %v, want %v", err, wantErr)
		}
		if err := pool.post("/cloud/project", nil, &struct{}{}); !errors.Is(err, wantErr) {
			t.Errorf("post error = %v, want %v", err, wantErr)
		}
	})
}

// TestOVHClientPool_ConcurrentAcquireRelease exercises the pool from many goroutines so
// that `go test -race` fails loudly if the locking ever regresses.
func TestOVHClientPool_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()

	newClient, _ := countingClientFactory()
	pool := newOVHClientPool(newClient)

	wg := sync.WaitGroup{}
	for range 64 {
		wg.Go(func() {
			for range 20 {
				client, err := pool.acquire()
				if err != nil {
					t.Errorf("acquire failed: %v", err)
					return
				}
				if client == nil {
					t.Error("acquire returned no client")
					return
				}
				pool.release(client)
			}
		})
	}
	wg.Wait()
}
