---
title: k0smotron
---

# k0smotron sub-cluster discovery

[k0smotron](https://k0smotron.io) runs k0s control planes as workloads inside a host Kubernetes cluster.
`kswitch` can discover these nested clusters interactively — no extra configuration needed.

## Usage

Press `→` on any cluster in the selection list to expand it and reveal any k0smotron sub-clusters running inside it.

```
  my-host-cluster
▼ my-host-cluster          ← pressed →
    default/tenant-a       (k0smotron)
    default/tenant-b       (k0smotron)
  other-cluster
```

| Key | Action |
|-----|--------|
| `→` | Expand: discover k0smotron clusters inside the selected cluster |
| `←` | Collapse the expanded node, or jump to parent if on a child |
| `↑` / `↓` | Navigate as usual across parent and child items |
| `Enter` | Switch to the selected cluster (works on both parents and children) |

While discovery is in progress the item shows `⟳`. If the cluster has no k0smotron installation, `→` silently does nothing.

Expansion works recursively: pressing `→` on a k0smotron sub-cluster will discover any k0smotron clusters nested inside it.

## How it works

When you press `→`, kswitch:

1. Fetches the kubeconfig for the selected cluster from its backing store.
2. Connects to that cluster and lists `k0smotron.io/v1beta1` Cluster resources.
3. For each cluster, reads the Secret `<name>-kubeconfig` (key: `value`) in the same namespace.
4. Inserts the discovered clusters as child items in the list.

The child kubeconfigs are held in memory for the duration of the session and never written to disk until you press `Enter` to select one.

## Requirements

- k0smotron must be installed in the target cluster (`k0smotron.io` CRD present).
- The kubeconfig for the host cluster must have permission to:
  - `list` `clusters.k0smotron.io` (cluster-scoped or per-namespace)
  - `get` the `<name>-kubeconfig` Secret in each cluster's namespace.

## No configuration required

k0smotron discovery is built into the TUI and requires no entry in `switch-config.yaml`.
It works with any cluster surfaced by any configured store (filesystem, CAPI, GKE, etc.).
