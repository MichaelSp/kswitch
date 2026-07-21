package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// minimalKubeconfig builds a minimal valid kubeconfig with the given current-context.
func minimalKubeconfig(t *testing.T, contextName string) []byte {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["c"] = &clientcmdapi.Cluster{Server: "https://localhost:6443"}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{}
	cfg.Contexts[contextName] = &clientcmdapi.Context{Cluster: "c", AuthInfo: "u"}
	cfg.CurrentContext = contextName
	data, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatalf("build kubeconfig: %v", err)
	}
	return data
}

// kubeconfigForServer builds a kubeconfig pointing at the given server URL.
func kubeconfigForServer(t *testing.T, serverURL, contextName string) []byte {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["c"] = &clientcmdapi.Cluster{
		Server:                serverURL,
		InsecureSkipTLSVerify: true,
	}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{}
	cfg.Contexts[contextName] = &clientcmdapi.Context{Cluster: "c", AuthInfo: "u"}
	cfg.CurrentContext = contextName
	data, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatalf("build kubeconfig: %v", err)
	}
	return data
}

func TestNewK0smotronInMemoryStore(t *testing.T) {
	s := NewK0smotronInMemoryStore("test-id")
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if got := s.GetKind(); got != types.StoreKindK0smotron {
		t.Errorf("GetKind() = %q, want %q", got, types.StoreKindK0smotron)
	}
	if got := s.GetID(); got != "k0smotron.test-id" {
		t.Errorf("GetID() = %q, want k0smotron.test-id", got)
	}
}

func TestK0smotronInMemoryStore_GetContextPrefix(t *testing.T) {
	s := NewK0smotronInMemoryStore("x")
	if got := s.GetContextPrefix("anything"); got != "k0smotron" {
		t.Errorf("GetContextPrefix() = %q, want k0smotron", got)
	}
}

func TestK0smotronInMemoryStore_StartSearch_IsNoop(t *testing.T) {
	s := NewK0smotronInMemoryStore("x")
	ch := make(chan storetypes.SearchResult, 1)
	s.StartSearch(ch)
	select {
	case r := <-ch:
		t.Errorf("StartSearch sent unexpected result: %+v", r)
	default:
	}
}

func TestK0smotronInMemoryStore_GetKubeconfigForPath(t *testing.T) {
	s := NewK0smotronInMemoryStore("x")
	s.pathToKubeconfig["parent/ns/cluster"] = []byte("kubeconfig-data")

	t.Run("found", func(t *testing.T) {
		data, err := s.GetKubeconfigForPath("parent/ns/cluster", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != "kubeconfig-data" {
			t.Errorf("got %q, want kubeconfig-data", data)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := s.GetKubeconfigForPath("does/not/exist", nil)
		if err == nil {
			t.Fatal("expected error for missing path")
		}
	})
}

func TestK0smotronInMemoryStore_VerifyKubeconfigPaths(t *testing.T) {
	s := NewK0smotronInMemoryStore("x")
	if err := s.VerifyKubeconfigPaths(); err != nil {
		t.Errorf("VerifyKubeconfigPaths() = %v, want nil", err)
	}
}

func TestDiscoverK0smotronClusters_BadKubeconfig(t *testing.T) {
	_, _, err := DiscoverK0smotronClusters("parent", []byte("not-a-kubeconfig"))
	if err == nil {
		t.Error("expected error for malformed kubeconfig")
	}
}

func TestDiscoverK0smotronClusters_UnreachableServer(t *testing.T) {
	kfg := minimalKubeconfig(t, "k0s")
	// must not panic; connection error or graceful no-CRD are both acceptable
	_, _, _ = DiscoverK0smotronClusters("parent", kfg)
}

func TestDiscoverK0smotronClusters_InMemoryLayer(t *testing.T) {
	memStore := NewK0smotronInMemoryStore("test")
	kfg := minimalKubeconfig(t, "k0s")
	memStore.pathToKubeconfig["parent/default/tenant-a"] = kfg
	memStore.pathToKubeconfig["parent/default/tenant-b"] = kfg

	for _, path := range []string{"parent/default/tenant-a", "parent/default/tenant-b"} {
		data, err := memStore.GetKubeconfigForPath(path, nil)
		if err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
		if len(data) == 0 {
			t.Errorf("path %q: empty kubeconfig", path)
		}
	}
}

// fakeK8sServer stands up a minimal fake Kubernetes HTTP server that returns a
// k0smotron ClusterList and the matching kubeconfig Secret.
func fakeK8sServer(t *testing.T, clusterName, namespace string, childKubeconfig []byte) *httptest.Server {
	t.Helper()

	encodedKfg := base64.StdEncoding.EncodeToString(childKubeconfig)

	clusterList := map[string]any{
		"apiVersion": "k0smotron.io/v1beta2",
		"kind":       "ClusterList",
		"metadata":   map[string]any{"resourceVersion": "1"},
		"items": []map[string]any{{
			"apiVersion": "k0smotron.io/v1beta2",
			"kind":       "Cluster",
			"metadata":   map[string]any{"name": clusterName, "namespace": namespace},
		}},
	}
	clusterListJSON, _ := json.Marshal(clusterList)

	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": clusterName + "-kubeconfig", "namespace": namespace},
		"data":       map[string]any{"value": encodedKfg},
	}
	secretJSON, _ := json.Marshal(secret)

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"versions":["v1"]}`)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"secrets","namespaced":true,"kind":"Secret","verbs":["get","list"]}]}`)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"groups":[{"name":"k0smotron.io","versions":[{"groupVersion":"k0smotron.io/v1beta2","version":"v1beta2"}],"preferredVersion":{"groupVersion":"k0smotron.io/v1beta2","version":"v1beta2"}}]}`)
	})
	mux.HandleFunc("/apis/k0smotron.io/v1beta2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"k0smotron.io/v1beta2","resources":[{"name":"clusters","namespaced":true,"kind":"Cluster","verbs":["get","list"]}]}`)
	})
	mux.HandleFunc("/apis/k0smotron.io/v1beta2/clusters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(clusterListJSON)
	})
	mux.HandleFunc("/api/v1/namespaces/"+namespace+"/secrets/"+clusterName+"-kubeconfig",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(secretJSON)
		})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
	})

	return httptest.NewServer(mux)
}

func TestDiscoverK0smotronClusters_WithFakeServer(t *testing.T) {
	childKfg := minimalKubeconfig(t, "k0s")
	srv := fakeK8sServer(t, "tenant-a", "default", childKfg)
	defer srv.Close()

	parentKfg := kubeconfigForServer(t, srv.URL, "parent")
	memStore, entries, err := DiscoverK0smotronClusters("parent", parentKfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Name != "tenant-a" {
		t.Errorf("Name = %q, want tenant-a", e.Name)
	}
	if e.ContextName != "k0s" {
		t.Errorf("ContextName = %q, want k0s", e.ContextName)
	}
	if e.StoreID != memStore.GetID() {
		t.Errorf("StoreID mismatch: %q != %q", e.StoreID, memStore.GetID())
	}
	data, err := memStore.GetKubeconfigForPath(e.Path, nil)
	if err != nil || len(data) == 0 {
		t.Errorf("GetKubeconfigForPath: data=%d err=%v", len(data), err)
	}
}

func TestDiscoverK0smotronClusters_SecretMissingValue(t *testing.T) {
	// Secret exists but has no "value" key — cluster should be skipped.
	clusterName, namespace := "tenant-b", "default"
	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": clusterName + "-kubeconfig", "namespace": namespace},
		"data":       map[string]any{},
	}
	secretJSON, _ := json.Marshal(secret)

	clusterList := map[string]any{
		"apiVersion": "k0smotron.io/v1beta2",
		"kind":       "ClusterList",
		"metadata":   map[string]any{"resourceVersion": "1"},
		"items": []map[string]any{{
			"apiVersion": "k0smotron.io/v1beta2",
			"kind":       "Cluster",
			"metadata":   map[string]any{"name": clusterName, "namespace": namespace},
		}},
	}
	clusterListJSON, _ := json.Marshal(clusterList)

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"versions":["v1"]}`)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"secrets","namespaced":true,"kind":"Secret","verbs":["get","list"]}]}`)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"groups":[{"name":"k0smotron.io","versions":[{"groupVersion":"k0smotron.io/v1beta2","version":"v1beta2"}],"preferredVersion":{"groupVersion":"k0smotron.io/v1beta2","version":"v1beta2"}}]}`)
	})
	mux.HandleFunc("/apis/k0smotron.io/v1beta2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"k0smotron.io/v1beta2","resources":[{"name":"clusters","namespaced":true,"kind":"Cluster","verbs":["get","list"]}]}`)
	})
	mux.HandleFunc("/apis/k0smotron.io/v1beta2/clusters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(clusterListJSON)
	})
	mux.HandleFunc("/api/v1/namespaces/"+namespace+"/secrets/"+clusterName+"-kubeconfig",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(secretJSON)
		})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	parentKfg := kubeconfigForServer(t, srv.URL, "parent")
	_, entries, err := DiscoverK0smotronClusters("parent", parentKfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (secret missing value key), got %d", len(entries))
	}
}

func TestK0smotronGVR(t *testing.T) {
	want := schema.GroupVersionResource{
		Group:    "k0smotron.io",
		Version:  "v1beta2",
		Resource: "clusters",
	}
	if k0smotronClusterGVR != want {
		t.Errorf("GVR = %v, want %v", k0smotronClusterGVR, want)
	}
}

func TestK0smotronClusterEntry(t *testing.T) {
	e := K0smotronClusterEntry{
		ContextName: "k0s",
		StoreID:     "k0smotron.parent",
	}
	if e.ContextName != "k0s" {
		t.Errorf("ContextName = %q, want k0s", e.ContextName)
	}
	if e.StoreID != "k0smotron.parent" {
		t.Errorf("StoreID = %q", e.StoreID)
	}
}
