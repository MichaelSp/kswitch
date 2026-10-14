// Copyright 2026 The Kswitch authors
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
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// kubernetesStoreWithClient constructs a KubernetesStore and replaces its
// buildClient with one backed by the given fake.Clientset.
// It uses the injection hook below so the real field stays unexported.
func newKubernetesStoreForTest(t *testing.T, cfg types.StoreConfigKubernetes) (*KubernetesStore, *fake.Clientset) {
	t.Helper()
	s, err := NewKubernetesStore(types.KubeconfigStore{
		Kind:   types.StoreKindKubernetes,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("NewKubernetesStore: %v", err)
	}
	fakeClient := fake.NewSimpleClientset()
	s.clientOverride = fakeClient
	return s, fakeClient
}

// --- constructor validation -------------------------------------------------

func TestNewKubernetesStore_MissingSelector(t *testing.T) {
	_, err := NewKubernetesStore(types.KubeconfigStore{
		Kind:   types.StoreKindKubernetes,
		Config: types.StoreConfigKubernetes{},
	})
	if err == nil {
		t.Fatal("expected error when neither secretName nor secretLabelSelector is set")
	}
}

func TestNewKubernetesStore_BothSelectorAndName(t *testing.T) {
	_, err := NewKubernetesStore(types.KubeconfigStore{
		Kind: types.StoreKindKubernetes,
		Config: types.StoreConfigKubernetes{
			SecretName:          "my-secret",
			SecretLabelSelector: "app=foo",
		},
	})
	if err == nil {
		t.Fatal("expected error when both secretName and secretLabelSelector are set")
	}
}

func TestNewKubernetesStore_Defaults(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		SecretName: "my-secret",
	})
	if s.Config.Namespace != "default" {
		t.Errorf("namespace default expected, got %q", s.Config.Namespace)
	}
	if s.Config.Path != "kubeconfig" {
		t.Errorf("path default expected, got %q", s.Config.Path)
	}
}

// --- GetContextPrefix -------------------------------------------------------

func TestKubernetesStore_GetContextPrefix(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{SecretName: "x"})
	if s.GetContextPrefix("") != "kubernetes" {
		t.Errorf("unexpected prefix %q", s.GetContextPrefix(""))
	}
}

func TestKubernetesStore_GetContextPrefix_ShowPrefixFalse(t *testing.T) {
	f := false
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{SecretName: "x"})
	s.KubeconfigStore.ShowPrefix = &f
	if s.GetContextPrefix("") != "" {
		t.Errorf("expected empty prefix when ShowPrefix=false")
	}
}

// --- StartSearch ------------------------------------------------------------

const sampleKubeconfig = `apiVersion: v1
kind: Config
clusters: []
users: []
contexts: []
current-context: ""
`

func TestKubernetesStore_StartSearch_ByName(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:  "kube-system",
		SecretName: "my-kubeconfig",
	})

	_, err := fc.CoreV1().Secrets("kube-system").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "my-kubeconfig"},
		Data:       map[string][]byte{"kubeconfig": []byte(sampleKubeconfig)},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var results []storetypes.SearchResult
	for r := range ch {
		results = append(results, r)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if results[0].KubeconfigPath != "kube-system/my-kubeconfig" {
		t.Errorf("unexpected path %q", results[0].KubeconfigPath)
	}
}

func TestKubernetesStore_StartSearch_ByLabelSelector(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:           "prod",
		SecretLabelSelector: "app=myapp",
	})

	for _, name := range []string{"cluster-a", "cluster-b"} {
		_, err := fc.CoreV1().Secrets("prod").Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "prod",
				Name:      name,
				Labels:    map[string]string{"app": "myapp"},
			},
			Data: map[string][]byte{"kubeconfig": []byte(sampleKubeconfig)},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var results []storetypes.SearchResult
	for r := range ch {
		if r.Error != nil {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		results = append(results, r)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestKubernetesStore_StartSearch_MissingKey_Skipped(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:  "default",
		SecretName: "no-such-key-secret",
	})

	_, err := fc.CoreV1().Secrets("default").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "no-such-key-secret"},
		Data:       map[string][]byte{"other-key": []byte("data")},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	for r := range ch {
		if r.Error != nil {
			t.Fatalf("unexpected error for missing key: %v", r.Error)
		}
		t.Errorf("expected no results, got path %q", r.KubeconfigPath)
	}
}

func TestKubernetesStore_StartSearch_AliasesInTags(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:  "default",
		SecretName: "aliased-secret",
		Alias:      types.KubernetesAliasConfig{List: []string{"prod", "production"}},
	})

	_, err := fc.CoreV1().Secrets("default").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "aliased-secret"},
		Data:       map[string][]byte{"kubeconfig": []byte(sampleKubeconfig)},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var result *storetypes.SearchResult
	for r := range ch {
		rr := r
		result = &rr
	}
	if result == nil {
		t.Fatal("expected one result")
	}
	if result.Tags["aliases"] != "prod,production" {
		t.Errorf("unexpected aliases tag %q", result.Tags["aliases"])
	}
}

// --- GetKubeconfigForPath ---------------------------------------------------

func TestKubernetesStore_GetKubeconfig_FullKubeconfig(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:  "default",
		SecretName: "my-kc",
	})

	_, err := fc.CoreV1().Secrets("default").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "my-kc"},
		Data:       map[string][]byte{"kubeconfig": []byte(sampleKubeconfig)},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.GetKubeconfigForPath("default/my-kc", map[string]string{
		"namespace":  "default",
		"secretName": "my-kc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "apiVersion:") {
		t.Error("expected kubeconfig to contain 'apiVersion:'")
	}
}

func TestKubernetesStore_GetKubeconfig_BearerToken(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:  "default",
		SecretName: "my-token",
		Path:       "token",
	})

	_, err := fc.CoreV1().Secrets("default").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "my-token"},
		Data:       map[string][]byte{"token": []byte("my-bearer-token-value")},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.GetKubeconfigForPath("default/my-token", map[string]string{
		"namespace":  "default",
		"secretName": "my-token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "apiVersion:") {
		t.Error("synthesised kubeconfig must contain apiVersion:")
	}
	if !strings.Contains(string(data), "my-bearer-token-value") {
		t.Error("synthesised kubeconfig must contain the token value")
	}
}

func TestKubernetesStore_GetKubeconfig_PathParseFallback(t *testing.T) {
	s, fc := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		Namespace:  "default",
		SecretName: "fallback-secret",
	})

	_, err := fc.CoreV1().Secrets("ns1").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "secret1"},
		Data:       map[string][]byte{"kubeconfig": []byte(sampleKubeconfig)},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Pass empty tags — store must parse namespace/name from path.
	data, err := s.GetKubeconfigForPath("ns1/secret1", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "apiVersion:") {
		t.Error("expected kubeconfig to contain 'apiVersion:'")
	}
}

// --- helpers ----------------------------------------------------------------

func TestSecretPath_RoundTrip(t *testing.T) {
	ns, name := "my-ns", "my-secret"
	path := secretPath(ns, name)
	gotNs, gotName, err := parseSecretPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotNs != ns || gotName != name {
		t.Errorf("round-trip failed: got %q/%q", gotNs, gotName)
	}
}

func TestParseSecretPath_Invalid(t *testing.T) {
	for _, bad := range []string{"", "noSlash", "/noname", "nons/"} {
		_, _, err := parseSecretPath(bad)
		if err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestLooksLikeKubeconfig(t *testing.T) {
	if !looksLikeKubeconfig([]byte(sampleKubeconfig)) {
		t.Error("sampleKubeconfig should be detected as kubeconfig")
	}
	if looksLikeKubeconfig([]byte("some-bearer-token")) {
		t.Error("plain token should not be detected as kubeconfig")
	}
}

// --- parseStoreRef ----------------------------------------------------------

func TestParseStoreRef_Valid(t *testing.T) {
	storeID, contextPath, err := parseStoreRef("store://gke.eu-prod/europe-west1--my-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storeID != "gke.eu-prod" {
		t.Errorf("storeID: got %q, want %q", storeID, "gke.eu-prod")
	}
	if contextPath != "europe-west1--my-cluster" {
		t.Errorf("contextPath: got %q, want %q", contextPath, "europe-west1--my-cluster")
	}
}

func TestParseStoreRef_PathWithSlash(t *testing.T) {
	// context paths from some stores (e.g. kubernetes store itself) contain "/"
	storeID, contextPath, err := parseStoreRef("store://filesystem.default/ns/secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storeID != "filesystem.default" {
		t.Errorf("storeID: got %q", storeID)
	}
	if contextPath != "ns/secret" {
		t.Errorf("contextPath: got %q", contextPath)
	}
}

func TestParseStoreRef_Invalid(t *testing.T) {
	cases := []string{
		"",
		"gke.eu-prod",           // missing prefix
		"store://",              // no store-id
		"store://gke.eu-prod",   // no slash → no context-path
		"store:///context-path", // empty store-id
		"store://gke.eu-prod/",  // empty context-path
	}
	for _, ref := range cases {
		_, _, err := parseStoreRef(ref)
		if err == nil {
			t.Errorf("expected error for %q", ref)
		}
	}
}

// --- ResolvePeerStores ------------------------------------------------------

// stubStoreForPeer is a minimal KubeconfigStore that returns a fixed kubeconfig.
type stubStoreForPeer struct {
	KubernetesStore // embed to satisfy interface via BaseStore
	id              string
	kubeconfig      []byte
}

func (s *stubStoreForPeer) GetID() string { return s.id }
func (s *stubStoreForPeer) GetKubeconfigForPath(_ string, _ map[string]string) ([]byte, error) {
	return s.kubeconfig, nil
}

func TestResolvePeerStores_NoStoreID(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{SecretName: "x"})
	// StoreID is empty — must be a no-op with no error.
	if err := s.ResolvePeerStores(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.peerKubeconfig != nil {
		t.Error("peerKubeconfig should remain nil when storeID is empty")
	}
}

func TestResolvePeerStores_PeerFound(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		SecretName: "x",
		StoreID:    "store://filesystem.default/some/path",
	})

	peer := &stubStoreForPeer{id: "filesystem.default", kubeconfig: []byte(sampleKubeconfig)}
	allStores := map[string]storetypes.KubeconfigStore{"filesystem.default": peer}

	if err := s.ResolvePeerStores(allStores); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(s.peerKubeconfig) != sampleKubeconfig {
		t.Error("peerKubeconfig not set to peer store's kubeconfig")
	}
}

func TestResolvePeerStores_PeerNotFound(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		SecretName: "x",
		StoreID:    "store://gke.eu-prod/cluster",
	})
	if err := s.ResolvePeerStores(map[string]storetypes.KubeconfigStore{}); err == nil {
		t.Fatal("expected error when peer store is not found")
	}
}

func TestResolvePeerStores_InvalidFormat(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		SecretName: "x",
		StoreID:    "not-a-valid-ref",
	})
	if err := s.ResolvePeerStores(nil); err == nil {
		t.Fatal("expected error for invalid storeID format")
	}
}

// --- ResolvePeerStoresForAll ------------------------------------------------

func TestResolvePeerStoresForAll_SkipsNonKubernetesStores(t *testing.T) {
	// All non-kubernetes stores should be silently skipped.
	kindStore, err := NewKindStore(types.KubeconfigStore{Kind: types.StoreKindKind})
	if err != nil {
		t.Fatal(err)
	}
	if err := ResolvePeerStoresForAll([]storetypes.KubeconfigStore{kindStore}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolvePeerStoresForAll_ResolvesWrappedStore(t *testing.T) {
	s, _ := newKubernetesStoreForTest(t, types.StoreConfigKubernetes{
		SecretName: "x",
		StoreID:    "store://filesystem.default/some/path",
	})

	peer := &stubStoreForPeer{id: "filesystem.default", kubeconfig: []byte(sampleKubeconfig)}

	// Wrap the kubernetes store in a thin wrapper that implements Unwrap().
	wrapped := &unwrappableStore{inner: s, id: s.GetID()}

	stores := []storetypes.KubeconfigStore{peer, wrapped}
	if err := ResolvePeerStoresForAll(stores); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(s.peerKubeconfig) != sampleKubeconfig {
		t.Error("peerKubeconfig not populated through wrapper")
	}
}

// unwrappableStore simulates a cache wrapper.
type unwrappableStore struct {
	inner storetypes.KubeconfigStore
	id    string
}

func (u *unwrappableStore) GetID() string                               { return u.id }
func (u *unwrappableStore) GetKind() types.StoreKind                    { return u.inner.GetKind() }
func (u *unwrappableStore) GetContextPrefix(p string) string            { return u.inner.GetContextPrefix(p) }
func (u *unwrappableStore) VerifyKubeconfigPaths() error                { return nil }
func (u *unwrappableStore) StartSearch(ch chan storetypes.SearchResult) {}
func (u *unwrappableStore) GetKubeconfigForPath(p string, t map[string]string) ([]byte, error) {
	return u.inner.GetKubeconfigForPath(p, t)
}
func (u *unwrappableStore) GetLogger() *logrus.Entry              { return u.inner.GetLogger() }
func (u *unwrappableStore) GetStoreConfig() types.KubeconfigStore { return u.inner.GetStoreConfig() }
func (u *unwrappableStore) Unwrap() storetypes.KubeconfigStore    { return u.inner }
