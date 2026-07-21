package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	"github.com/sirupsen/logrus"
)

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

// minimalKubeconfig builds a minimal valid kubeconfig pointing at localhost.
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

// ---- stub store ----

type stubStore struct {
	id   string
	data map[string][]byte
}

func (s *stubStore) GetID() string                              { return s.id }
func (s *stubStore) GetKind() types.StoreKind                   { return types.StoreKindK0smotron }
func (s *stubStore) GetContextPrefix(_ string) string           { return "k0smotron" }
func (s *stubStore) VerifyKubeconfigPaths() error               { return nil }
func (s *stubStore) StartSearch(_ chan storetypes.SearchResult) {}
func (s *stubStore) GetLogger() *logrus.Entry                   { return logrus.WithField("store", "stub") }
func (s *stubStore) GetStoreConfig() types.KubeconfigStore      { return types.KubeconfigStore{} }
func (s *stubStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	if b, ok := s.data[path]; ok {
		return b, nil
	}
	return nil, nil
}

// errorStore always errors on GetKubeconfigForPath.
type errorStore struct{ stubStore }

func (e *errorStore) GetKubeconfigForPath(_ string, _ map[string]string) ([]byte, error) {
	return nil, fmt.Errorf("injected error")
}

var errTest = fmt.Errorf("test error")

// ---- markLoading ----

func TestMarkLoading(t *testing.T) {
	m := NewModel(nil, false)
	m.allItems = []item{
		{path: "a", displayName: "A"},
		{path: "b", displayName: "B"},
	}
	m.filtered = m.allItems

	m2 := m.markLoading("a")

	if !m2.allItems[0].isLoading {
		t.Error("expected allItems[0].isLoading = true")
	}
	if m2.allItems[1].isLoading {
		t.Error("expected allItems[1].isLoading = false")
	}
}

// ---- applyExpandResult ----

func TestApplyExpandResult_WithChildren(t *testing.T) {
	m := NewModel(nil, false)
	m.allItems = []item{
		{path: "parent", displayName: "Parent"},
		{path: "other", displayName: "Other"},
	}
	m.filtered = append([]item{}, m.allItems...)

	children := []item{
		{path: "parent/ns/child1", displayName: "child1", parentPath: "parent", depth: 1},
		{path: "parent/ns/child2", displayName: "child2", parentPath: "parent", depth: 1},
	}
	stub := &stubStore{id: "k0smotron.parent"}
	msg := expandResultMsg{parentPath: "parent", children: children, store: stub}

	m2 := m.applyExpandResult(msg)

	var parent item
	for _, it := range m2.allItems {
		if it.path == "parent" {
			parent = it
		}
	}
	if !parent.expanded {
		t.Error("expected parent.expanded = true")
	}
	if len(m2.allItems) != 4 {
		t.Fatalf("expected 4 items, got %d", len(m2.allItems))
	}
	// children before parent in allItems (fzf bottom-to-top: parent renders above)
	if m2.allItems[0].path != "parent/ns/child1" {
		t.Errorf("allItems[0] = %q, want child1", m2.allItems[0].path)
	}
	if m2.allItems[2].path != "parent" {
		t.Errorf("allItems[2] = %q, want parent", m2.allItems[2].path)
	}
	if _, ok := m2.dynamicStores["k0smotron.parent"]; !ok {
		t.Error("expected dynamic store to be registered")
	}
}

func TestApplyExpandResult_NoChildren(t *testing.T) {
	m := NewModel(nil, false)
	m.allItems = []item{{path: "parent", displayName: "Parent"}}
	m.filtered = append([]item{}, m.allItems...)

	m2 := m.applyExpandResult(expandResultMsg{parentPath: "parent"})

	if m2.allItems[0].expanded {
		t.Error("expected expanded = false when no children")
	}
	if !m2.allItems[0].expandedEmpty {
		t.Error("expected expandedEmpty = true when no children")
	}
}

func TestApplyExpandResult_WithError(t *testing.T) {
	m := NewModel(nil, false)
	m.allItems = []item{{path: "parent", displayName: "Parent"}}
	m.filtered = append([]item{}, m.allItems...)

	m2 := m.applyExpandResult(expandResultMsg{parentPath: "parent", err: errTest})

	if m2.allItems[0].expanded || m2.allItems[0].expandedEmpty {
		t.Error("expected no expand state on error")
	}
	if len(m2.allItems) != 1 {
		t.Errorf("expected 1 item, got %d", len(m2.allItems))
	}
}

// ---- collapseItem ----

func TestCollapseItem(t *testing.T) {
	m := NewModel(nil, false)
	m.allItems = []item{
		{path: "parent/ns/child1", parentPath: "parent", depth: 1},
		{path: "parent/ns/child2", parentPath: "parent", depth: 1},
		{path: "parent", expanded: true},
		{path: "other"},
	}
	m.filtered = append([]item{}, m.allItems...)

	m2 := m.collapseItem("parent")

	if len(m2.allItems) != 2 {
		t.Fatalf("expected 2 items after collapse, got %d", len(m2.allItems))
	}
	if m2.allItems[0].path != "parent" {
		t.Errorf("allItems[0] = %q, want parent", m2.allItems[0].path)
	}
	if m2.allItems[0].expanded {
		t.Error("expected parent.expanded = false after collapse")
	}
	if m2.allItems[1].path != "other" {
		t.Errorf("allItems[1] = %q, want other", m2.allItems[1].path)
	}
}

func TestCollapseItem_Recursive(t *testing.T) {
	m := NewModel(nil, false)
	m.allItems = []item{
		{path: "p/ns/child", parentPath: "parent", depth: 1, expanded: true},
		{path: "p/ns/child/ns/gc", parentPath: "p/ns/child", depth: 2},
		{path: "parent", expanded: true},
	}
	m.filtered = append([]item{}, m.allItems...)

	m2 := m.collapseItem("parent")

	if len(m2.allItems) != 1 || m2.allItems[0].path != "parent" {
		t.Errorf("expected only parent after recursive collapse, got %v", m2.allItems)
	}
}

// ---- dynamicStoreAdapter ----

func TestDynamicStoreAdapter(t *testing.T) {
	inner := &stubStore{id: "k0smotron.x", data: map[string][]byte{"path": []byte("cfg")}}
	a := &dynamicStoreAdapter{inner: inner}

	if a.GetID() != "k0smotron.x" {
		t.Errorf("GetID() = %q", a.GetID())
	}
	if a.GetKind() != types.StoreKindK0smotron {
		t.Errorf("GetKind() = %q", a.GetKind())
	}
	if a.GetContextPrefix("x") != "k0smotron" {
		t.Errorf("GetContextPrefix() = %q", a.GetContextPrefix("x"))
	}
	if err := a.VerifyKubeconfigPaths(); err != nil {
		t.Errorf("VerifyKubeconfigPaths() = %v", err)
	}
	a.StartSearch(nil)
	if a.GetLogger() == nil {
		t.Error("GetLogger() = nil")
	}
	if cfg := a.GetStoreConfig(); cfg.Kind != types.StoreKindK0smotron {
		t.Errorf("GetStoreConfig kind = %q, want k0smotron", cfg.Kind)
	}
	data, err := a.GetKubeconfigForPath("path", nil)
	if err != nil || string(data) != "cfg" {
		t.Errorf("GetKubeconfigForPath() = %q, %v", data, err)
	}
}

// ---- expandK0smotronCmd ----

func TestExpandK0smotronCmd_NoStore(t *testing.T) {
	cmd := expandK0smotronCmd(nil, nil, item{path: "p", storeID: "missing"})
	r, ok := cmd().(expandResultMsg)
	if !ok {
		t.Fatal("expected expandResultMsg")
	}
	if r.err != nil || r.children != nil {
		t.Errorf("expected empty result, got err=%v children=%v", r.err, r.children)
	}
}

func TestExpandK0smotronCmd_StoreError(t *testing.T) {
	stores := map[string]storetypes.KubeconfigStore{"s": &errorStore{}}
	cmd := expandK0smotronCmd(stores, nil, item{path: "p", storeID: "s"})
	r, ok := cmd().(expandResultMsg)
	if !ok {
		t.Fatal("expected expandResultMsg")
	}
	if r.err == nil {
		t.Error("expected error from store")
	}
}

func TestExpandK0smotronCmd_ValidKubeconfig(t *testing.T) {
	// Store returns a valid kubeconfig; DiscoverK0smotronClusters will fail
	// to connect (no real server) but must not panic.
	kfg := minimalKubeconfig(t, "k0s")
	stores := map[string]storetypes.KubeconfigStore{
		"s": &stubStore{id: "s", data: map[string][]byte{"p": kfg}},
	}
	cmd := expandK0smotronCmd(stores, nil, item{path: "p", storeID: "s"})
	if _, ok := cmd().(expandResultMsg); !ok {
		t.Fatal("expected expandResultMsg")
	}
}

func TestExpandK0smotronCmd_EmptyClusterList(t *testing.T) {
	// Build a fake server that returns an empty ClusterList so DiscoverK0smotronClusters
	// returns (store, nil, nil) — exercises the len(entries)==0 early-return.
	mux := http.NewServeMux()
	emptyList := `{"apiVersion":"k0smotron.io/v1beta2","kind":"ClusterList","metadata":{},"items":[]}`
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"versions":["v1"]}`)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[]}`)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"groups":[{"name":"k0smotron.io","versions":[{"groupVersion":"k0smotron.io/v1beta2","version":"v1beta2"}],"preferredVersion":{"groupVersion":"k0smotron.io/v1beta2","version":"v1beta2"}}]}`)
	})
	mux.HandleFunc("/apis/k0smotron.io/v1beta2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"k0smotron.io/v1beta2","resources":[{"name":"clusters","namespaced":true,"kind":"Cluster","verbs":["get","list"]}]}`)
	})
	mux.HandleFunc("/apis/k0smotron.io/v1beta2/clusters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, emptyList)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	kfg := kubeconfigForServer(t, srv.URL, "ctx")
	stores := map[string]storetypes.KubeconfigStore{
		"s": &stubStore{id: "s", data: map[string][]byte{"p": kfg}},
	}
	cmd := expandK0smotronCmd(stores, nil, item{path: "p", storeID: "s"})
	r, ok := cmd().(expandResultMsg)
	if !ok {
		t.Fatal("expected expandResultMsg")
	}
	if r.err != nil {
		t.Errorf("unexpected error: %v", r.err)
	}
	if len(r.children) != 0 {
		t.Errorf("expected 0 children, got %d", len(r.children))
	}
}
