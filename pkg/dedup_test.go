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

package pkg

import (
	"fmt"
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// stubStore is a minimal KubeconfigStore for testing.
type stubStore struct {
	kind        types.StoreKind
	kubeconfigs map[string][]byte // path → kubeconfig bytes
}

func (s *stubStore) GetID() string                              { return string(s.kind) + ".default" }
func (s *stubStore) GetKind() types.StoreKind                   { return s.kind }
func (s *stubStore) GetContextPrefix(_ string) string           { return string(s.kind) }
func (s *stubStore) VerifyKubeconfigPaths() error               { return nil }
func (s *stubStore) StartSearch(_ chan storetypes.SearchResult) {}
func (s *stubStore) GetLogger() *logrus.Entry {
	return logrus.WithField("store", s.kind)
}
func (s *stubStore) GetStoreConfig() types.KubeconfigStore {
	return types.KubeconfigStore{Kind: s.kind}
}
func (s *stubStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	if b, ok := s.kubeconfigs[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("not found: %s", path)
}

func makeKubeconfig(t *testing.T, server, ca, contextName string) []byte {
	t.Helper()
	kc := types.KubeConfig{
		TypeMeta:       types.TypeMeta{APIVersion: "v1", Kind: "Config"},
		CurrentContext: contextName,
		Contexts: []types.KubeContext{
			{Name: contextName, Context: types.Context{Cluster: contextName}},
		},
		Clusters: []types.KubeCluster{
			{Name: contextName, Cluster: types.Cluster{Server: server, CertificateAuthorityData: ca}},
		},
	}
	b, err := yaml.Marshal(kc)
	if err != nil {
		t.Fatalf("marshal kubeconfig: %v", err)
	}
	return b
}

func kindStore(kubeconfigs map[string][]byte) storetypes.KubeconfigStore {
	return &stubStore{kind: types.StoreKindKind, kubeconfigs: kubeconfigs}
}

func fsStore(kubeconfigs map[string][]byte) storetypes.KubeconfigStore {
	return &stubStore{kind: types.StoreKindFilesystem, kubeconfigs: kubeconfigs}
}

func dc(store storetypes.KubeconfigStore, path, name string) DiscoveredContext {
	return DiscoveredContext{Store: &store, Path: path, Name: name}
}

func contextNames(contexts []DiscoveredContext) []string {
	names := make([]string, len(contexts))
	for i, c := range contexts {
		names[i] = c.Name
	}
	return names
}

func containsName(contexts []DiscoveredContext, name string) bool {
	for _, c := range contexts {
		if c.Name == name {
			return true
		}
	}
	return false
}

func TestDeduplicateKindContexts_NoDuplicate(t *testing.T) {
	kc := makeKubeconfig(t, "https://127.0.0.1:6443", "ca-abc", "my-cluster")
	fs := fsStore(map[string][]byte{"some-other": makeKubeconfig(t, "https://10.0.0.1:6443", "ca-xyz", "other")})
	kind := kindStore(map[string][]byte{"my-cluster": kc})

	contexts := []DiscoveredContext{
		dc(fs, "some-other", "other"),
		dc(kind, "my-cluster", "kind/my-cluster"),
	}

	result := filterKindDuplicates(contexts)
	if len(result) != 2 {
		t.Errorf("expected 2 contexts, got %d: %v", len(result), contextNames(result))
	}
}

func TestDeduplicateKindContexts_DropFilesystemDuplicate(t *testing.T) {
	const server = "https://127.0.0.1:6443"
	const ca = "ca-data-abc"
	kc := makeKubeconfig(t, server, ca, "my-cluster")

	kind := kindStore(map[string][]byte{"my-cluster": kc})
	fs := fsStore(map[string][]byte{"~/.kube/config": kc}) // same cluster, different path

	contexts := []DiscoveredContext{
		dc(fs, "~/.kube/config", "my-cluster"),    // filesystem copy — should be dropped
		dc(kind, "my-cluster", "kind/my-cluster"), // kind copy — should survive
	}

	result := filterKindDuplicates(contexts)
	if len(result) != 1 {
		t.Fatalf("expected 1 context, got %d: %v", len(result), contextNames(result))
	}
	if result[0].Name != "kind/my-cluster" {
		t.Errorf("expected kind/my-cluster, got %q", result[0].Name)
	}
}

func TestDeduplicateKindContexts_KindOrderDoesNotMatter(t *testing.T) {
	const server = "https://127.0.0.1:6443"
	const ca = "ca-data-abc"
	kc := makeKubeconfig(t, server, ca, "my-cluster")

	kind := kindStore(map[string][]byte{"my-cluster": kc})
	fs := fsStore(map[string][]byte{"~/.kube/config": kc})

	// filesystem entry comes AFTER kind entry — should still be dropped
	contexts := []DiscoveredContext{
		dc(kind, "my-cluster", "kind/my-cluster"),
		dc(fs, "~/.kube/config", "my-cluster"),
	}

	result := filterKindDuplicates(contexts)
	if len(result) != 1 {
		t.Fatalf("expected 1 context, got %d: %v", len(result), contextNames(result))
	}
	if result[0].Name != "kind/my-cluster" {
		t.Errorf("expected kind/my-cluster, got %q", result[0].Name)
	}
}

func TestDeduplicateKindContexts_NoKindStore(t *testing.T) {
	fs := fsStore(map[string][]byte{
		"a": makeKubeconfig(t, "https://10.0.0.1:6443", "ca-1", "cluster-a"),
		"b": makeKubeconfig(t, "https://10.0.0.2:6443", "ca-2", "cluster-b"),
	})

	contexts := []DiscoveredContext{
		dc(fs, "a", "cluster-a"),
		dc(fs, "b", "cluster-b"),
	}

	result := filterKindDuplicates(contexts)
	if len(result) != 2 {
		t.Errorf("expected 2 contexts, got %d: %v", len(result), contextNames(result))
	}
}

func TestDeduplicateKindContexts_MultipleKindClusters(t *testing.T) {
	kcA := makeKubeconfig(t, "https://127.0.0.1:6443", "ca-a", "cluster-a")
	kcB := makeKubeconfig(t, "https://127.0.0.1:6444", "ca-b", "cluster-b")
	kcC := makeKubeconfig(t, "https://10.0.0.1:6443", "ca-c", "cluster-c")

	kind := kindStore(map[string][]byte{
		"cluster-a": kcA,
		"cluster-b": kcB,
	})
	fs := fsStore(map[string][]byte{
		"kube-a": kcA, // dup of cluster-a
		"kube-b": kcB, // dup of cluster-b
		"kube-c": kcC, // not a dup
	})

	contexts := []DiscoveredContext{
		dc(fs, "kube-a", "cluster-a"),
		dc(fs, "kube-b", "cluster-b"),
		dc(fs, "kube-c", "cluster-c"),
		dc(kind, "cluster-a", "kind/cluster-a"),
		dc(kind, "cluster-b", "kind/cluster-b"),
	}

	result := filterKindDuplicates(contexts)
	if len(result) != 3 {
		t.Fatalf("expected 3 contexts, got %d: %v", len(result), contextNames(result))
	}
	for _, want := range []string{"cluster-c", "kind/cluster-a", "kind/cluster-b"} {
		if !containsName(result, want) {
			t.Errorf("expected %q in result, got %v", want, contextNames(result))
		}
	}
}

func TestDeduplicateKindContexts_KubeconfigFetchError(t *testing.T) {
	// kind store can't fetch kubeconfig — filesystem entry should survive
	kind := kindStore(map[string][]byte{}) // empty, will error on fetch
	fs := fsStore(map[string][]byte{
		"some-path": makeKubeconfig(t, "https://127.0.0.1:6443", "ca-abc", "my-cluster"),
	})

	contexts := []DiscoveredContext{
		dc(fs, "some-path", "my-cluster"),
		dc(kind, "missing", "kind/my-cluster"),
	}

	result := filterKindDuplicates(contexts)
	if len(result) != 2 {
		t.Errorf("expected 2 contexts, got %d: %v", len(result), contextNames(result))
	}
}

// --- filterPreferExternal tests ---

func TestFilterPreferExternal_KeepsExternalOnly(t *testing.T) {
	fs := fsStore(map[string][]byte{})
	contexts := []DiscoveredContext{
		dc(fs, "path/a", "ctx-virtualservice-0"),
		dc(fs, "path/a", "ctx-virtualservice-c"),
		dc(fs, "path/a", "ctx-external"),
	}

	result := filterPreferExternal(contexts)
	if len(result) != 1 {
		t.Fatalf("expected 1 context, got %d: %v", len(result), contextNames(result))
	}
	if result[0].Name != "ctx-external" {
		t.Errorf("expected ctx-external, got %q", result[0].Name)
	}
}

func TestFilterPreferExternal_FallbackWhenNoExternal(t *testing.T) {
	fs := fsStore(map[string][]byte{})
	contexts := []DiscoveredContext{
		dc(fs, "path/a", "ctx-virtualservice-0"),
		dc(fs, "path/a", "ctx-virtualservice-c"),
	}

	result := filterPreferExternal(contexts)
	if len(result) != 2 {
		t.Fatalf("expected 2 contexts, got %d: %v", len(result), contextNames(result))
	}
}

func TestFilterPreferExternal_IndependentPerPath(t *testing.T) {
	fs := fsStore(map[string][]byte{})
	contexts := []DiscoveredContext{
		// path/a has an external — keep only it
		dc(fs, "path/a", "a-virtualservice"),
		dc(fs, "path/a", "a-external"),
		// path/b has no external — keep all
		dc(fs, "path/b", "b-virtualservice-0"),
		dc(fs, "path/b", "b-virtualservice-1"),
	}

	result := filterPreferExternal(contexts)
	if len(result) != 3 {
		t.Fatalf("expected 3 contexts, got %d: %v", len(result), contextNames(result))
	}
	if !containsName(result, "a-external") {
		t.Error("expected a-external")
	}
	if containsName(result, "a-virtualservice") {
		t.Error("a-virtualservice should have been filtered out")
	}
	if !containsName(result, "b-virtualservice-0") || !containsName(result, "b-virtualservice-1") {
		t.Error("b contexts should survive unchanged")
	}
}

func TestFilterPreferExternal_MultipleExternalSamePathAllKept(t *testing.T) {
	fs := fsStore(map[string][]byte{})
	contexts := []DiscoveredContext{
		dc(fs, "path/a", "ctx-external"),
		dc(fs, "path/a", "ctx2-external"),
		dc(fs, "path/a", "ctx-virtualservice"),
	}

	result := filterPreferExternal(contexts)
	if len(result) != 2 {
		t.Fatalf("expected 2 contexts, got %d: %v", len(result), contextNames(result))
	}
	for _, want := range []string{"ctx-external", "ctx2-external"} {
		if !containsName(result, want) {
			t.Errorf("expected %q in result", want)
		}
	}
}
