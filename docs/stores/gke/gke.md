---
title: Google GKE
---

# GKE store

Kswitch can discover Google Kubernetes Engine (GKE) clusters with the help of a locally installed `gcloud` tool.
`gcloud` takes care of the authentication and authorization flow.

## Setup

Please make sure that the `gcloud` tool is installed and on your `PATH`.

Next, create the GKE store configuration in the `kswitch` configuration file.
The only currently supported `authenticationType` is `gcloud`.

```yaml
cat ~/.kube/switch-config.yaml

kind: SwitchConfig
version: "v1alpha1"
kubeconfigStores:
  - kind: gke
    id: id-only-required-if-there-are-more-than-one-store
    config:
      # optionally set the account. Otherwise, the currently active gcloud account will be used.
      gcpAccount: my-gcp-account
      authentication:
        authenticationType: gcloud
      # optionally prefer a specific type of endpoint: dns, private, public
      preferredEndpoint: private
      # optionally limit to certain projects in account
      projectIDs:
        - project-1
        - project-2
```

## Limiting the projects that are searched

Listing GKE clusters costs one API call per project. Accounts that are members of many
organizations can see thousands of projects, which makes the search take several seconds even
though almost none of those projects contain a cluster.

There are three ways to narrow the search down. They can be combined and are applied in this order:

```yaml
kubeconfigStores:
  - kind: gke
    config:
      # 1. a server-side filter handed to the Cloud Resource Manager API.
      # Keeps matching the projects of organizations you are added to later on.
      # See https://cloud.google.com/sdk/gcloud/reference/topic/filters
      projectFilter: "parent.type:organization parent.id:1234567890"
      # 2. an explicit list of project IDs
      projectIDs:
        - project-1
      # 3. glob patterns matched against the project ID.
      # A pattern prefixed with "!" excludes the matching projects.
      projectPatterns:
        - "acme-*"
        - "!acme-sandbox-*"
```

Prefer `projectFilter` or `projectPatterns` over `projectIDs` when you are regularly added to new
organizations or projects: both keep matching new projects without touching the configuration.

## Search performance

The GKE store keeps two caches in the state directory so that the expensive parts of the search are
not repeated on every invocation:

- the discovered projects, refreshed after `refreshProjectsAfter` (defaults to `24h`)
- the projects that answered with a permanent error (billing disabled, Kubernetes Engine API not
  enabled, no permission). They are skipped for `skipUnusableProjectsFor` (defaults to `24h`)

```yaml
kubeconfigStores:
  - kind: gke
    config:
      # how long the discovered projects are reused before they are listed again.
      # Set to 0 to list the projects on every search.
      refreshProjectsAfter: 24h
      # how long projects that cannot serve GKE clusters are skipped.
      # Set to 0 to query every project on every search.
      skipUnusableProjectsFor: 24h
      # how many projects are queried for clusters in parallel (default 32).
      # Lower this when running into GCP API rate limits.
      maxConcurrentProjectRequests: 32
```

Both caches are dropped by `kswitch clean`, and a project leaves the skip list as soon as it serves
clusters again.

## Re-authentication for expired credentials
By using `kswitch` you are essentially reusing the valid credentials (`JWT` token) obtained via gcloud's OIDC flow.
As OIDC id tokens have an expiration date, these credentials can expire.
`kswitch` detects failed requests against the GCP API and triggers a re-authentication via `gcloud` (this will open the default Web browser).

```bash
switch
INFO[0014] Sucessfully obtained application default credentials.  store=gke
switched to context "gke_landscaper".
```

## Search for GKE Clusters

Kubeconfig context names are fuzzy-searchable using the following semantics.

In General: 
- `gke_<account-name>-<region/zone>-<cluster-name>/gke_<cluster-name>`

Example:
- `gke_sweet-account-europe-west2-a-sweet-cluster/gke_sweet-cluster`

In this example:
- Account name: sweet-account
- Location (zone / region): europe-west-2-a 
  - this is a zone for a zonal cluster and a region for a regional GKE cluster 
- Cluster name: sweet-cluster

However, remember that you can always define an `alias` for each context to define a name that you can better remember or query .

This is how looks like using the `switch` search (not that account information has been removed):
![](gke_search.png)
