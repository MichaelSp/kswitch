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
	"github.com/MichaelSp/kswitch/pkg/store/doks"
	gardenclient "github.com/MichaelSp/kswitch/pkg/store/gardener/copied_gardenctlv2"
	"github.com/MichaelSp/kswitch/pkg/store/plugins"
	"github.com/MichaelSp/kswitch/types"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	eks "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/digitalocean/doctl/do"
	exoscale "github.com/exoscale/egoscale/v3"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/linode/linodego/v2"
	"github.com/ovh/go-ovh/ovh"
	"github.com/rancher/norman/clientbase"
	managementClient "github.com/rancher/rancher/pkg/client/generated/management/v3"
	"github.com/scaleway/scaleway-sdk-go/scw"
	gkev1 "google.golang.org/api/container/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FilesystemStore struct {
	BaseStore
	KubeconfigName        string
	kubeconfigDirectories []string
	kubeconfigFilepaths   []string
}

type VaultStore struct {
	BaseStore
	Client             *vaultapi.Client
	VaultKeyKubeconfig string
	KubeconfigName     string
	EngineVersion      string
	vaultPaths         []string
}

type GardenerStore struct {
	BaseStore
	lazyInit
	GardenClient           gardenclient.Client
	Client                 client.Client
	Config                 *types.StoreConfigGardener
	LandscapeIdentity      string
	LandscapeName          string
	StateDirectory         string
	CachePathToShoot       *clusterCache[string, gardencorev1beta1.Shoot]
	CachePathToManagedSeed *clusterCache[string, seedmanagementv1alpha1.ManagedSeed]
	CacheCaNameToCACM      *clusterCache[string, corev1.ConfigMap]
}

type EKSStore struct {
	BaseStore
	lazyInit
	Client *awseks.Client
	Config *types.StoreConfigEKS
	// DiscoveredClusters maps the kubeconfig path (az_<resource-group>--<cluster-name>) -> cluster
	// This is a cache for the clusters discovered during the initial search for kubeconfig paths
	// when not using a search index
	DiscoveredClusters *clusterCache[string, *eks.Cluster]
	StateDirectory     string
}

type GKEStore struct {
	BaseStore
	lazyInit
	GkeClient *gkev1.Service
	Config    *types.StoreConfigGKE
	// DiscoveredClusters maps the kubeconfig path (gke--project-name--clusterName) -> cluster
	// This is a cache for the clusters discovered during the initial search for kubeconfig paths
	// when not using a search index
	DiscoveredClusters *clusterCache[string, *gkev1.Cluster]
	// ProjectNameToID contains a mapping projectName -> project ID
	// used to construct the kubeconfig path containing the project name instead of a technical project id
	ProjectNameToID map[string]string
	StateDirectory  string
}

type AzureStore struct {
	BaseStore
	lazyInit
	AksClient *armcontainerservice.ManagedClustersClient
	Config    *types.StoreConfigAzure
	// DiscoveredClusters maps the kubeconfig path (az_<resource-group>--<cluster-name>) -> cluster
	// This is a cache for the clusters discovered during the initial search for kubeconfig paths
	// when not using a search index
	DiscoveredClusters *clusterCache[string, *armcontainerservice.ManagedCluster]
	StateDirectory     string
}

type ExoscaleStore struct {
	BaseStore
	Client             *exoscale.Client
	DiscoveredClusters *clusterCache[exoscale.UUID, ExoscaleKube]
}

type RancherStore struct {
	BaseStore
	ClientOpts *clientbase.ClientOpts
	Client     *managementClient.Client
}

type OVHStore struct {
	BaseStore
	Client       *ovh.Client
	OVHKubeCache *clusterCache[string, OVHKube] // keyed by clusterID
}

type ScalewayStore struct {
	BaseStore
	Client             *scw.Client
	DiscoveredClusters *clusterCache[string, ScalewayKube]
}

type DigitalOceanStore struct {
	BaseStore
	lazyInit
	ContextToKubernetesService map[string]do.KubernetesService
	Config                     doks.DoctlConfig
}

type AkamaiStore struct {
	BaseStore
	Client *linodego.Client
	Config *types.StoreConfigAkamai
}

type CapiStore struct {
	BaseStore
	Client client.Client
	Config *types.StoreConfigCapi
}

type PluginStore struct {
	BaseStore
	Config *types.StoreConfigPlugin
	Client plugins.Store
}
