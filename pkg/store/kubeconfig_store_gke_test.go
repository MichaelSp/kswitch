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

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// gkeProject describes what the fake GKE API answers for one GCP project.
type gkeProject struct {
	name     string // GCP project name, part of the kubeconfig path
	id       string // GCP project ID, used in the API request
	clusters []string
	// status, when non-zero, makes the fake API fail the request for this project.
	status int
	// hang blocks the request until the client gives up (its context is done).
	hang bool
	// delay slows the response down by this duration.
	delay time.Duration
}

// listClustersPath is the path the container/v1 client calls to list the
// clusters of a project across all zones.
func listClustersPath(projectID string) string {
	return fmt.Sprintf("/v1/projects/%s/zones/-/clusters", projectID)
}

// fakeGKEAPI serves the "list clusters" endpoint for the given projects and
// records how many requests were in flight at the same time.
type fakeGKEAPI struct {
	server *httptest.Server

	mu             sync.Mutex
	inFlight       int
	maxInFlight    int
	requestsByPath map[string]int

	// arrived is closed once every project has a request in flight, so a
	// handler can prove the requests really do overlap.
	arrived     chan struct{}
	arrivedOnce sync.Once
	expected    int
}

// newFakeGKEAPI starts a fake GKE API. When waitForAll is true, every handler
// blocks until all projects have a request in flight (or 5s pass), which fails
// the concurrency assertion below if the store queries projects sequentially.
func newFakeGKEAPI(t *testing.T, projects []gkeProject, waitForAll bool) *fakeGKEAPI {
	t.Helper()

	api := &fakeGKEAPI{
		requestsByPath: map[string]int{},
		arrived:        make(chan struct{}),
		expected:       len(projects),
	}

	byPath := map[string]gkeProject{}
	for _, p := range projects {
		byPath[listClustersPath(p.id)] = p
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		project, ok := byPath[r.URL.Path]
		if !ok {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}

		api.enter(r.URL.Path)
		defer api.leave()

		if waitForAll {
			select {
			case <-api.arrived:
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Second):
				// the store did not query the projects concurrently
			}
		}

		if project.hang {
			<-r.Context().Done()
			return
		}
		if project.delay > 0 {
			select {
			case <-time.After(project.delay):
			case <-r.Context().Done():
				return
			}
		}
		if project.status != 0 {
			http.Error(w, "boom", project.status)
			return
		}

		resp := container.ListClustersResponse{}
		for _, name := range project.clusters {
			resp.Clusters = append(resp.Clusters, &container.Cluster{Name: name, Location: "europe-west1"})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})

	api.server = httptest.NewServer(mux)
	t.Cleanup(api.server.Close)
	return api
}

func (a *fakeGKEAPI) enter(path string) {
	a.mu.Lock()
	a.inFlight++
	if a.inFlight > a.maxInFlight {
		a.maxInFlight = a.inFlight
	}
	a.requestsByPath[path]++
	allArrived := a.inFlight >= a.expected
	a.mu.Unlock()

	if allArrived {
		a.arrivedOnce.Do(func() { close(a.arrived) })
	}
}

func (a *fakeGKEAPI) leave() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inFlight--
}

func (a *fakeGKEAPI) concurrency() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.maxInFlight
}

func (a *fakeGKEAPI) requests(projectID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.requestsByPath[listClustersPath(projectID)]
}

// newTestGKEStore returns a GKE store that talks to the fake API and is marked
// as already initialized, so StartSearch goes straight to listing clusters.
func newTestGKEStore(t *testing.T, api *fakeGKEAPI, projects []gkeProject) *GKEStore {
	t.Helper()

	client, err := container.NewService(context.Background(),
		option.WithEndpoint(api.server.URL),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("create fake GKE client: %v", err)
	}

	projectNameToID := map[string]string{}
	for _, p := range projects {
		projectNameToID[p.name] = p.id
	}

	s := &GKEStore{
		BaseStore:          NewBaseStore(types.StoreKindGKE, types.KubeconfigStore{Kind: types.StoreKindGKE}),
		GkeClient:          client,
		Config:             &types.StoreConfigGKE{},
		ProjectNameToID:    projectNameToID,
		DiscoveredClusters: newClusterCache[string, *container.Cluster](),
	}
	// pretend the (network-dependent) initialization already succeeded
	s.ready.Store(true)

	return s
}

// searchResults holds what StartSearch delivered on the channel.
type searchResults struct {
	paths    []string
	errs     []error
	duration time.Duration
}

// runStartSearch drains the search channel while StartSearch runs and closes it
// afterwards, which also asserts that StartSearch does not leave goroutines
// sending results behind (that would panic on the closed channel).
func runStartSearch(t *testing.T, s *GKEStore) searchResults {
	t.Helper()

	channel := make(chan storetypes.SearchResult)
	var (
		results searchResults
		drained = make(chan struct{})
	)
	go func() {
		defer close(drained)
		for r := range channel {
			if r.Error != nil {
				results.errs = append(results.errs, r.Error)
				continue
			}
			results.paths = append(results.paths, r.KubeconfigPath)
		}
	}()

	start := time.Now()
	s.StartSearch(channel)
	results.duration = time.Since(start)

	close(channel)
	<-drained

	sort.Strings(results.paths)
	return results
}

// setClusterListTimeout shortens the per-project time budget for a test.
func setClusterListTimeout(t *testing.T, d time.Duration) {
	t.Helper()

	previous := gkeClusterListTimeout
	gkeClusterListTimeout = d
	t.Cleanup(func() { gkeClusterListTimeout = previous })
}

// TestGKEStoreStartSearch_QueriesProjectsConcurrently pins down that all
// projects are listed in parallel: the fake API only answers once every project
// has a request in flight.
func TestGKEStoreStartSearch_QueriesProjectsConcurrently(t *testing.T) {
	projects := []gkeProject{
		{name: "team-a", id: "team-a-1234", clusters: []string{"prod", "dev"}},
		{name: "team-b", id: "team-b-1234", clusters: []string{"prod"}},
		{name: "sys-1", id: "sys-1-1234", clusters: []string{"staging"}},
		{name: "sys-2", id: "sys-2-1234"},
	}
	api := newFakeGKEAPI(t, projects, true)
	s := newTestGKEStore(t, api, projects)

	results := runStartSearch(t, s)

	if len(results.errs) != 0 {
		t.Fatalf("unexpected errors: %v", results.errs)
	}
	want := []string{
		"gke_sys-1--europe-west1--staging",
		"gke_team-a--europe-west1--dev",
		"gke_team-a--europe-west1--prod",
		"gke_team-b--europe-west1--prod",
	}
	if strings.Join(results.paths, ",") != strings.Join(want, ",") {
		t.Errorf("discovered paths = %v, want %v", results.paths, want)
	}
	if got := api.concurrency(); got != len(projects) {
		t.Errorf("max concurrent list requests = %d, want %d (projects are queried sequentially)", got, len(projects))
	}

	// every discovered cluster must be cached for GetKubeconfigForPath
	for _, path := range want {
		if _, ok := s.DiscoveredClusters.Get(path); !ok {
			t.Errorf("cluster %q is missing from the discovered cluster cache", path)
		}
	}
}

// TestGKEStoreStartSearch_TimeoutBudgetIsPerProject is the regression test for
// the shared deadline: every project used to draw from a single time budget, so
// an account with many projects lost the later ones to spurious "context
// deadline exceeded" errors even though no single project was slow.
func TestGKEStoreStartSearch_TimeoutBudgetIsPerProject(t *testing.T) {
	const (
		perProjectDelay = 100 * time.Millisecond
		timeout         = 400 * time.Millisecond
	)
	setClusterListTimeout(t, timeout)

	var projects []gkeProject
	for i := range 8 {
		projects = append(projects, gkeProject{
			name:     fmt.Sprintf("project-%d", i),
			id:       fmt.Sprintf("project-%d-1234", i),
			clusters: []string{"prod"},
			delay:    perProjectDelay,
		})
	}
	// the accumulated response time has to exceed a single shared budget
	if time.Duration(len(projects))*perProjectDelay <= timeout {
		t.Fatalf("test setup: %d projects × %s do not exceed the %s budget", len(projects), perProjectDelay, timeout)
	}

	api := newFakeGKEAPI(t, projects, false)
	s := newTestGKEStore(t, api, projects)

	results := runStartSearch(t, s)

	if len(results.errs) != 0 {
		t.Errorf("unexpected errors: %v", results.errs)
	}
	if len(results.paths) != len(projects) {
		t.Errorf("discovered %d clusters, want %d: %v", len(results.paths), len(projects), results.paths)
	}
}

// TestGKEStoreStartSearch_SlowProjectDoesNotStarveOthers checks that a project
// that never answers only costs its own result.
func TestGKEStoreStartSearch_SlowProjectDoesNotStarveOthers(t *testing.T) {
	setClusterListTimeout(t, 300*time.Millisecond)

	projects := []gkeProject{
		{name: "hangs", id: "hangs-1234", clusters: []string{"prod"}, hang: true},
		{name: "team-a", id: "team-a-1234", clusters: []string{"prod"}},
		{name: "team-b", id: "team-b-1234", clusters: []string{"prod"}},
	}
	api := newFakeGKEAPI(t, projects, false)
	s := newTestGKEStore(t, api, projects)

	results := runStartSearch(t, s)

	want := []string{"gke_team-a--europe-west1--prod", "gke_team-b--europe-west1--prod"}
	if strings.Join(results.paths, ",") != strings.Join(want, ",") {
		t.Errorf("discovered paths = %v, want %v", results.paths, want)
	}
	if len(results.errs) != 1 {
		t.Fatalf("got %d errors, want 1 (only the hanging project): %v", len(results.errs), results.errs)
	}
	if !strings.Contains(results.errs[0].Error(), "hangs-1234") {
		t.Errorf("error = %v, want it to name project hangs-1234", results.errs[0])
	}
	// the search must not wait longer than one project's budget
	if results.duration > 3*time.Second {
		t.Errorf("StartSearch took %s, want roughly the per-project timeout", results.duration)
	}
}

// TestGKEStoreStartSearch_ProjectErrorDoesNotAbortSearch checks that a project
// the account cannot list (e.g. a disabled GKE API) is reported but does not
// hide the clusters of the other projects.
func TestGKEStoreStartSearch_ProjectErrorDoesNotAbortSearch(t *testing.T) {
	projects := []gkeProject{
		{name: "forbidden", id: "forbidden-1234", status: http.StatusForbidden},
		{name: "team-a", id: "team-a-1234", clusters: []string{"prod"}},
		{name: "team-b", id: "team-b-1234", clusters: []string{"dev"}},
	}
	api := newFakeGKEAPI(t, projects, false)
	s := newTestGKEStore(t, api, projects)

	results := runStartSearch(t, s)

	want := []string{"gke_team-a--europe-west1--prod", "gke_team-b--europe-west1--dev"}
	sort.Strings(want)
	if strings.Join(results.paths, ",") != strings.Join(want, ",") {
		t.Errorf("discovered paths = %v, want %v", results.paths, want)
	}
	if len(results.errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(results.errs), results.errs)
	}
	if !strings.Contains(results.errs[0].Error(), "forbidden-1234") {
		t.Errorf("error = %v, want it to name project forbidden-1234", results.errs[0])
	}
}

// TestGKEStoreStartSearch_QueriesEveryProjectOnce guards against a loop-variable
// capture bug: each project has to be listed exactly once.
func TestGKEStoreStartSearch_QueriesEveryProjectOnce(t *testing.T) {
	projects := []gkeProject{
		{name: "team-a", id: "team-a-1234", clusters: []string{"prod"}},
		{name: "team-b", id: "team-b-1234", clusters: []string{"prod"}},
		{name: "team-c", id: "team-c-1234", clusters: []string{"prod"}},
	}
	api := newFakeGKEAPI(t, projects, false)
	s := newTestGKEStore(t, api, projects)

	results := runStartSearch(t, s)

	if len(results.errs) != 0 {
		t.Fatalf("unexpected errors: %v", results.errs)
	}
	for _, p := range projects {
		if got := api.requests(p.id); got != 1 {
			t.Errorf("project %s was listed %d times, want 1", p.id, got)
		}
	}
	if len(results.paths) != len(projects) {
		t.Errorf("discovered %d clusters, want %d", len(results.paths), len(projects))
	}
}

// TestGKEStoreStartSearch_NoProjects covers the empty store: nothing is
// searched and StartSearch returns without blocking.
func TestGKEStoreStartSearch_NoProjects(t *testing.T) {
	api := newFakeGKEAPI(t, nil, false)
	s := newTestGKEStore(t, api, nil)

	results := runStartSearch(t, s)

	if len(results.paths) != 0 || len(results.errs) != 0 {
		t.Errorf("got paths %v and errors %v, want none", results.paths, results.errs)
	}
}
