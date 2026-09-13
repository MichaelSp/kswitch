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
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ovh/go-ovh/ovh"
	"github.com/sirupsen/logrus"
	"go.uber.org/goleak"

	"github.com/MichaelSp/kswitch/types"
)

// ovhFakeAPI stands in for the OVH API and serves the endpoints the store walks: the
// projects of the account, the cluster IDs of a project, the details of a cluster and
// the generation of a kubeconfig.
type ovhFakeAPI struct {
	// clusters maps a project ID to its clusters, keyed by cluster ID and holding the
	// cluster name
	clusters map[string]map[string]string
	// failListProjects makes the listing of the projects themselves answer a 500
	failListProjects bool
	// failProjects are the projects whose cluster listing answers a 500
	failProjects map[string]bool
	// failClusters are the cluster IDs whose detail lookup answers a 500
	failClusters map[string]bool
	// delay is slept in every cluster related handler, so that a serial implementation
	// is measurably slower than a parallel one
	delay time.Duration

	mutex sync.Mutex
	// inFlight and maxInFlight record the observed concurrency
	inFlight, maxInFlight int
	// kubeconfigRequests counts the generated kubeconfigs, keyed by cluster ID
	kubeconfigRequests map[string]int
}

func (a *ovhFakeAPI) enter() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.inFlight++
	a.maxInFlight = max(a.maxInFlight, a.inFlight)
}

func (a *ovhFakeAPI) leave() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.inFlight--
}

func (a *ovhFakeAPI) concurrency() int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.maxInFlight
}

func (a *ovhFakeAPI) kubeconfigCount(clusterID string) int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.kubeconfigRequests[clusterID]
}

func (a *ovhFakeAPI) countKubeconfig(clusterID string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.kubeconfigRequests == nil {
		a.kubeconfigRequests = map[string]int{}
	}
	a.kubeconfigRequests[clusterID]++
}

func (a *ovhFakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// the go-ovh client synchronises its clock against the API before it signs a request
	if r.URL.Path == "/auth/time" {
		_, _ = fmt.Fprintf(w, "%d", time.Now().Unix())
		return
	}

	a.enter()
	defer a.leave()

	segments := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch {
	// GET /cloud/project
	case len(segments) == 2:
		if a.failListProjects {
			http.Error(w, `{"message":"listing the projects failed"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(slices.Sorted(maps.Keys(a.clusters)))

	// GET /cloud/project/<project>/kube
	case len(segments) == 4:
		time.Sleep(a.delay)
		project := segments[2]
		if a.failProjects[project] {
			http.Error(w, `{"message":"listing the clusters failed"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(slices.Sorted(maps.Keys(a.clusters[project])))

	// GET /cloud/project/<project>/kube/<cluster>
	case len(segments) == 5:
		time.Sleep(a.delay)
		project, id := segments[2], segments[4]
		if a.failClusters[id] {
			http.Error(w, `{"message":"reading the cluster failed"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(OVHKube{ID: id, Name: a.clusters[project][id]})

	// POST /cloud/project/<project>/kube/<cluster>/kubeconfig
	case len(segments) == 6 && segments[5] == "kubeconfig":
		time.Sleep(a.delay)
		project, id := segments[2], segments[4]
		a.countKubeconfig(id)
		if a.failClusters[id] {
			http.Error(w, `{"message":"generating the kubeconfig failed"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"content": "apiVersion: v1\nkind: Config\ncurrent-context: " + a.clusters[project][id] + "\n",
		})

	default:
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}
}

// newTestOVHStore returns an OVHStore talking to the given fake API.
func newTestOVHStore(t *testing.T, api *ovhFakeAPI) *OVHStore {
	t.Helper()

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	newClient := func() (*ovh.Client, error) {
		client, err := ovh.NewClient("ovh-eu", "application-key", "application-secret", "consumer-key")
		if err != nil {
			return nil, err
		}
		if err := client.SetEndpoint(server.URL); err != nil {
			return nil, err
		}
		return client, nil
	}
	if _, err := newClient(); err != nil {
		t.Fatalf("failed to build an OVH client: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(&strings.Builder{})

	return &OVHStore{
		BaseStore: BaseStore{
			Kind:            types.StoreKindOVH,
			KubeconfigStore: types.KubeconfigStore{Kind: types.StoreKindOVH},
			Logger:          logrus.NewEntry(logger),
		},
		Clients:      newOVHClientPool(newClient),
		OVHKubeCache: newClusterCache[string, OVHKube](),
	}
}

func TestNewOVHStore(t *testing.T) {
	// the OVH SDK also reads ~/.ovh.conf, point it at an empty home so that the
	// developer machine cannot influence the result
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name      string
		config    map[string]any
		wantError string
	}{
		{
			name: "complete credentials",
			config: map[string]any{
				"application_key":    "ak",
				"application_secret": "as",
				"consumer_key":       "ck",
			},
		},
		{
			name: "explicit endpoint",
			config: map[string]any{
				"application_key":    "ak",
				"application_secret": "as",
				"consumer_key":       "ck",
				"endpoint":           "https://eu.api.ovh.com/1.0",
			},
		},
		{
			name:      "no config at all",
			config:    nil,
			wantError: "application key",
		},
		{
			name: "missing application key",
			config: map[string]any{
				"application_secret": "as",
				"consumer_key":       "ck",
			},
			wantError: "application key",
		},
		{
			name: "missing application secret",
			config: map[string]any{
				"application_key": "ak",
				"consumer_key":    "ck",
			},
			wantError: "application secret",
		},
		{
			name: "missing consumer key",
			config: map[string]any{
				"application_key":    "ak",
				"application_secret": "as",
			},
			wantError: "consumer key",
		},
		{
			// a malformed endpoint must be reported by the constructor and not on
			// the first API call, which only happens once a search is running
			name: "malformed endpoint",
			config: map[string]any{
				"application_key":    "ak",
				"application_secret": "as",
				"consumer_key":       "ck",
				"endpoint":           "https://eu.api.ovh.com/1.0/",
			},
			wantError: "failed to initialize OVH client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewOVHStore(types.KubeconfigStore{
				Kind:   types.StoreKindOVH,
				Config: tt.config,
			})

			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("NewOVHStore succeeded, want an error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("NewOVHStore error = %q, want it to contain %q", err, tt.wantError)
				}
				if store != nil {
					t.Errorf("NewOVHStore returned a store together with an error")
				}
				return
			}

			if err != nil {
				t.Fatalf("NewOVHStore failed: %v", err)
			}
			if store.Clients == nil {
				t.Errorf("NewOVHStore left the client pool nil")
			}
			if store.OVHKubeCache == nil {
				t.Errorf("NewOVHStore left the cluster cache nil")
			}
			// the pool must be usable straight away
			client, err := store.Clients.acquire()
			if err != nil {
				t.Fatalf("acquire on a freshly built store failed: %v", err)
			}
			store.Clients.release(client)
		})
	}
}

func TestOVHStore_GetContextPrefix(t *testing.T) {
	t.Parallel()

	id := "ovh-prod"
	showPrefix := true
	hidePrefix := false

	tests := []struct {
		name  string
		store types.KubeconfigStore
		want  string
	}{
		{
			name:  "defaults to the store kind",
			store: types.KubeconfigStore{Kind: types.StoreKindOVH},
			want:  "ovh",
		},
		{
			name:  "uses the store ID when set",
			store: types.KubeconfigStore{Kind: types.StoreKindOVH, ID: &id},
			want:  "ovh-prod",
		},
		{
			name:  "explicitly enabled prefix still uses the ID",
			store: types.KubeconfigStore{Kind: types.StoreKindOVH, ID: &id, ShowPrefix: &showPrefix},
			want:  "ovh-prod",
		},
		{
			name:  "disabled prefix wins over the ID",
			store: types.KubeconfigStore{Kind: types.StoreKindOVH, ID: &id, ShowPrefix: &hidePrefix},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &OVHStore{BaseStore: NewBaseStore(types.StoreKindOVH, tt.store)}
			if got := store.GetContextPrefix("some/cluster"); got != tt.want {
				t.Errorf("GetContextPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOVHStore_StartSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		api  *ovhFakeAPI
		// wantClusters maps the expected kubeconfig path to its expected project tag
		wantClusters map[string]string
		wantErrors   int
		// wantErrorContains is looked for in the first reported error
		wantErrorContains string
	}{
		{
			name: "clusters of every project are reported with their identifying tags",
			api: &ovhFakeAPI{clusters: map[string]map[string]string{
				"project-a": {"id-1": "alpha", "id-2": "beta"},
				"project-b": {"id-3": "gamma"},
			}},
			wantClusters: map[string]string{"alpha": "project-a", "beta": "project-a", "gamma": "project-b"},
		},
		{
			name: "an account without projects reports nothing",
			api:  &ovhFakeAPI{clusters: map[string]map[string]string{}},
		},
		{
			name: "a project without clusters is skipped",
			api: &ovhFakeAPI{clusters: map[string]map[string]string{
				"project-a": {},
				"project-b": {"id-1": "alpha"},
			}},
			wantClusters: map[string]string{"alpha": "project-b"},
		},
		{
			name: "an unreadable project does not hide the clusters of the others",
			api: &ovhFakeAPI{
				clusters: map[string]map[string]string{
					"project-a": {"id-1": "alpha"},
					"forbidden": {"id-2": "beta"},
					"project-c": {"id-3": "gamma"},
				},
				failProjects: map[string]bool{"forbidden": true},
			},
			wantClusters:      map[string]string{"alpha": "project-a", "gamma": "project-c"},
			wantErrors:        1,
			wantErrorContains: "forbidden",
		},
		{
			name: "an unreadable cluster does not hide the other clusters",
			api: &ovhFakeAPI{
				clusters:     map[string]map[string]string{"project-a": {"id-1": "alpha", "id-2": "beta", "id-3": "gamma"}},
				failClusters: map[string]bool{"id-2": true},
			},
			wantClusters:      map[string]string{"alpha": "project-a", "gamma": "project-a"},
			wantErrors:        1,
			wantErrorContains: "id-2",
		},
		{
			name:              "a failing project listing is reported once",
			api:               &ovhFakeAPI{failListProjects: true},
			wantErrors:        1,
			wantErrorContains: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newTestOVHStore(t, tt.api)
			found, errs := drainSearch(store)

			if len(errs) != tt.wantErrors {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tt.wantErrors, errs)
			}
			if tt.wantErrorContains != "" && !strings.Contains(errs[0].Error(), tt.wantErrorContains) {
				t.Errorf("error %v does not mention %q", errs[0], tt.wantErrorContains)
			}
			if len(found) != len(tt.wantClusters) {
				t.Fatalf("got %d clusters %v, want %d", len(found), slices.Sorted(maps.Keys(found)), len(tt.wantClusters))
			}
			for path, project := range tt.wantClusters {
				tags, ok := found[path]
				if !ok {
					t.Errorf("cluster %q missing, got %v", path, slices.Sorted(maps.Keys(found)))
					continue
				}
				if tags[tagOVHProjectID] != project {
					t.Errorf("cluster %q: tag %q = %q, want %q", path, tagOVHProjectID, tags[tagOVHProjectID], project)
				}
				if tags[tagOVHClusterID] == "" {
					t.Errorf("cluster %q: tag %q is empty", path, tagOVHClusterID)
				}
				// the details must be cached so that GetKubeconfigForPath can resolve a
				// path that carries no tags
				if _, cached := store.OVHKubeCache.Get(tags[tagOVHClusterID]); !cached {
					t.Errorf("cluster %q was not added to the cache", path)
				}
			}
		})
	}
}

// TestOVHStore_StartSearch_IsParallel covers that projects and clusters are queried
// concurrently and that the two levels overlap: the OVH API answers one project resp.
// one cluster per request and every round trip takes seconds, so a serial search is
// unusably slow.
func TestOVHStore_StartSearch_IsParallel(t *testing.T) {
	t.Parallel()

	const (
		projects           = 8
		clustersPerProject = 4
		delay              = 50 * time.Millisecond
	)

	api := &ovhFakeAPI{clusters: map[string]map[string]string{}, delay: delay}
	for p := range projects {
		project := fmt.Sprintf("project-%d", p)
		api.clusters[project] = map[string]string{}
		for c := range clustersPerProject {
			api.clusters[project][fmt.Sprintf("id-%d-%d", p, c)] = fmt.Sprintf("cluster-%d-%d", p, c)
		}
	}

	start := time.Now()
	found, errs := drainSearch(newTestOVHStore(t, api))
	elapsed := time.Since(start)

	if len(errs) > 0 {
		t.Fatalf("got errors: %v", errs)
	}
	if len(found) != projects*clustersPerProject {
		t.Fatalf("got %d clusters, want %d", len(found), projects*clustersPerProject)
	}

	// serially every request is awaited on its own, so the search would take
	// (projects + projects*clustersPerProject) * delay = 2s
	serial := (projects + projects*clustersPerProject) * delay
	if elapsed > serial/4 {
		t.Errorf("search took %v, want well below the serial %v", elapsed, serial)
	}
	if got := api.concurrency(); got < 2 {
		t.Errorf("saw at most %d requests in flight, want concurrent requests", got)
	}
}

// TestOVHStore_StartSearch_ConcurrencyIsBounded covers that the search does not open an
// unbounded number of connections against the OVH API.
func TestOVHStore_StartSearch_ConcurrencyIsBounded(t *testing.T) {
	t.Parallel()

	const projects = 4 * maxConcurrentListRequests

	api := &ovhFakeAPI{clusters: map[string]map[string]string{}, delay: 10 * time.Millisecond}
	for p := range projects {
		api.clusters[fmt.Sprintf("project-%d", p)] = map[string]string{
			fmt.Sprintf("id-%d", p): fmt.Sprintf("cluster-%d", p),
		}
	}

	found, errs := drainSearch(newTestOVHStore(t, api))

	if len(errs) > 0 {
		t.Fatalf("got errors: %v", errs)
	}
	if len(found) != projects {
		t.Fatalf("got %d clusters, want %d", len(found), projects)
	}
	// the listing and the describing each get their own budget
	if want, got := 2*maxConcurrentListRequests, api.concurrency(); got > want {
		t.Errorf("saw %d requests in flight, want at most %d", got, want)
	}
}

// TestOVHStore_StartSearch_DoesNotLeakGoroutines covers that the worker pools of the
// search always drain, including when a project or a cluster fails half way through.
// It is deliberately not parallel: goleak inspects every goroutine of the process.
func TestOVHStore_StartSearch_DoesNotLeakGoroutines(t *testing.T) {
	// registered first so that it runs last: t.Cleanup is LIFO and the test server has
	// to be shut down before the goroutines are counted
	t.Cleanup(func() { goleak.VerifyNone(t) })

	api := &ovhFakeAPI{
		clusters: map[string]map[string]string{
			"project-a": {"id-1": "alpha", "id-2": "beta"},
			"forbidden": {"id-3": "gamma"},
			"project-c": {"id-4": "delta"},
		},
		failProjects: map[string]bool{"forbidden": true},
		failClusters: map[string]bool{"id-2": true},
	}

	found, errs := drainSearch(newTestOVHStore(t, api))

	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), errs)
	}
	if len(found) != 2 {
		t.Fatalf("got %d clusters %v, want 2", len(found), slices.Sorted(maps.Keys(found)))
	}
}

func TestOVHStore_GetKubeconfigForPath(t *testing.T) {
	t.Parallel()

	newAPI := func() *ovhFakeAPI {
		return &ovhFakeAPI{clusters: map[string]map[string]string{
			"project-a": {"id-1": "alpha"},
			"project-b": {"id-2": "beta"},
		}}
	}

	t.Run("resolves the cluster from the tags without a prior search", func(t *testing.T) {
		t.Parallel()

		store := newTestOVHStore(t, newAPI())
		got, err := store.GetKubeconfigForPath("alpha", map[string]string{
			tagOVHClusterID: "id-1",
			tagOVHProjectID: "project-a",
		})
		if err != nil {
			t.Fatalf("GetKubeconfigForPath failed: %v", err)
		}
		if !strings.Contains(string(got), "current-context: alpha") {
			t.Errorf("got kubeconfig %q, want the one of alpha", got)
		}
	})

	t.Run("falls back to the cache for a path without tags", func(t *testing.T) {
		t.Parallel()

		api := newAPI()
		store := newTestOVHStore(t, api)
		if _, errs := drainSearch(store); len(errs) > 0 {
			t.Fatalf("search failed: %v", errs)
		}

		got, err := store.GetKubeconfigForPath("beta", nil)
		if err != nil {
			t.Fatalf("GetKubeconfigForPath failed: %v", err)
		}
		if !strings.Contains(string(got), "current-context: beta") {
			t.Errorf("got kubeconfig %q, want the one of beta", got)
		}
		if got := api.kubeconfigCount("id-2"); got != 1 {
			t.Errorf("generated %d kubeconfigs for id-2, want 1", got)
		}
	})

	t.Run("reports a path that cannot be resolved", func(t *testing.T) {
		t.Parallel()

		store := newTestOVHStore(t, newAPI())
		if _, err := store.GetKubeconfigForPath("unknown", nil); err == nil {
			t.Fatal("expected an error for an unresolvable path")
		}
	})

	// the search retrieves the kubeconfigs of a store in parallel, so this method is
	// called from several goroutines at once
	t.Run("is safe to call concurrently", func(t *testing.T) {
		t.Parallel()

		store := newTestOVHStore(t, newAPI())
		clusters := map[string]string{"alpha": "id-1", "beta": "id-2"}
		projects := map[string]string{"alpha": "project-a", "beta": "project-b"}

		wg := sync.WaitGroup{}
		for range 16 {
			for name, id := range clusters {
				wg.Go(func() {
					got, err := store.GetKubeconfigForPath(name, map[string]string{
						tagOVHClusterID: id,
						tagOVHProjectID: projects[name],
					})
					if err != nil {
						t.Errorf("GetKubeconfigForPath(%q) failed: %v", name, err)
						return
					}
					if !strings.Contains(string(got), "current-context: "+name) {
						t.Errorf("GetKubeconfigForPath(%q) = %q", name, got)
					}
				})
			}
		}
		wg.Wait()
	})
}

func TestOVHStore_ContextNamesForPath(t *testing.T) {
	t.Parallel()

	// the search-result path is the OVH cluster name, and OVH always names the context
	// of a generated kubeconfig after it. Predicting the name is what lets the search
	// skip the kubeconfig generation, a POST costing seconds per cluster.
	tests := []struct {
		name string
		path string
		tags map[string]string
		want []string
	}{
		{
			name: "cluster name",
			path: "production",
			tags: map[string]string{tagOVHClusterID: "id-1", tagOVHProjectID: "project-a"},
			want: []string{"kubernetes-admin@production"},
		},
		{
			name: "the tags are not needed to name the context",
			path: "alpha-ia",
			tags: nil,
			want: []string{"kubernetes-admin@alpha-ia"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newTestOVHStore(t, &ovhFakeAPI{})
			if got := store.ContextNamesForPath(tt.path, tt.tags); !slices.Equal(got, tt.want) {
				t.Errorf("ContextNamesForPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
