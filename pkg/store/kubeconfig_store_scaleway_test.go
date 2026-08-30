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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/sirupsen/logrus"
	"go.uber.org/goleak"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// scalewayFakeAPI stands in for the Scaleway API and serves the project listing and the
// cluster listing, both paginated the way the real API is.
type scalewayFakeAPI struct {
	// clusters maps a project ID to its clusters, keyed by cluster ID and holding the
	// cluster name
	clusters map[string]map[string]string
	// pageSize is the number of entries returned per page
	pageSize int
	// failListProjects makes the listing of the projects answer a 500
	failListProjects bool
	// failProjects are the projects whose cluster listing answers a 403
	failProjects map[string]bool
	// failKubeconfig are the clusters whose kubeconfig download answers a 500
	failKubeconfig map[string]bool
	// delay is slept in the cluster listing, so that a serial implementation is
	// measurably slower than a parallel one
	delay time.Duration

	mutex sync.Mutex
	// inFlight and maxInFlight record the observed concurrency
	inFlight, maxInFlight int
	// pages records the requested page numbers per path
	pages map[string][]int
}

func (a *scalewayFakeAPI) enter(path string, page int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.inFlight++
	a.maxInFlight = max(a.maxInFlight, a.inFlight)
	if a.pages == nil {
		a.pages = map[string][]int{}
	}
	a.pages[path] = append(a.pages[path], page)
}

func (a *scalewayFakeAPI) leave() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.inFlight--
}

func (a *scalewayFakeAPI) concurrency() int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.maxInFlight
}

func (a *scalewayFakeAPI) requestedPages(path string) []int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return slices.Sorted(slices.Values(a.pages[path]))
}

// page returns the requested slice of entries together with the total count.
func (a *scalewayFakeAPI) page(entries []string, page int) ([]string, int) {
	size := a.pageSize
	if size <= 0 {
		size = len(entries)
	}
	start := min((page-1)*size, len(entries))
	return entries[start:min(start+size, len(entries))], len(entries)
}

// kubeconfigOf returns the kubeconfig the fake API serves for a cluster.
func kubeconfigOf(clusterID string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %[1]s
  cluster:
    server: https://%[1]s.example.invalid
users:
- name: %[1]s
  user:
    token: token-%[1]s
contexts:
- name: %[1]s
  context:
    cluster: %[1]s
    user: %[1]s
current-context: %[1]s
`, clusterID)
}

func (a *scalewayFakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)

	switch {
	case r.URL.Path == "/account/v3/projects":
		a.enter("projects", page)
		defer a.leave()
		if a.failListProjects {
			http.Error(w, `{"message":"listing the projects failed"}`, http.StatusInternalServerError)
			return
		}
		ids, total := a.page(slices.Sorted(maps.Keys(a.clusters)), page)
		projects := make([]map[string]string, 0, len(ids))
		for _, id := range ids {
			projects = append(projects, map[string]string{"id": id, "name": id})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": total, "projects": projects})

	case strings.HasSuffix(r.URL.Path, "/kubeconfig"):
		// /k8s/v1/regions/<region>/clusters/<id>/kubeconfig
		segments := strings.Split(r.URL.Path, "/")
		clusterID := segments[len(segments)-2]
		a.enter("kubeconfig", page)
		defer a.leave()
		if a.failKubeconfig[clusterID] {
			http.Error(w, `{"message":"kubeconfig generation failed"}`, http.StatusInternalServerError)
			return
		}
		// the API answers with a JSON envelope carrying the base64 encoded file
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":         "kubeconfig",
			"content_type": "application/octet-stream",
			"content":      []byte(kubeconfigOf(clusterID)),
		})

	case strings.HasSuffix(r.URL.Path, "/clusters"):
		project := r.URL.Query().Get("project_id")
		a.enter("clusters", page)
		defer a.leave()
		time.Sleep(a.delay)
		if a.failProjects[project] {
			http.Error(w, `{"message":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		ids, total := a.page(slices.Sorted(maps.Keys(a.clusters[project])), page)
		clusters := make([]map[string]string, 0, len(ids))
		for _, id := range ids {
			clusters = append(clusters, map[string]string{"id": id, "name": a.clusters[project][id]})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": total, "clusters": clusters})

	default:
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}
}

// newTestScalewayStore returns a ScalewayStore talking to the given fake API.
func newTestScalewayStore(t *testing.T, api *scalewayFakeAPI) *ScalewayStore {
	t.Helper()

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	client, err := scw.NewClient(
		scw.WithAPIURL(server.URL),
		scw.WithAuth("SCWXXXXXXXXXXXXXXXXX", "11111111-1111-1111-1111-111111111111"),
		scw.WithDefaultRegion(scw.RegionFrPar),
	)
	if err != nil {
		t.Fatalf("failed to build the Scaleway client: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(&strings.Builder{})

	return &ScalewayStore{
		BaseStore: BaseStore{
			Kind:            types.StoreKindScaleway,
			KubeconfigStore: types.KubeconfigStore{Kind: types.StoreKindScaleway},
			Logger:          logrus.NewEntry(logger),
		},
		Client:             client,
		DiscoveredClusters: newClusterCache[string, ScalewayKube](),
	}
}

func TestNewScalewayStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    map[string]any
		wantError string
	}{
		{
			name: "complete credentials",
			config: map[string]any{
				"access_key":      "SCWXXXXXXXXXXXXXXXXX",
				"secret_key":      "11111111-1111-1111-1111-111111111111",
				"organization_id": "22222222-2222-2222-2222-222222222222",
			},
		},
		{
			// no region configured falls back to fr-par instead of failing
			name: "explicit region",
			config: map[string]any{
				"access_key":      "SCWXXXXXXXXXXXXXXXXX",
				"secret_key":      "11111111-1111-1111-1111-111111111111",
				"organization_id": "22222222-2222-2222-2222-222222222222",
				"region":          "nl-ams",
			},
		},
		{
			name:      "no config at all",
			config:    nil,
			wantError: "access key",
		},
		{
			name: "missing access key",
			config: map[string]any{
				"secret_key":      "11111111-1111-1111-1111-111111111111",
				"organization_id": "22222222-2222-2222-2222-222222222222",
			},
			wantError: "access key",
		},
		{
			name: "missing organization ID",
			config: map[string]any{
				"access_key": "SCWXXXXXXXXXXXXXXXXX",
				"secret_key": "11111111-1111-1111-1111-111111111111",
			},
			wantError: "organization ID",
		},
		{
			name: "missing secret key",
			config: map[string]any{
				"access_key":      "SCWXXXXXXXXXXXXXXXXX",
				"organization_id": "22222222-2222-2222-2222-222222222222",
			},
			wantError: "secret key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewScalewayStore(types.KubeconfigStore{
				Kind:   types.StoreKindScaleway,
				Config: tt.config,
			})

			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("NewScalewayStore succeeded, want an error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("NewScalewayStore error = %q, want it to contain %q", err, tt.wantError)
				}
				if store != nil {
					t.Errorf("NewScalewayStore returned a store together with an error")
				}
				return
			}

			if err != nil {
				t.Fatalf("NewScalewayStore failed: %v", err)
			}
			if store.Client == nil {
				t.Errorf("NewScalewayStore left the client nil")
			}
			if store.DiscoveredClusters == nil {
				t.Errorf("NewScalewayStore left the cluster cache nil")
			}
		})
	}
}

func TestScalewayStore_GetContextPrefix(t *testing.T) {
	t.Parallel()

	id := "scw-prod"
	showPrefix := true
	hidePrefix := false

	tests := []struct {
		name  string
		store types.KubeconfigStore
		want  string
	}{
		{
			name:  "defaults to the store kind",
			store: types.KubeconfigStore{Kind: types.StoreKindScaleway},
			want:  "scaleway",
		},
		{
			name:  "uses the store ID when set",
			store: types.KubeconfigStore{Kind: types.StoreKindScaleway, ID: &id},
			want:  "scw-prod",
		},
		{
			name:  "explicitly enabled prefix still uses the ID",
			store: types.KubeconfigStore{Kind: types.StoreKindScaleway, ID: &id, ShowPrefix: &showPrefix},
			want:  "scw-prod",
		},
		{
			name:  "disabled prefix wins over the ID",
			store: types.KubeconfigStore{Kind: types.StoreKindScaleway, ID: &id, ShowPrefix: &hidePrefix},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &ScalewayStore{BaseStore: NewBaseStore(types.StoreKindScaleway, tt.store)}
			if got := store.GetContextPrefix("some/cluster"); got != tt.want {
				t.Errorf("GetContextPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScalewayStore_StartSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		api  *scalewayFakeAPI
		// wantClusters maps the expected kubeconfig path to its expected project tag
		wantClusters map[string]string
		wantErrors   int
		// wantErrorContains is looked for in the first reported error
		wantErrorContains string
	}{
		{
			name: "clusters of every project are reported with their identifying tags",
			api: &scalewayFakeAPI{clusters: map[string]map[string]string{
				"project-a": {"id-1": "alpha", "id-2": "beta"},
				"project-b": {"id-3": "gamma"},
			}},
			wantClusters: map[string]string{"alpha": "project-a", "beta": "project-a", "gamma": "project-b"},
		},
		{
			name: "a project without clusters is skipped",
			api: &scalewayFakeAPI{clusters: map[string]map[string]string{
				"project-a": {},
				"project-b": {"id-1": "alpha"},
			}},
			wantClusters: map[string]string{"alpha": "project-b"},
		},
		{
			name: "a project the credentials cannot read does not hide the others",
			api: &scalewayFakeAPI{
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
			name:              "a failing project listing is reported once",
			api:               &scalewayFakeAPI{failListProjects: true},
			wantErrors:        1,
			wantErrorContains: "could not list projects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newTestScalewayStore(t, tt.api)
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
				if tags[tagScalewayProjectID] != project {
					t.Errorf("cluster %q: tag %q = %q, want %q", path, tagScalewayProjectID, tags[tagScalewayProjectID], project)
				}
				if tags[tagScalewayClusterID] == "" {
					t.Errorf("cluster %q: tag %q is empty", path, tagScalewayClusterID)
				}
				if _, cached := store.DiscoveredClusters.Get(tags[tagScalewayClusterID]); !cached {
					t.Errorf("cluster %q was not added to the cache", path)
				}
			}
		})
	}
}

// TestScalewayStore_StartSearch_ReadsEveryPage covers that the search asks for all the
// pages of the project and of the cluster listing. Without scw.WithAllPages the SDK
// requests a single page and everything past the server side default page size is
// silently missing from the search.
func TestScalewayStore_StartSearch_ReadsEveryPage(t *testing.T) {
	t.Parallel()

	const (
		projects           = 5
		clustersPerProject = 5
		pageSize           = 2
	)

	api := &scalewayFakeAPI{clusters: map[string]map[string]string{}, pageSize: pageSize}
	for p := range projects {
		project := fmt.Sprintf("project-%d", p)
		api.clusters[project] = map[string]string{}
		for c := range clustersPerProject {
			api.clusters[project][fmt.Sprintf("id-%d-%d", p, c)] = fmt.Sprintf("cluster-%d-%d", p, c)
		}
	}

	found, errs := drainSearch(newTestScalewayStore(t, api))

	if len(errs) > 0 {
		t.Fatalf("got errors: %v", errs)
	}
	if want := projects * clustersPerProject; len(found) != want {
		t.Fatalf("got %d clusters, want %d: the listing stopped early", len(found), want)
	}
	// 5 projects at 2 per page is 3 pages
	if got := api.requestedPages("projects"); len(got) != 3 {
		t.Errorf("requested the project pages %v, want 3 pages", got)
	}
}

// TestScalewayStore_StartSearch_IsParallel covers that the projects are queried
// concurrently: listing the clusters costs one round trip per project.
func TestScalewayStore_StartSearch_IsParallel(t *testing.T) {
	t.Parallel()

	const (
		projects = 12
		delay    = 50 * time.Millisecond
	)

	api := &scalewayFakeAPI{clusters: map[string]map[string]string{}, delay: delay}
	for p := range projects {
		project := fmt.Sprintf("project-%d", p)
		api.clusters[project] = map[string]string{fmt.Sprintf("id-%d", p): fmt.Sprintf("cluster-%d", p)}
	}

	start := time.Now()
	found, errs := drainSearch(newTestScalewayStore(t, api))
	elapsed := time.Since(start)

	if len(errs) > 0 {
		t.Fatalf("got errors: %v", errs)
	}
	if len(found) != projects {
		t.Fatalf("got %d clusters, want %d", len(found), projects)
	}
	if serial := projects * delay; elapsed > serial/2 {
		t.Errorf("search took %v, want well below the serial %v", elapsed, serial)
	}
	if got := api.concurrency(); got < 2 {
		t.Errorf("saw at most %d requests in flight, want concurrent requests", got)
	}
}

// TestScalewayStore_StartSearch_ConcurrencyIsBounded covers that the search does not
// open an unbounded number of connections against the Scaleway API.
func TestScalewayStore_StartSearch_ConcurrencyIsBounded(t *testing.T) {
	t.Parallel()

	const projects = 4 * maxConcurrentListRequests

	api := &scalewayFakeAPI{clusters: map[string]map[string]string{}, delay: 10 * time.Millisecond}
	for p := range projects {
		api.clusters[fmt.Sprintf("project-%d", p)] = map[string]string{
			fmt.Sprintf("id-%d", p): fmt.Sprintf("cluster-%d", p),
		}
	}

	found, errs := drainSearch(newTestScalewayStore(t, api))

	if len(errs) > 0 {
		t.Fatalf("got errors: %v", errs)
	}
	if len(found) != projects {
		t.Fatalf("got %d clusters, want %d", len(found), projects)
	}
	if got := api.concurrency(); got > maxConcurrentListRequests {
		t.Errorf("saw %d requests in flight, want at most %d", got, maxConcurrentListRequests)
	}
}

// TestScalewayStore_StartSearch_DoesNotLeakGoroutines covers that the worker pool of the
// search always drains, including when a project fails half way through. It is
// deliberately not parallel: goleak inspects every goroutine of the process.
func TestScalewayStore_StartSearch_DoesNotLeakGoroutines(t *testing.T) {
	// registered first so that it runs last: t.Cleanup is LIFO and the test server has
	// to be shut down before the goroutines are counted
	t.Cleanup(func() { goleak.VerifyNone(t) })

	api := &scalewayFakeAPI{
		clusters: map[string]map[string]string{
			"project-a": {"id-1": "alpha"},
			"forbidden": {"id-2": "beta"},
		},
		failProjects: map[string]bool{"forbidden": true},
	}

	found, errs := drainSearch(newTestScalewayStore(t, api))

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if len(found) != 1 {
		t.Fatalf("got %d clusters %v, want 1", len(found), slices.Sorted(maps.Keys(found)))
	}
}

func TestScalewayStore_GetKubeconfigForPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prime decides whether a search runs first, filling the cluster cache
		prime          bool
		path           string
		tags           map[string]string
		failKubeconfig map[string]bool
		wantCluster    string
		wantError      string
	}{
		{
			name:        "resolves the cluster from the tags",
			path:        "prod",
			tags:        map[string]string{tagScalewayClusterID: "cluster-a"},
			wantCluster: "cluster-a",
		},
		{
			// entries coming from a search index carry no tags, the name is then
			// resolved against the cache filled by the search
			name:        "falls back to the cache when the tags are empty",
			prime:       true,
			path:        "prod",
			wantCluster: "cluster-a",
		},
		{
			name:      "unknown name without tags",
			prime:     true,
			path:      "does-not-exist",
			wantError: `could not resolve a Scaleway cluster ID for "does-not-exist"`,
		},
		{
			name:      "unknown name and empty cache",
			path:      "prod",
			wantError: `could not resolve a Scaleway cluster ID for "prod"`,
		},
		{
			name:           "the API refuses to generate the kubeconfig",
			path:           "prod",
			tags:           map[string]string{tagScalewayClusterID: "cluster-a"},
			failKubeconfig: map[string]bool{"cluster-a": true},
			wantError:      "failed to get kubeconfig for cluster 'prod'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &scalewayFakeAPI{
				clusters: map[string]map[string]string{
					"project-1": {"cluster-a": "prod", "cluster-b": "staging"},
				},
				pageSize:       10,
				failKubeconfig: tt.failKubeconfig,
			}
			store := newTestScalewayStore(t, api)
			if tt.prime {
				if _, errs := drainSearch(store); len(errs) > 0 {
					t.Fatalf("priming search reported %v", errs)
				}
			}

			got, err := store.GetKubeconfigForPath(tt.path, tt.tags)

			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("GetKubeconfigForPath succeeded, want an error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("GetKubeconfigForPath error = %q, want it to contain %q", err, tt.wantError)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetKubeconfigForPath failed: %v", err)
			}
			if want := kubeconfigOf(tt.wantCluster); string(got) != want {
				t.Errorf("GetKubeconfigForPath returned\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// drainSearch drains a search into its results, keyed by kubeconfig path, and its errors.
func drainSearch(store storetypes.KubeconfigStore) (map[string]map[string]string, []error) {
	channel := make(chan storetypes.SearchResult)
	go func() {
		defer close(channel)
		store.StartSearch(channel)
	}()

	found := map[string]map[string]string{}
	var errs []error
	for result := range channel {
		if result.Error != nil {
			errs = append(errs, result.Error)
			continue
		}
		found[result.KubeconfigPath] = result.Tags
	}
	return found, errs
}
