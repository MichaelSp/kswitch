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

package setcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	kubeconfigutil "github.com/MichaelSp/kswitch/pkg/util/kubectx_copied"
	"github.com/MichaelSp/kswitch/types"
)

// namingStubStore is a store that names its contexts during the search without
// downloading the kubeconfig, like the OVH store does, and that hands out a kubeconfig
// whose context is named independently of that prediction.
type namingStubStore struct {
	path string
	// declared is the context name announced during the search
	declared string
	// actual is the context name the kubeconfig really carries
	actual string
}

func (s *namingStubStore) GetID() string                    { return "ovh" }
func (s *namingStubStore) GetKind() types.StoreKind         { return types.StoreKindOVH }
func (s *namingStubStore) GetContextPrefix(_ string) string { return "ovh" }
func (s *namingStubStore) VerifyKubeconfigPaths() error     { return nil }
func (s *namingStubStore) GetLogger() *logrus.Entry         { return logrus.WithField("store", "ovh") }

func (s *namingStubStore) GetStoreConfig() types.KubeconfigStore {
	id := "ovh"
	return types.KubeconfigStore{Kind: types.StoreKindOVH, ID: &id}
}

func (s *namingStubStore) StartSearch(channel chan storetypes.SearchResult) {
	channel <- storetypes.SearchResult{KubeconfigPath: s.path}
}

func (s *namingStubStore) ContextNamesForPath(_ string, _ map[string]string) []string {
	return []string{s.declared}
}

func (s *namingStubStore) GetKubeconfigForPath(_ string, _ map[string]string) ([]byte, error) {
	return fmt.Appendf(nil, `apiVersion: v1
kind: Config
current-context: ""
contexts:
- name: %s
  context:
    cluster: %s
clusters:
- name: %s
  cluster:
    server: https://%s.example.invalid
`, s.actual, s.path, s.path, s.path), nil
}

// TestSetContext_ContextNamerPrediction covers the selection-time reconciliation: a
// store may announce a context name during the search without downloading the
// kubeconfig, and the switch must still produce a usable current-context when that
// prediction turns out not to match the kubeconfig the provider hands over.
func TestSetContext_ContextNamerPrediction(t *testing.T) {
	tests := []struct {
		name string
		// declared is the context name announced during the search
		declared string
		// actual is the context name the downloaded kubeconfig really carries
		actual             string
		wantCurrentContext string
	}{
		{
			name:               "the prediction matches the kubeconfig",
			declared:           "kubernetes-admin@production",
			actual:             "kubernetes-admin@production",
			wantCurrentContext: "kubernetes-admin@production",
		},
		{
			// without the fallback the current-context would point at a context the
			// kubeconfig does not contain, and kubectl would fail on the next call
			name:               "a diverging prediction falls back to the real context",
			declared:           "kubernetes-admin@production",
			actual:             "admin@production",
			wantCurrentContext: "admin@production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SetContext writes the resulting kubeconfig below $HOME/.kube, and the
			// temporary directory is created non-recursively
			home := t.TempDir()
			t.Setenv("HOME", home)
			if err := os.Mkdir(filepath.Join(home, ".kube"), 0700); err != nil {
				t.Fatalf("failed to create the kube directory: %v", err)
			}

			store := &namingStubStore{path: "production", declared: tt.declared, actual: tt.actual}
			stores := []storetypes.KubeconfigStore{store}

			path, context, err := SetContext("ovh/"+tt.declared, stores, &types.Config{}, t.TempDir(), true, false)
			if err != nil {
				t.Fatalf("SetContext failed: %v", err)
			}
			if context == nil || *context != "ovh/"+tt.declared {
				t.Errorf("SetContext returned context %v, want %q", context, "ovh/"+tt.declared)
			}
			if path == nil {
				t.Fatal("SetContext returned no kubeconfig path")
			}

			written, err := kubeconfigutil.NewKubeconfigForPath(*path)
			if err != nil {
				t.Fatalf("failed to read the written kubeconfig: %v", err)
			}
			if got := written.GetCurrentContext(); got != tt.wantCurrentContext {
				t.Errorf("current-context = %q, want %q", got, tt.wantCurrentContext)
			}
		})
	}
}
