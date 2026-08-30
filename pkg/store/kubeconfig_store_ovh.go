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
	"fmt"
	"sync"

	"github.com/ovh/go-ovh/ovh"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindOVH, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewOVHStore(s)
	})
}

var (
	_ storetypes.KubeconfigStore = (*OVHStore)(nil)
	_ storetypes.ContextNamer    = (*OVHStore)(nil)
)

func NewOVHStore(store types.KubeconfigStore) (*OVHStore, error) {
	ovhStoreConfig, err := ParseStoreConfig[types.StoreConfigOVH](store)
	if err != nil {
		return nil, err
	}

	ovhApplicationKey := ovhStoreConfig.OVHApplicationKey
	if len(ovhApplicationKey) == 0 {
		return nil, fmt.Errorf("when using the OVH kubeconfig store, the application key for OVH has to be provided via a SwitchConfig file")
	}
	ovhApplicationSecret := ovhStoreConfig.OVHApplicationSecret
	if len(ovhApplicationSecret) == 0 {
		return nil, fmt.Errorf("when using the OVH kubeconfig store, the application secret for OVH has to be provided via a SwitchConfig file")
	}
	ovhConsumerKey := ovhStoreConfig.OVHConsumerKey
	if len(ovhConsumerKey) == 0 {
		return nil, fmt.Errorf("when using the OVH kubeconfig store, the consumer key for OVH has to be provided via a SwitchConfig file")
	}
	ovhEndpoint := ovhStoreConfig.OVHEndpoint
	if len(ovhEndpoint) == 0 {
		ovhEndpoint = "ovh-eu"
	}

	newClient := func() (*ovh.Client, error) {
		client, err := ovh.NewClient(ovhEndpoint, ovhApplicationKey, ovhApplicationSecret, ovhConsumerKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OVH client: %w", err)
		}
		return client, nil
	}

	// fail early on a malformed endpoint or credentials rather than on the first request
	if _, err := newClient(); err != nil {
		return nil, err
	}

	return &OVHStore{
		BaseStore:    NewBaseStore(types.StoreKindOVH, store),
		Clients:      newOVHClientPool(newClient),
		OVHKubeCache: newClusterCache[string, OVHKube](),
	}, nil
}

type OVHKube struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Project string
}

const (
	// tagOVHClusterID is the search-result tag holding the unique OVH cluster ID
	tagOVHClusterID = "clusterID"
	// tagOVHProjectID is the search-result tag holding the OVH project ID
	tagOVHProjectID = "projectID"
)

func (r *OVHStore) GetContextPrefix(path string) string {
	if r.GetStoreConfig().ShowPrefix != nil && !*r.GetStoreConfig().ShowPrefix {
		return ""
	}

	if r.GetStoreConfig().ID != nil {
		return *r.GetStoreConfig().ID
	}

	return string(types.StoreKindOVH)
}

func (r *OVHStore) StartSearch(channel chan storetypes.SearchResult) {
	r.Logger.Debug("OVH: start search")

	projects := []string{}
	// list OVH projects
	err := r.Clients.get("/cloud/project", &projects)
	if err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          err,
		}
		return
	}

	// the OVH API only answers for one project resp. one cluster per request and each
	// round trip takes seconds. The clusters of a project are described as soon as that
	// project has been listed, so both levels are queried in parallel. A project or a
	// cluster that fails no longer aborts the search either: one inaccessible project
	// must not hide the clusters of all the others.
	clusters := make(chan ovhClusterRef)
	go func() {
		defer close(clusters)
		r.listClusters(projects, clusters, channel)
	}()
	r.describeClusters(clusters, channel)
}

// ovhClusterRef identifies a Kubernetes cluster in the OVH API.
type ovhClusterRef struct {
	project string
	id      string
}

// listClusters lists the Kubernetes clusters of every project in parallel and hands
// their references to the describers.
func (r *OVHStore) listClusters(projects []string, clusters chan<- ovhClusterRef, channel chan storetypes.SearchResult) {
	var (
		wg        sync.WaitGroup
		semaphore = make(chan struct{}, maxConcurrentListRequests)
	)

	for _, project := range projects {
		semaphore <- struct{}{}
		wg.Go(func() {
			defer func() { <-semaphore }()

			clusterIDs := []string{}
			if err := r.Clients.get(fmt.Sprintf("/cloud/project/%v/kube", project), &clusterIDs); err != nil {
				channel <- storetypes.SearchResult{
					Error: fmt.Errorf("failed to list the Kubernetes clusters of OVH project %q: %w", project, err),
				}
				return
			}

			for _, id := range clusterIDs {
				clusters <- ovhClusterRef{project: project, id: id}
			}
		})
	}
	wg.Wait()
}

// describeClusters fetches the details of the discovered clusters in parallel and
// reports them on the search channel.
func (r *OVHStore) describeClusters(clusters <-chan ovhClusterRef, channel chan storetypes.SearchResult) {
	wg := sync.WaitGroup{}

	for range maxConcurrentListRequests {
		wg.Go(func() {
			for cluster := range clusters {
				r.describeCluster(cluster, channel)
			}
		})
	}
	wg.Wait()
}

// describeCluster reports a single Kubernetes cluster on the search channel.
func (r *OVHStore) describeCluster(cluster ovhClusterRef, channel chan storetypes.SearchResult) {
	var kube OVHKube
	if err := r.Clients.get(fmt.Sprintf("/cloud/project/%v/kube/%v", cluster.project, cluster.id), &kube); err != nil {
		channel <- storetypes.SearchResult{
			Error: fmt.Errorf("failed to get the OVH Kubernetes cluster %q of project %q: %w", cluster.id, cluster.project, err),
		}
		return
	}
	kube.Project = cluster.project
	r.OVHKubeCache.Set(kube.ID, kube)

	channel <- storetypes.SearchResult{
		KubeconfigPath: kube.Name,
		// the cluster ID and project uniquely identify the cluster in the
		// OVH API. Carrying them in the tags lets the kubeconfig be fetched
		// without the in-memory cache (e.g. when a search index is used)
		// and without colliding on duplicate cluster names.
		Tags: map[string]string{
			tagOVHClusterID: kube.ID,
			tagOVHProjectID: cluster.project,
		},
		Error: nil,
	}
}

func (r *OVHStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	r.Logger.Debugf("OVH: getting secret for path %q", path)

	// prefer the IDs carried in the tags (set during the search): they are
	// unique and work even when the in-memory cache is empty (search index).
	clusterID := tags[tagOVHClusterID]
	project := tags[tagOVHProjectID]
	if clusterID == "" || project == "" {
		// fallback for entries without tags: resolve from the cache by name
		for _, c := range r.OVHKubeCache.Values() {
			if c.Name == path {
				clusterID = c.ID
				project = c.Project
				break
			}
		}
	}
	if clusterID == "" || project == "" {
		return nil, fmt.Errorf("could not resolve an OVH cluster ID for %q", path)
	}

	response := struct {
		Content string `json:"content"`
	}{}
	err := r.Clients.post(fmt.Sprintf("/cloud/project/%v/kube/%v/kubeconfig", project, clusterID), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig for cluster '%s': %w", path, err)
	}
	return []byte(response.Content), nil
}

// ContextNamesForPath returns the context name OVH puts in the kubeconfig of a
// cluster, so that the search does not have to generate the kubeconfig (a POST
// taking seconds per cluster) only to read that name back out of it.
func (r *OVHStore) ContextNamesForPath(path string, _ map[string]string) []string {
	return []string{fmt.Sprintf("kubernetes-admin@%s", path)}
}
