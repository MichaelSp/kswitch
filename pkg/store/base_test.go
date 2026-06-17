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
	"testing"

	"github.com/MichaelSp/kswitch/types"
)

func TestBaseStore_GetID(t *testing.T) {
	tests := []struct {
		name string
		kind types.StoreKind
		id   *string
		want string
	}{
		{"no id falls back to default", types.StoreKindEKS, nil, "eks.default"},
		{"explicit id is used", types.StoreKindEKS, new("prod"), "eks.prod"},
		{"different kind", types.StoreKindVault, nil, "vault.default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBaseStore(tt.kind, types.KubeconfigStore{ID: tt.id})
			if got := b.GetID(); got != tt.want {
				t.Errorf("GetID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseStore_GetKind(t *testing.T) {
	b := NewBaseStore(types.StoreKindGKE, types.KubeconfigStore{})
	if got := b.GetKind(); got != types.StoreKindGKE {
		t.Errorf("GetKind() = %q, want %q", got, types.StoreKindGKE)
	}
}

func TestParseStoreConfig(t *testing.T) {
	t.Run("nil config returns a usable empty struct", func(t *testing.T) {
		cfg, err := ParseStoreConfig[types.StoreConfigEKS](types.KubeconfigStore{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected a non-nil config")
		}
		if cfg.Region != nil || cfg.Profile != "" {
			t.Errorf("expected zero-value config, got %+v", cfg)
		}
	})

	t.Run("valid config round-trips into the typed struct", func(t *testing.T) {
		store := types.KubeconfigStore{Config: map[string]any{"region": "eu-west-1", "profile": "myprofile"}}
		cfg, err := ParseStoreConfig[types.StoreConfigEKS](store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Region == nil || *cfg.Region != "eu-west-1" {
			t.Errorf("Region = %v, want eu-west-1", cfg.Region)
		}
		if cfg.Profile != "myprofile" {
			t.Errorf("Profile = %q, want myprofile", cfg.Profile)
		}
	})

	t.Run("malformed config returns an error", func(t *testing.T) {
		// profile is a string field; feeding it a mapping makes the YAML decode fail
		store := types.KubeconfigStore{Config: map[string]any{"profile": map[string]any{"oops": true}}}
		if _, err := ParseStoreConfig[types.StoreConfigEKS](store); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

// TestGetContextPrefix_ShowPrefix locks the behaviour of the showPrefix option,
// including the two stores (capi, akamai) that previously ignored it.
func TestGetContextPrefix_ShowPrefix(t *testing.T) {
	hidden := types.KubeconfigStore{ShowPrefix: new(false)}

	t.Run("capi honours showPrefix=false", func(t *testing.T) {
		s := &CapiStore{BaseStore: NewBaseStore(types.StoreKindCapi, hidden)}
		if got := s.GetContextPrefix("ns-name"); got != "" {
			t.Errorf("GetContextPrefix() = %q, want empty", got)
		}
	})
	t.Run("capi default prefix is the kind", func(t *testing.T) {
		s := &CapiStore{BaseStore: NewBaseStore(types.StoreKindCapi, types.KubeconfigStore{})}
		if got := s.GetContextPrefix("ns-name"); got != "capi" {
			t.Errorf("GetContextPrefix() = %q, want capi", got)
		}
	})

	t.Run("akamai honours showPrefix=false", func(t *testing.T) {
		s := &AkamaiStore{BaseStore: NewBaseStore(types.StoreKindAkamai, hidden)}
		if got := s.GetContextPrefix("c1"); got != "" {
			t.Errorf("GetContextPrefix() = %q, want empty", got)
		}
	})
	t.Run("akamai default prefix is kind/path", func(t *testing.T) {
		s := &AkamaiStore{BaseStore: NewBaseStore(types.StoreKindAkamai, types.KubeconfigStore{})}
		if got := s.GetContextPrefix("c1"); got != "akamai/c1" {
			t.Errorf("GetContextPrefix() = %q, want akamai/c1", got)
		}
	})

	t.Run("eks strips double dashes", func(t *testing.T) {
		s := &EKSStore{BaseStore: NewBaseStore(types.StoreKindEKS, types.KubeconfigStore{})}
		if got := s.GetContextPrefix("eks_prof--region--cluster"); got != "eks_prof-region-cluster" {
			t.Errorf("GetContextPrefix() = %q, want eks_prof-region-cluster", got)
		}
	})
}
