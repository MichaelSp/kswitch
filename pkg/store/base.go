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

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/MichaelSp/kswitch/types"
)

// BaseStore provides the boilerplate shared by every kubeconfig store
// implementation. Concrete stores embed it to inherit the common
// KubeconfigStore methods (GetID, GetKind, GetStoreConfig, GetLogger and a
// no-op VerifyKubeconfigPaths) and only have to implement the behaviour that is
// actually specific to the backing store (StartSearch, GetKubeconfigForPath,
// GetContextPrefix, ...).
//
// A store that needs a different behaviour for any of the shared methods simply
// declares its own method with the same name: it shadows the promoted one.
type BaseStore struct {
	Kind            types.StoreKind
	KubeconfigStore types.KubeconfigStore
	Logger          *logrus.Entry
}

// NewBaseStore builds a BaseStore for the given store kind and configuration.
// It pre-populates the logger so that stores accessing the Logger field
// directly (instead of through GetLogger) never hit a nil pointer.
func NewBaseStore(kind types.StoreKind, kubeconfigStore types.KubeconfigStore) BaseStore {
	return BaseStore{
		Kind:            kind,
		KubeconfigStore: kubeconfigStore,
		Logger:          logrus.WithField("store", kind),
	}
}

// GetID returns the unique store ID
//   - "<store kind>.default" if the kubeconfigStore.ID is not set
//   - "<store kind>.<id>" if the kubeconfigStore.ID is set
func (b *BaseStore) GetID() string {
	id := "default"
	if b.KubeconfigStore.ID != nil {
		id = *b.KubeconfigStore.ID
	}
	return fmt.Sprintf("%s.%s", b.Kind, id)
}

// GetKind returns the store kind (e.g., filesystem)
func (b *BaseStore) GetKind() types.StoreKind {
	return b.Kind
}

// GetStoreConfig returns the store's configuration from the switch config file
func (b *BaseStore) GetStoreConfig() types.KubeconfigStore {
	return b.KubeconfigStore
}

// GetLogger returns the logger of the store, lazily creating it if necessary.
func (b *BaseStore) GetLogger() *logrus.Entry {
	if b.Logger == nil {
		b.Logger = logrus.WithField("store", b.Kind)
	}
	return b.Logger
}

// VerifyKubeconfigPaths is a no-op by default. Stores that allow configuring
// search paths (e.g. filesystem, vault) override it.
func (b *BaseStore) VerifyKubeconfigPaths() error {
	return nil
}

// ParseStoreConfig decodes the store-specific configuration (an untyped value
// read from the switch config file) into the typed config struct T.
//
// It replaces the yaml.Marshal/yaml.Unmarshal dance that was previously
// duplicated in every store constructor. A non-nil *T is always returned when
// err is nil, even when the store has no configuration block, so callers can
// rely on the pointer being usable.
func ParseStoreConfig[T any](store types.KubeconfigStore) (*T, error) {
	config := new(T)
	if store.Config == nil {
		return config, nil
	}

	buf, err := yaml.Marshal(store.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to process %s store config: %w", store.Kind, err)
	}

	if err := yaml.Unmarshal(buf, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s store config: %w", store.Kind, err)
	}

	return config, nil
}
