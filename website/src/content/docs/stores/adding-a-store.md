---
title: Adding a store
---

# Adding a new kubeconfig store

This guide explains how to add support for a new backing store (a cloud
provider, a managed Kubernetes platform, a secret store, ...).

Thanks to the shared `BaseStore` and the store registry, adding a store does
**not** require touching the startup wiring (`cmd/switcher/switcher.go`). There
are only two places to edit plus one new file.

## Overview

A store implements the `KubeconfigStore` interface
([`pkg/store/types/types.go`](../../pkg/store/types/types.go)):

```go
type KubeconfigStore interface {
    GetID() string
    GetKind() types.StoreKind
    GetContextPrefix(path string) string
    VerifyKubeconfigPaths() error
    StartSearch(channel chan SearchResult)
    GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error)
    GetLogger() *logrus.Entry
    GetStoreConfig() types.KubeconfigStore
}
```

Most of these are boilerplate. By embedding `store.BaseStore`
([`pkg/store/base.go`](../../pkg/store/base.go)) you only have to implement what
is actually specific to your store:

- `StartSearch` — discover the available kubeconfigs and push their paths (and
  optional `Tags`) into the channel.
- `GetKubeconfigForPath` — fetch the raw kubeconfig bytes for one path.
- `GetContextPrefix` — (usually) the prefix shown in the fuzzy-search list.

`GetID`, `GetKind`, `GetStoreConfig`, `GetLogger` and a no-op
`VerifyKubeconfigPaths` are provided by `BaseStore`. To change any of them,
just declare a method with the same name on your store: it shadows the
promoted one (see `GardenerStore.GetID` for a real example).

## Step 1 — declare the store kind

In [`types/config.go`](../../types/config.go):

1. Add a `StoreKind` constant:
   ```go
   // StoreKindFoo is an identifier for the Foo store
   StoreKindFoo StoreKind = "foo"
   ```
2. Add it to `ValidStoreKinds` so the config validator accepts it.
3. If your store needs configuration, add a typed config struct:
   ```go
   type StoreConfigFoo struct {
       APIToken string `yaml:"apiToken"`
       Region   string `yaml:"region"`
   }
   ```

## Step 2 — implement and register the store

Create `pkg/store/kubeconfig_store_foo.go`. The struct field for your store lives
in [`pkg/store/types.go`](../../pkg/store/types.go) (embed `BaseStore`):

```go
type FooStore struct {
    BaseStore
    Client *foosdk.Client
    Config *types.StoreConfigFoo
}
```

Then the implementation file:

```go
package store

import (
    "fmt"

    storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
    "github.com/MichaelSp/kswitch/types"
)

// register the store so cmd/switcher can build it without a hardcoded switch
func init() {
    Register(types.StoreKindFoo, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
        return NewFooStore(s)
    })
}

func NewFooStore(store types.KubeconfigStore) (*FooStore, error) {
    // ParseStoreConfig replaces the yaml.Marshal/yaml.Unmarshal boilerplate.
    // It returns a usable (non-nil) *StoreConfigFoo even with no config block.
    config, err := ParseStoreConfig[types.StoreConfigFoo](store)
    if err != nil {
        return nil, err
    }

    if config.APIToken == "" {
        return nil, fmt.Errorf("the Foo store requires apiToken in the SwitchConfig file")
    }

    return &FooStore{
        BaseStore: NewBaseStore(types.StoreKindFoo, store),
        Config:    config,
        // Client: ...
    }, nil
}

func (s *FooStore) GetContextPrefix(path string) string {
    if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
        return ""
    }
    return string(types.StoreKindFoo)
}

func (s *FooStore) StartSearch(channel chan storetypes.SearchResult) {
    // discover clusters and push their paths
    // channel <- storetypes.SearchResult{KubeconfigPath: name, Tags: map[string]string{"clusterID": id}}
}

func (s *FooStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
    // fetch and return the raw kubeconfig bytes
    return nil, fmt.Errorf("not implemented")
}
```

The `Dependencies` argument carries the process-wide inputs some stores need
(`StateDirectory`, `KubeconfigName`, `VaultAPIAddressFromFlag`,
`VaultTokenFileName`). Use only what you need.

### Identifying clusters: prefer Tags

When the path you publish in `StartSearch` does not, on its own, uniquely
identify the cluster (duplicate names across projects/regions, opaque IDs),
attach a `Tags` map to the `SearchResult` and read it back in
`GetKubeconfigForPath`. Tags are persisted in the search index, so they also
work when discovery has not run in the current process. See the Scaleway and
Akamai stores for examples.

## Optional interfaces

- **`Previewer`** ([`pkg/store/types/types.go`](../../pkg/store/types/types.go)):
  implement `GetSearchPreview(path string, tags map[string]string) (string, error)`
  to show a custom preview before the kubeconfig (see EKS / Azure).
- **Lazy initialization**: if connecting to the backing store is slow, do it in
  an `InitializeFooStore()` method called from `StartSearch` /
  `GetKubeconfigForPath` rather than in the constructor, so the fuzzy search can
  appear quickly (see GKE / Azure / EKS).
- **Store-specific validation**: add a validator under `pkg/store/foo/` and call
  it from [`pkg/config/validation/validation.go`](../../pkg/config/validation/validation.go)
  (see Gardener / GKE).

## Step 3 — document and verify

- Add a `docs/stores/foo/foo.md` page and link it from
  [`docs/kubeconfig_stores.md`](../kubeconfig_stores.md).
- `go build ./...` already verifies your store satisfies the interface (the
  registry factory returns `storetypes.KubeconfigStore`).
- A factory may return `(nil, nil)` to opt out for the current environment
  (e.g. the Digital Ocean store when no `doctl` config exists); the startup loop
  skips it. Return an **untyped** nil in that case, never a typed nil pointer.
