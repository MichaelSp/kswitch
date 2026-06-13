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

	"github.com/ovh/go-ovh/ovh"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindOVH, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewOVHStore(s)
	})
}

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

	ovhClient, err := ovh.NewClient(ovhEndpoint, ovhApplicationKey, ovhApplicationSecret, ovhConsumerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OVH client: %w", err)
	}

	return &OVHStore{
		BaseStore:    NewBaseStore(types.StoreKindOVH, store),
		Client:       ovhClient,
		OVHKubeCache: make(map[string]OVHKube),
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
	err := r.Client.Get("/cloud/project", &projects)
	if err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          err,
		}
		return
	}

	// for each project, list Kubernetes cluster
	for _, project := range projects {
		clustersID := []string{}
		err := r.Client.Get(fmt.Sprintf("/cloud/project/%v/kube", project), &clustersID)
		if err != nil {
			channel <- storetypes.SearchResult{
				KubeconfigPath: "",
				Error:          err,
			}
			return
		}

		for _, id := range clustersID {
			var kube OVHKube
			err := r.Client.Get(fmt.Sprintf("/cloud/project/%v/kube/%v", project, id), &kube)
			if err != nil {
				channel <- storetypes.SearchResult{
					KubeconfigPath: "",
					Error:          err,
				}
				return
			}
			kube.Project = project
			r.OVHKubeCache[kube.ID] = kube

			channel <- storetypes.SearchResult{
				KubeconfigPath: kube.Name,
				// the cluster ID and project uniquely identify the cluster in the
				// OVH API. Carrying them in the tags lets the kubeconfig be fetched
				// without the in-memory cache (e.g. when a search index is used)
				// and without colliding on duplicate cluster names.
				Tags: map[string]string{
					tagOVHClusterID: kube.ID,
					tagOVHProjectID: project,
				},
				Error: nil,
			}
		}

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
		for _, c := range r.OVHKubeCache {
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
	err := r.Client.Post(fmt.Sprintf("/cloud/project/%v/kube/%v/kubeconfig", project, clusterID), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig for cluster '%s': %w", path, err)
	}
	return []byte(response.Content), nil
}
