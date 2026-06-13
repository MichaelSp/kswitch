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

	"github.com/scaleway/scaleway-sdk-go/api/account/v3"
	"github.com/scaleway/scaleway-sdk-go/api/k8s/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindScaleway, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewScalewayStore(s)
	})
}

var _ storetypes.KubeconfigStore = (*ScalewayStore)(nil)

func NewScalewayStore(store types.KubeconfigStore) (*ScalewayStore, error) {
	scalewayStoreConfig, err := ParseStoreConfig[types.StoreConfigScaleway](store)
	if err != nil {
		return nil, err
	}
	base := NewBaseStore(types.StoreKindScaleway, store)

	scalewayAccessKey := scalewayStoreConfig.ScalewayAccessKey
	if len(scalewayAccessKey) == 0 {
		return nil, fmt.Errorf("when using the Scaleway kubeconfig store, the access key for Scaleway has to be provided via a SwitchConfig file")
	}
	scalewayOrganizationID := scalewayStoreConfig.ScalewayOrganizationID
	if len(scalewayOrganizationID) == 0 {
		return nil, fmt.Errorf("when using the Scaleway kubeconfig store, the organization ID for Scaleway has to be provided via a SwitchConfig file")
	}
	scalewaySecretKey := scalewayStoreConfig.ScalewaySecretKey
	if len(scalewaySecretKey) == 0 {
		return nil, fmt.Errorf("when using the Scaleway kubeconfig store, the secret key for Scaleway has to be provided via a SwitchConfig file")
	}
	scalewayRegion := scalewayStoreConfig.ScalewayRegion
	if len(scalewayRegion) == 0 {
		base.Logger.Warning("No region specified for scaleway, using default 'fr-par'")
		scalewayRegion = "fr-par"
	}

	client, err := scw.NewClient(
		scw.WithDefaultOrganizationID(scalewayOrganizationID),
		scw.WithAuth(scalewayAccessKey, scalewaySecretKey),
		scw.WithDefaultRegion(scw.Region(scalewayRegion)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Scaleway client: %w", err)
	}

	return &ScalewayStore{
		BaseStore:          base,
		Client:             client,
		DiscoveredClusters: make(map[string]ScalewayKube),
	}, nil
}

type ScalewayKube struct {
	ID      string
	Name    string
	Project string
}

const (
	// tagScalewayClusterID is the search-result tag holding the unique Scaleway cluster ID
	tagScalewayClusterID = "clusterID"
	// tagScalewayProjectID is the search-result tag holding the Scaleway project ID
	tagScalewayProjectID = "projectID"
)

func (s *ScalewayStore) GetContextPrefix(path string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}

	if s.GetStoreConfig().ID != nil {
		return *s.GetStoreConfig().ID
	}

	return string(types.StoreKindScaleway)
}

func (s *ScalewayStore) StartSearch(channel chan storetypes.SearchResult) {
	s.Logger.Debug("Scaleway: start search")

	papi := account.NewProjectAPI(s.Client)
	if papi == nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("failed to create scaleway project API"),
		}
		return
	}
	pres, err := papi.ListProjects(
		&account.ProjectAPIListProjectsRequest{},
	)
	if err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("could not list projects in Scaleway: %w", err),
		}
		return
	}
	// list projects

	kapi := k8s.NewAPI(s.Client)
	if kapi == nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("failed to create Kubernetes API instance for scaleway: %w", err),
		}
		return
	}

	for _, project := range pres.Projects {
		cres, err := kapi.ListClusters(&k8s.ListClustersRequest{ProjectID: &project.ID})
		if err != nil {
			channel <- storetypes.SearchResult{
				KubeconfigPath: "",
				Error:          fmt.Errorf("failed to retrieve Kubernetes cluster for project %v: %w", project.Name, err),
			}
			return
		}
		if cres.TotalCount == 0 {
			s.Logger.Debug("No k8s clusters in project", project.Name)
			continue
		}
		for _, cluster := range cres.Clusters {
			s.DiscoveredClusters[cluster.ID] = ScalewayKube{ID: cluster.ID, Name: cluster.Name, Project: project.ID}
			channel <- storetypes.SearchResult{
				KubeconfigPath: cluster.Name,
				// the cluster ID uniquely identifies the cluster in the Scaleway
				// API. It is carried in the tags so the kubeconfig can be fetched
				// without relying on the in-memory cache (e.g. when a search index
				// is used) and without colliding on duplicate cluster names.
				Tags: map[string]string{
					tagScalewayClusterID: cluster.ID,
					tagScalewayProjectID: project.ID,
				},
				Error: nil,
			}
		}
	}
}

func (s *ScalewayStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	s.Logger.Debugf("Scaleway: getting secret for path %q", path)

	// prefer the cluster ID carried in the tags (set during the search): it is
	// unique and works even when the in-memory cache is empty (search index).
	clusterID := tags[tagScalewayClusterID]
	if clusterID == "" {
		// fallback for entries without tags: resolve the ID from the cache by name
		for _, c := range s.DiscoveredClusters {
			if c.Name == path {
				clusterID = c.ID
				break
			}
		}
	}
	if clusterID == "" {
		return nil, fmt.Errorf("could not resolve a Scaleway cluster ID for %q", path)
	}

	kapi := k8s.NewAPI(s.Client)

	config, err := kapi.GetClusterKubeConfig(&k8s.GetClusterKubeConfigRequest{
		ClusterID: clusterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig for cluster '%s': %w", path, err)
	}
	return config.GetRaw(), nil
}
