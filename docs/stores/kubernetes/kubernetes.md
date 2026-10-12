---
title: Kubernetes
---

# Kubernetes store

The `kubernetes` store discovers kubeconfigs (or bearer tokens) stored as Kubernetes Secrets in a cluster.
It connects to the cluster — either the ambient kubeconfig / current context, or a specific cluster from
any other configured store — and exposes each matching secret as a selectable context in the `kswitch` TUI.

## Configuration

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: kubernetes
  config:
    # Cluster to connect to.  Format: store://<store-id>/<context-path>
    # If omitted, the ambient kubeconfig / current context is used.
    # + optional
    storeID: "store://gke.eu-prod/europe-west1--my-management-cluster"

    # Namespace to search for secrets.
    # Defaults to "default".
    # + optional
    namespace: kube-system

    # Exact secret name.  Mutually exclusive with secretLabelSelector.
    # + optional
    secretName: my-cluster-kubeconfig

    # Label-selector string.  Mutually exclusive with secretName.
    # + optional
    secretLabelSelector: "app=myapp,env=prod"

    # Key inside the secret whose value is either a full kubeconfig YAML
    # or a raw bearer token.  Defaults to "kubeconfig".
    # + optional
    path: kubeconfig

    # Optional display aliases for every context discovered from this store.
    # Each alias appears alongside the context name in the search list.
    # + optional
    aliases:
      - prod
      - production
```

## Targeting a specific cluster with `storeID`

By default the store uses the ambient kubeconfig / current context.
To connect to a specific cluster managed by another store, set `storeID` to a `store://` URI:

```
store://<store-id>/<context-path>
```

| Part | Description | Example |
|---|---|---|
| `store-id` | The `id` of another `kubeconfigStores` entry, in `<kind>.<id>` form | `gke.eu-prod`, `filesystem.default` |
| `context-path` | The opaque path that store's `StartSearch` emits for the target cluster | `europe-west1--my-cluster` |

The easiest way to find the right value is to run `kswitch` and look at the context path shown
in the TUI for the cluster you want to target.

### Example: secrets on a GKE management cluster

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
# The GKE store that manages the management cluster.
- kind: gke
  id: eu-prod
  config:
    projectIDs: ["my-project"]

# Read kubeconfigs from secrets stored in that cluster.
- kind: kubernetes
  config:
    storeID: "store://gke.eu-prod/europe-west1--my-management-cluster"
    namespace: managed-clusters
    secretLabelSelector: "role=tenant-kubeconfig"
    aliases:
      - tenant
```

### Example: ambient kubeconfig (no storeID)

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: kubernetes
  config:
    namespace: kube-system
    secretName: my-cluster-kubeconfig
```

## `secretName` vs `secretLabelSelector`

Exactly one of the two must be set:

| Field | Behaviour |
|---|---|
| `secretName` | Reads a single, named secret |
| `secretLabelSelector` | Lists all secrets matching the selector; one entry per secret |

## Secret value formats

The value at `path` (default `kubeconfig`) is interpreted as follows:

| Content | Handling |
|---|---|
| Contains `apiVersion:` | Treated as a full kubeconfig YAML and returned as-is |
| Anything else | Treated as a bearer token; a minimal kubeconfig is synthesised |

The synthesised kubeconfig uses an empty server URL and the secret name as the context name.
Patch the secret with a proper kubeconfig if you need a real server endpoint.

## Multiple stores

You can configure several `kubernetes` store entries — one per namespace or remote cluster — by assigning unique IDs:

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: kubernetes
  id: prod-secrets
  config:
    storeID: "store://gke.eu-prod/europe-west1--my-cluster"
    namespace: production
    secretLabelSelector: "role=kubeconfig"
    aliases:
      - prod
- kind: kubernetes
  id: staging-secrets
  config:
    namespace: staging
    secretLabelSelector: "role=kubeconfig"
    aliases:
      - staging
```
