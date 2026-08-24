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
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// storesByID indexes stores the way Switcher does.
func storesByID(stores ...storetypes.KubeconfigStore) map[string]storetypes.KubeconfigStore {
	byID := map[string]storetypes.KubeconfigStore{}
	for _, s := range stores {
		byID[s.GetID()] = s
	}
	return byID
}

// TestResolveStore_PathCollisionAcrossStores is the regression test for
// resolving the selected context's store by identity: both stores expose a
// cluster under the path "prod", so a path-keyed lookup would return whichever
// store was indexed last.
func TestResolveStore_PathCollisionAcrossStores(t *testing.T) {
	gke := &stubStore{
		kind:        types.StoreKindGKE,
		kubeconfigs: map[string][]byte{"prod": []byte("gke-prod")},
	}
	eks := &stubStore{
		kind:        types.StoreKindEKS,
		kubeconfigs: map[string][]byte{"prod": []byte("eks-prod")},
	}
	kindToStore := storesByID(gke, eks)

	tests := []struct {
		storeID string
		want    string
	}{
		{storeID: gke.GetID(), want: "gke-prod"},
		{storeID: eks.GetID(), want: "eks-prod"},
	}

	for _, tt := range tests {
		store, err := resolveStore(kindToStore, tt.storeID, nil)
		if err != nil {
			t.Fatalf("resolveStore(%q) error = %v", tt.storeID, err)
		}

		data, err := store.GetKubeconfigForPath("prod", nil)
		if err != nil {
			t.Fatalf("GetKubeconfigForPath() error = %v", err)
		}
		if string(data) != tt.want {
			t.Errorf("storeID %q returned kubeconfig %q, want %q", tt.storeID, data, tt.want)
		}
	}
}

func TestResolveStore_DynamicStoreTakesPrecedence(t *testing.T) {
	gke := &stubStore{kind: types.StoreKindGKE, kubeconfigs: map[string][]byte{"prod": []byte("gke-prod")}}
	dynamic := &stubStore{
		kind:        types.StoreKindK0smotron,
		kubeconfigs: map[string][]byte{"prod": []byte("k0smotron-child")},
	}

	// Even with a storeID pointing at a regular store, the dynamic (in-memory)
	// k0smotron store wins for sub-cluster selections.
	store, err := resolveStore(storesByID(gke), gke.GetID(), dynamic)
	if err != nil {
		t.Fatalf("resolveStore() error = %v", err)
	}
	if store != dynamic {
		t.Fatalf("resolveStore() = %v, want the dynamic store", store)
	}
}

func TestResolveStore_UnknownStoreID(t *testing.T) {
	gke := &stubStore{kind: types.StoreKindGKE}

	if _, err := resolveStore(storesByID(gke), "vault.default", nil); err == nil {
		t.Error("resolveStore() with an unknown storeID: expected an error, got nil")
	}
}

func TestResolveStore_EmptyStoreID(t *testing.T) {
	if _, err := resolveStore(map[string]storetypes.KubeconfigStore{}, "", nil); err == nil {
		t.Error("resolveStore() with no stores: expected an error, got nil")
	}
}
