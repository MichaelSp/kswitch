package store

import (
	"context"
	"fmt"
	"time"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var k0smotronClusterGVR = schema.GroupVersionResource{
	Group:    "k0smotron.io",
	Version:  "v1beta1",
	Resource: "clusters",
}

// K0smotronInMemoryStore is a lightweight in-memory store used to serve kubeconfigs
// discovered dynamically via k0smotron cluster expansion in the TUI.
// It is created on demand when the user presses → on a cluster item.
type K0smotronInMemoryStore struct {
	BaseStore
	// pathToKubeconfig maps a stable path key to raw kubeconfig bytes.
	pathToKubeconfig map[string][]byte
}

var _ storetypes.KubeconfigStore = (*K0smotronInMemoryStore)(nil)

func NewK0smotronInMemoryStore(id string) *K0smotronInMemoryStore {
	storeID := id
	storeConfig := types.KubeconfigStore{
		Kind: types.StoreKindK0smotron,
		ID:   &storeID,
	}
	return &K0smotronInMemoryStore{
		BaseStore:        NewBaseStore(types.StoreKindK0smotron, storeConfig),
		pathToKubeconfig: make(map[string][]byte),
	}
}

func (s *K0smotronInMemoryStore) GetContextPrefix(_ string) string {
	return string(types.StoreKindK0smotron)
}

// StartSearch is a no-op: this store is populated dynamically, not via background search.
func (s *K0smotronInMemoryStore) StartSearch(_ chan storetypes.SearchResult) {}

func (s *K0smotronInMemoryStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	data, ok := s.pathToKubeconfig[path]
	if !ok {
		return nil, fmt.Errorf("k0smotron: kubeconfig not found for path %q", path)
	}
	return data, nil
}

// DiscoverK0smotronClusters connects to the cluster at kubeconfigData and returns
// all k0smotron sub-clusters as a populated K0smotronInMemoryStore plus a list of
// (path, contextName, namespace, name) tuples for the TUI to display.
func DiscoverK0smotronClusters(parentPath string, kubeconfigData []byte) (*K0smotronInMemoryStore, []K0smotronClusterEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, nil, fmt.Errorf("k0smotron: build rest config: %w", err)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("k0smotron: create client: %w", err)
	}

	clusterList := &unstructured.UnstructuredList{}
	clusterList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   k0smotronClusterGVR.Group,
		Version: k0smotronClusterGVR.Version,
		Kind:    "ClusterList",
	})

	if err := k8sClient.List(ctx, clusterList); err != nil {
		if meta.IsNoMatchError(err) {
			// k0smotron not installed in this cluster — not an error
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("k0smotron: list clusters: %w", err)
	}

	storeID := "k0smotron-" + parentPath
	memStore := NewK0smotronInMemoryStore(storeID)
	var entries []K0smotronClusterEntry

	for _, cluster := range clusterList.Items {
		name := cluster.GetName()
		namespace := cluster.GetNamespace()
		secretName := name + "-kubeconfig"

		secret := &corev1.Secret{}
		secretKey := client.ObjectKey{Namespace: namespace, Name: secretName}
		if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
			continue // kubeconfig not ready yet
		}

		kubeconfigBytes, ok := secret.Data["value"]
		if !ok || len(kubeconfigBytes) == 0 {
			continue
		}

		path := fmt.Sprintf("%s/%s/%s", parentPath, namespace, name)
		memStore.pathToKubeconfig[path] = kubeconfigBytes

		displayName := fmt.Sprintf("%s/%s", namespace, name)
		entries = append(entries, K0smotronClusterEntry{
			Path:        path,
			DisplayName: displayName,
			Namespace:   namespace,
			Name:        name,
			StoreID:     storeID,
		})
	}

	return memStore, entries, nil
}

// K0smotronClusterEntry carries the TUI-relevant info about a discovered sub-cluster.
type K0smotronClusterEntry struct {
	Path        string
	DisplayName string
	Namespace   string
	Name        string
	StoreID     string
}
