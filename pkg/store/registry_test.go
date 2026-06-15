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
	"slices"
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// TestRegisteredKindsMatchValidStoreKinds guards against drift between the
// registry (source of truth for instantiation) and types.ValidStoreKinds
// (source of truth for validation). Adding a store kind to one but not the
// other would be caught here.
func TestRegisteredKindsMatchValidStoreKinds(t *testing.T) {
	registered := RegisteredKinds()       // sorted
	valid := types.ValidStoreKinds.List() // sorted

	if !slices.Equal(registered, valid) {
		t.Errorf("registry and ValidStoreKinds diverge:\n registered = %v\n valid      = %v", registered, valid)
	}
}

func TestCreate_UnknownKind(t *testing.T) {
	if _, err := Create(types.KubeconfigStore{Kind: types.StoreKind("does-not-exist")}, Dependencies{}); err == nil {
		t.Fatal("expected an error for an unknown store kind")
	}
}

func TestCreate_Dispatch(t *testing.T) {
	// filesystem is the only store whose constructor needs neither credentials
	// nor network, so it is the safe choice to exercise registry dispatch.
	s, err := Create(types.KubeconfigStore{Kind: types.StoreKindFilesystem}, Dependencies{KubeconfigName: "config"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected a store, got nil")
	}
	if s.GetKind() != types.StoreKindFilesystem {
		t.Errorf("GetKind() = %q, want filesystem", s.GetKind())
	}
}

// TestCreate_NilOptOut verifies that when a factory opts out by returning an
// untyped (nil, nil), Create propagates a true nil interface — not a
// typed-nil-in-interface that would compare != nil at the call site (the
// DigitalOcean opt-out relies on this).
func TestCreate_NilOptOut(t *testing.T) {
	const kind = types.StoreKind("test-optout-kind")

	Register(kind, func(_ types.KubeconfigStore, _ Dependencies) (storetypes.KubeconfigStore, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		registryMu.Lock()
		delete(registry, kind)
		registryMu.Unlock()
	})

	s, err := Create(types.KubeconfigStore{Kind: kind}, Dependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil {
		t.Fatal("expected a true nil store for an opt-out factory")
	}
}
