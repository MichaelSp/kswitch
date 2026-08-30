// Copyright 2024 The Kswitch authors
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

package pkg

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"go.uber.org/goleak"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// searchStubStore is a KubeconfigStore that reports a fixed set of paths and counts how
// often its kubeconfigs are retrieved.
type searchStubStore struct {
	id    string
	kind  types.StoreKind
	paths []string
	// declared is what ContextNamesForPath returns; a nil map means the store does not
	// implement storetypes.ContextNamer at all, see contextNamerStore
	declared map[string][]string
	// noPrefix makes GetContextPrefix return "", like the stores that do not
	// prefix their context names
	noPrefix bool
	// maxConcurrent is surfaced through GetStoreConfig
	maxConcurrent *int

	mutex sync.Mutex
	// fetched records the paths whose kubeconfig has been downloaded
	fetched []string
}

func (s *searchStubStore) GetID() string                { return s.id }
func (s *searchStubStore) GetKind() types.StoreKind     { return s.kind }
func (s *searchStubStore) VerifyKubeconfigPaths() error { return nil }
func (s *searchStubStore) GetLogger() *logrus.Entry     { return logrus.WithField("store", s.id) }

func (s *searchStubStore) GetContextPrefix(_ string) string {
	if s.noPrefix {
		return ""
	}
	return s.id
}

func (s *searchStubStore) GetStoreConfig() types.KubeconfigStore {
	id := s.id
	return types.KubeconfigStore{Kind: s.kind, ID: &id, MaxConcurrentKubeconfigRequests: s.maxConcurrent}
}

func (s *searchStubStore) StartSearch(channel chan storetypes.SearchResult) {
	for _, path := range s.paths {
		channel <- storetypes.SearchResult{KubeconfigPath: path}
	}
}

func (s *searchStubStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	s.mutex.Lock()
	s.fetched = append(s.fetched, path)
	s.mutex.Unlock()

	return []byte(`apiVersion: v1
kind: Config
current-context: ctx-` + path + `
contexts:
- name: ctx-` + path + `
  context:
    cluster: ` + path + `
clusters:
- name: ` + path + `
  cluster:
    server: https://` + path + `.example.invalid
`), nil
}

func (s *searchStubStore) fetchedPaths() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	fetched := slices.Clone(s.fetched)
	slices.Sort(fetched)
	return fetched
}

// contextNamerStore is a searchStubStore that can name its contexts without downloading
// the kubeconfig, like the OVH store does.
type contextNamerStore struct {
	*searchStubStore
}

func (s *contextNamerStore) ContextNamesForPath(path string, _ map[string]string) []string {
	return s.declared[path]
}

// collect drains the search channel into the discovered context names and any errors.
//
// doSearch swaps the process-wide os.Stdout for the duration of the search, so its
// callers cannot use t.Parallel(): concurrent searches race on that global.
func collect(t *testing.T, stores []storetypes.KubeconfigStore) ([]string, []error) {
	t.Helper()

	channel, err := doSearch(stores, &types.Config{}, t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("doSearch failed: %v", err)
	}

	var (
		names  []string
		errors []error
	)
	for result := range *channel {
		if result.Error != nil {
			errors = append(errors, result.Error)
			continue
		}
		names = append(names, result.Name)
	}
	slices.Sort(names)
	return names, errors
}

func TestDoSearch_ContextNamer(t *testing.T) {
	tests := []struct {
		name string
		// declared maps a path to the context names the store claims for it
		declared    map[string][]string
		noPrefix    bool
		wantNames   []string
		wantFetched []string
	}{
		{
			// the whole point of the interface: no kubeconfig is downloaded during the
			// search, which for the cloud stores is a remote call per cluster
			name: "declared names skip the kubeconfig download",
			declared: map[string][]string{
				"alpha": {"kubernetes-admin@alpha"},
				"beta":  {"kubernetes-admin@beta"},
			},
			wantNames:   []string{"namer/kubernetes-admin@alpha", "namer/kubernetes-admin@beta"},
			wantFetched: nil,
		},
		{
			// a store may know some paths and not others; the unknown ones must still be
			// resolved the expensive way rather than disappear from the search
			name: "a path without a declared name falls back to the download",
			declared: map[string][]string{
				"alpha": {"kubernetes-admin@alpha"},
			},
			wantNames:   []string{"namer/ctx-beta", "namer/kubernetes-admin@alpha"},
			wantFetched: []string{"beta"},
		},
		{
			name:        "no declared name at all behaves like before",
			declared:    nil,
			wantNames:   []string{"namer/ctx-alpha", "namer/ctx-beta"},
			wantFetched: []string{"alpha", "beta"},
		},
		{
			name: "a path may declare several contexts",
			declared: map[string][]string{
				"alpha": {"first@alpha", "second@alpha"},
				"beta":  {"kubernetes-admin@beta"},
			},
			wantNames:   []string{"namer/first@alpha", "namer/kubernetes-admin@beta", "namer/second@alpha"},
			wantFetched: nil,
		},
		{
			// a store without a context prefix must get its declared names back verbatim
			name:     "a store without a prefix",
			noPrefix: true,
			declared: map[string][]string{
				"alpha": {"kubernetes-admin@alpha"},
				"beta":  {"kubernetes-admin@beta"},
			},
			wantNames:   []string{"kubernetes-admin@alpha", "kubernetes-admin@beta"},
			wantFetched: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &searchStubStore{
				id:       "namer",
				kind:     types.StoreKindOVH,
				paths:    []string{"alpha", "beta"},
				declared: tt.declared,
				noPrefix: tt.noPrefix,
			}
			store := storetypes.KubeconfigStore(&contextNamerStore{searchStubStore: stub})

			names, errs := collect(t, []storetypes.KubeconfigStore{store})
			if len(errs) > 0 {
				t.Fatalf("search reported %v", errs)
			}
			if !slices.Equal(names, tt.wantNames) {
				t.Errorf("context names = %v, want %v", names, tt.wantNames)
			}
			if got := stub.fetchedPaths(); !slices.Equal(got, tt.wantFetched) {
				t.Errorf("downloaded kubeconfigs for %v, want %v", got, tt.wantFetched)
			}
		})
	}
}

func TestDoSearch_StoreWithoutContextNamer(t *testing.T) {
	// a store that does not implement the interface must be unaffected
	stub := &searchStubStore{
		id:    "plain",
		kind:  types.StoreKindFilesystem,
		paths: []string{"alpha", "beta"},
	}

	names, errs := collect(t, []storetypes.KubeconfigStore{stub})
	if len(errs) > 0 {
		t.Fatalf("search reported %v", errs)
	}

	wantNames := []string{"plain/ctx-alpha", "plain/ctx-beta"}
	if !slices.Equal(names, wantNames) {
		t.Errorf("context names = %v, want %v", names, wantNames)
	}
	if got, want := stub.fetchedPaths(), []string{"alpha", "beta"}; !slices.Equal(got, want) {
		t.Errorf("downloaded kubeconfigs for %v, want %v", got, want)
	}
}

// TestDoSearch_DoesNotLeakGoroutines guards the per-path worker fan-out: the store
// goroutine must wait for every worker it started before it closes the result channel.
// More paths than the semaphore allows in flight, so that workers actually queue up.
//
// deliberately not parallel: goleak inspects every goroutine of the process.
func TestDoSearch_DoesNotLeakGoroutines(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	paths := make([]string, 0, defaultMaxConcurrentKubeconfigRequests*3)
	declared := make(map[string][]string)
	for i := range cap(paths) {
		path := fmt.Sprintf("cluster-%02d", i)
		paths = append(paths, path)
		// half of them are named up front, the other half go through the download
		if i%2 == 0 {
			declared[path] = []string{"kubernetes-admin@" + path}
		}
	}

	stub := &searchStubStore{
		id:       "namer",
		kind:     types.StoreKindOVH,
		paths:    paths,
		declared: declared,
	}

	names, errs := collect(t, []storetypes.KubeconfigStore{&contextNamerStore{searchStubStore: stub}})
	if len(errs) > 0 {
		t.Fatalf("search reported %v", errs)
	}
	if len(names) != len(paths) {
		t.Errorf("discovered %d contexts, want %d", len(names), len(paths))
	}
}

func TestMaxConcurrentKubeconfigRequests(t *testing.T) {
	t.Parallel()

	store := &searchStubStore{id: "store", kind: types.StoreKindFilesystem}
	if got := maxConcurrentKubeconfigRequests(store); got != defaultMaxConcurrentKubeconfigRequests {
		t.Errorf("expected the default of %d, got %d", defaultMaxConcurrentKubeconfigRequests, got)
	}

	configured := 3
	store.maxConcurrent = &configured
	if got := maxConcurrentKubeconfigRequests(store); got != configured {
		t.Errorf("expected the configured %d, got %d", configured, got)
	}

	// a zero would deadlock the semaphore, fall back to the default
	invalid := 0
	store.maxConcurrent = &invalid
	if got := maxConcurrentKubeconfigRequests(store); got != defaultMaxConcurrentKubeconfigRequests {
		t.Errorf("expected the default of %d for a non-positive value, got %d", defaultMaxConcurrentKubeconfigRequests, got)
	}
}
