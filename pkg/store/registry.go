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
	"fmt"
	"sort"
	"sync"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// Dependencies carries the process-wide inputs that some store constructors
// need in addition to their per-store configuration. It lets every store be
// built through a single Factory signature regardless of which of these inputs
// it actually consumes.
type Dependencies struct {
	KubeconfigName          string
	StateDirectory          string
	VaultAPIAddressFromFlag string
	VaultTokenFileName      string
}

// Factory builds a store from its configuration and the shared dependencies.
//
// A factory may return (nil, nil) to signal that the store does not apply in
// the current environment (e.g. the DigitalOcean store when no doctl config is
// present). Callers must skip such stores. To return a real nil store, a
// factory must return an untyped nil, never a typed nil pointer, otherwise the
// (nil, nil) contract breaks because of Go's typed-nil-in-interface behaviour.
type Factory func(store types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error)

var (
	registryMu sync.RWMutex
	registry   = map[types.StoreKind]Factory{}
)

// Register adds a factory for the given store kind. It is meant to be called
// from a store file's init(). Registering the same kind twice panics, as that
// is always a programming error.
func Register(kind types.StoreKind, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[kind]; exists {
		panic(fmt.Sprintf("store kind %q is already registered", kind))
	}
	registry[kind] = factory
}

// Create instantiates the store for the given configuration using the
// registered factory. It returns (nil, nil) when the factory opts out (see
// Factory). An unknown kind yields an error.
func Create(store types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
	registryMu.RLock()
	factory, ok := registry[store.Kind]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown store %q", store.Kind)
	}
	return factory(store, deps)
}

// RegisteredKinds returns the sorted list of registered store kinds. Useful for
// diagnostics and help output.
func RegisteredKinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, string(kind))
	}
	sort.Strings(kinds)
	return kinds
}
