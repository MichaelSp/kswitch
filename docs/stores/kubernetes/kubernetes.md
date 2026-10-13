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

    # Alias configuration for contexts discovered from this store.
    # + optional
    alias:
      # Static alias names added to every context from this store.
      # + optional
      list:
        - prod
        - production

      # Secret label names whose values are added as aliases per-secret.
      # Useful when each secret carries a human-friendly name label.
      # + optional
      fromLabels:
        - core.openmcp.cloud/mcp-name
        - openmcp.cloud/mcp-workspace
```

## Targeting a specific cluster with `storeID`

By default the store uses the ambient kubeconfig / current context.
To connect to a specific cluster managed by another store, set `storeID` to a `store://` URI:

```
store://<store-id>/<context-path>
```

| Part | Description | Example |
|---|---|---|
| `store-id` | The `id` of another `kubeconfigStores` entry, in `<kind>.<id>` form | `gke.eu-prod`, `filesystem.openmcp` |
| `context-path` | Filename or path that store emits for the target cluster. For `filesystem` stores this can be a bare filename, a `~/`-prefixed path, or a full absolute path. | `my-cluster.yaml`, `~/.kube/my-cluster.yaml` |

The easiest way to find the right value is to run `kswitch` and look at the context path shown
in the TUI for the cluster you want to target.

If the `context-path` does not match any entry in the peer store, kswitch reports all available
paths so you can pick the correct one:

```
no entry matching "bad-name" found in peer store (available: /Users/me/.kube/real-name.yaml)
```

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
    alias:
      list:
        - tenant
```

### Example: filesystem store as cluster source

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: filesystem
  id: openmcp
  paths:
    - "~/.kube/openmcp-live.yaml"

- kind: kubernetes
  config:
    storeID: "store://filesystem.openmcp/~/.kube/openmcp-live.yaml"
    namespace: project-my-team
    secretLabelSelector: "core.openmcp.cloud/oidc-provider=openmcp"
    alias:
      fromLabels:
        - core.openmcp.cloud/mcp-name
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

## `alias` — display aliases

Each context discovered from this store can be given one or more additional names that appear
in the TUI search list.  Aliases come from two sources that are merged:

| Source | Config key | Description |
|---|---|---|
| Static list | `alias.list` | Fixed names added to every context from this store |
| Secret labels | `alias.fromLabels` | List of label names; each label's value on the secret becomes an alias |

When a context has aliases, each alias appears as a separate selectable entry so users can
find the same context by any name.

```yaml
alias:
  list:
    - my-fixed-alias
  fromLabels:
    - core.openmcp.cloud/mcp-name
    - openmcp.cloud/mcp-workspace
```

## Secret value formats

The value at `path` (default `kubeconfig`) is interpreted as follows:

| Content | Handling |
|---|---|
| Contains `apiVersion:` | Treated as a full kubeconfig YAML and returned as-is |
| Anything else | Treated as a bearer token; a minimal kubeconfig is synthesised |

The synthesised kubeconfig uses an empty server URL and the secret name as the context name.
Patch the secret with a proper kubeconfig if you need a real server endpoint.

## Error messages

kswitch provides actionable errors when configuration references are wrong:

| Situation | Error includes |
|---|---|
| Unknown `store-id` in `storeID` | All configured store IDs |
| `context-path` not found in peer store | All paths the peer store exposes |
| Secret not found by `secretName` | All secret names in the namespace |
| Label selector matches nothing | All secret names in the namespace |
| Secret missing the configured `path` key | All keys present in that secret |

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
    alias:
      list:
        - prod
- kind: kubernetes
  id: staging-secrets
  config:
    namespace: staging
    secretLabelSelector: "role=kubeconfig"
    alias:
      list:
        - staging
```
