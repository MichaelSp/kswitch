// Copyright 2026 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindKubernetes, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewKubernetesStore(s)
	})
}

var _ storetypes.KubeconfigStore = (*KubernetesStore)(nil)

// storeRefPrefix is the URI scheme used in StoreConfigKubernetes.StoreID to
// reference another configured store.
//
// Format: store://<store-id>/<context-path>
//   - store-id    is the GetID() value of another KubeconfigStore
//     (e.g. "gke.eu-prod", "filesystem.default")
//   - context-path is the opaque KubeconfigPath emitted by that store's
//     StartSearch (e.g. "europe-west1--my-cluster")
//
// Example: store://gke.eu-prod/europe-west1--my-cluster
const storeRefPrefix = "store://"

// defaultSecretKey is the secret data key used when StoreConfigKubernetes.Path is empty.
const defaultSecretKey = "kubeconfig"

// defaultNamespace is the namespace searched when StoreConfigKubernetes.Namespace is empty.
const defaultNamespace = "default"

// NewKubernetesStore constructs a KubernetesStore from the generic store config.
func NewKubernetesStore(store types.KubeconfigStore) (*KubernetesStore, error) {
	cfg, err := ParseStoreConfig[types.StoreConfigKubernetes](store)
	if err != nil {
		return nil, err
	}
	if cfg.Namespace == "" {
		cfg.Namespace = defaultNamespace
	}
	if cfg.Path == "" {
		cfg.Path = defaultSecretKey
	}
	if cfg.SecretName == "" && cfg.SecretLabelSelector == "" {
		return nil, fmt.Errorf("kubernetes store: either secretName or secretLabelSelector must be set")
	}
	if cfg.SecretName != "" && cfg.SecretLabelSelector != "" {
		return nil, fmt.Errorf("kubernetes store: secretName and secretLabelSelector are mutually exclusive")
	}
	return &KubernetesStore{
		BaseStore: NewBaseStore(types.StoreKindKubernetes, store),
		Config:    cfg,
	}, nil
}

// GetContextPrefix returns the store prefix shown in the selection list.
func (s *KubernetesStore) GetContextPrefix(_ string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}
	return string(types.StoreKindKubernetes)
}

// ResolvePeerStores wires up the kubeconfig for the cluster referenced by
// Config.StoreID (format: store://<store-id>/<context-path>).
// It must be called after all stores have been constructed.  When StoreID is
// empty this is a no-op.
func (s *KubernetesStore) ResolvePeerStores(allStores map[string]storetypes.KubeconfigStore) error {
	if s.Config.StoreID == "" {
		return nil
	}

	peerID, contextPath, err := parseStoreRef(s.Config.StoreID)
	if err != nil {
		return fmt.Errorf("kubernetes store %q: invalid storeID: %w", s.GetID(), err)
	}

	peer, ok := allStores[peerID]
	if !ok {
		return fmt.Errorf("kubernetes store %q: referenced store %q not found (available: %s)",
			s.GetID(), peerID, availableStoreIDs(allStores))
	}

	kubeconfig, resolvedPath, err := resolveContextPath(peer, contextPath)
	if err != nil {
		return fmt.Errorf("kubernetes store %q: fetch kubeconfig from peer store %q path %q: %w",
			s.GetID(), peerID, contextPath, err)
	}

	s.peerKubeconfig = kubeconfig
	s.Logger.Debugf("kubernetes: resolved peer store %q, path %q -> %q", peerID, contextPath, resolvedPath)
	return nil
}

// resolveContextPath fetches the kubeconfig bytes for contextPath from the given store.
// It first attempts a direct GetKubeconfigForPath call (works for API-backed stores
// and absolute filesystem paths).  If that fails, it drives StartSearch on the
// peer store and looks for a result whose full path or basename matches contextPath,
// then retries GetKubeconfigForPath with the resolved absolute path.
// This lets users write just a filename (e.g. "openmcp-live") in the storeID
// instead of a full absolute path.
func resolveContextPath(peer storetypes.KubeconfigStore, contextPath string) ([]byte, string, error) {
	// Expand a leading "~/" so users can write paths the same way they do in
	// the store's own `paths:` config (e.g. "~/.kube/my.yaml").
	expandedPath := expandHomedir(contextPath)

	// Fast path: direct hit (API-backed stores, or contextPath is already absolute).
	data, err := peer.GetKubeconfigForPath(expandedPath, nil)
	if err == nil {
		return data, expandedPath, nil
	}
	directErr := err

	// Slow path: search the peer store for a result matching by full path or
	// basename.  VerifyKubeconfigPaths must run first so path-based stores
	// (e.g. filesystem) populate their internal directory/file lists.
	// Ignore the error: if verification fails the store emits nothing and we
	// fall through to directErr.
	_ = peer.VerifyKubeconfigPaths()

	// Collect all paths so we can show them in the error if nothing matches.
	ch := make(chan storetypes.SearchResult, 64)
	go func() {
		peer.StartSearch(ch)
		close(ch)
	}()

	wantBase := filepath.Base(expandedPath)
	var available []string
	for result := range ch {
		if result.Error != nil {
			continue
		}
		p := result.KubeconfigPath
		available = append(available, p)
		if p == expandedPath || filepath.Base(p) == wantBase {
			data, err := peer.GetKubeconfigForPath(p, result.Tags)
			if err != nil {
				return nil, "", err
			}
			return data, p, nil
		}
	}

	// Nothing matched — report what is available.
	if len(available) > 0 {
		return nil, "", fmt.Errorf("no entry matching %q found in peer store (available: %s)",
			contextPath, strings.Join(available, ", "))
	}
	return nil, "", directErr
}

// expandHomedir replaces a leading "~/" or bare "~" with the current user's
// home directory, mirroring the expansion done in FilesystemStore.VerifyKubeconfigPaths.
func expandHomedir(path string) string {
	if path == "~" {
		if usr, err := user.Current(); err == nil {
			return usr.HomeDir
		}
	} else if strings.HasPrefix(path, "~/") {
		if usr, err := user.Current(); err == nil {
			return filepath.Join(usr.HomeDir, path[2:])
		}
	}
	return path
}

// ResolvePeerStoresForAll calls ResolvePeerStores on every KubernetesStore in
// the slice.  Call this once in switcher.go after all stores have been built.
func ResolvePeerStoresForAll(stores []storetypes.KubeconfigStore) error {
	byID := make(map[string]storetypes.KubeconfigStore, len(stores))
	for _, s := range stores {
		byID[s.GetID()] = s
	}

	for _, s := range stores {
		ks, ok := s.(*KubernetesStore)
		if !ok {
			// unwrap cache wrapper if present
			if u, ok2 := unwrapKubernetesStore(s); ok2 {
				ks = u
			}
		}
		if ks == nil {
			continue
		}
		if err := ks.ResolvePeerStores(byID); err != nil {
			return err
		}
	}
	return nil
}

// unwrapKubernetesStore tries to reach through a single layer of wrapping
// (e.g. the in-memory cache) to find a *KubernetesStore.
// The cache wrapper implements storetypes.KubeconfigStore but exposes the
// underlying store via an Unwrap() method by convention.
func unwrapKubernetesStore(s storetypes.KubeconfigStore) (*KubernetesStore, bool) {
	type unwrapper interface {
		Unwrap() storetypes.KubeconfigStore
	}
	if u, ok := s.(unwrapper); ok {
		if ks, ok2 := u.Unwrap().(*KubernetesStore); ok2 {
			return ks, true
		}
	}
	return nil, false
}

// StartSearch lists the configured secrets and emits one SearchResult per secret.
func (s *KubernetesStore) StartSearch(channel chan storetypes.SearchResult) {
	s.Logger.Debug("kubernetes: start search")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	secrets, err := s.listSecrets(ctx)
	if err != nil {
		channel <- storetypes.SearchResult{Error: fmt.Errorf("kubernetes store: list secrets: %w", err)}
		return
	}

	for _, secret := range secrets {
		if _, ok := secret.Data[s.Config.Path]; !ok {
			s.Logger.Debugf("kubernetes: secret %s/%s has no key %q, skipping", secret.Namespace, secret.Name, s.Config.Path)
			continue
		}

		path := secretPath(secret.Namespace, secret.Name)
		tags := map[string]string{
			"namespace":  secret.Namespace,
			"secretName": secret.Name,
		}

		// Build the alias list from all configured sources:
		//   1. static list from Alias.List (new field)
		//   2. legacy Aliases field (back-compat)
		//   3. value of each label named in Alias.FromLabels (per-secret)
		var allAliases []string
		allAliases = append(allAliases, s.Config.Alias.List...)
		for _, labelKey := range s.Config.Alias.FromLabels {
			if v := secret.Labels[labelKey]; v != "" {
				allAliases = append(allAliases, v)
			}
		}
		if len(allAliases) > 0 {
			tags["aliases"] = strings.Join(allAliases, ",")
		}

		s.Logger.Debugf("kubernetes: found secret %s/%s", secret.Namespace, secret.Name)
		channel <- storetypes.SearchResult{
			KubeconfigPath: path,
			Tags:           tags,
		}
	}
}

// GetKubeconfigForPath fetches the kubeconfig bytes from the referenced secret.
// The value at Config.Path is interpreted as a full kubeconfig YAML when it
// contains the string "apiVersion:", otherwise it is treated as a bearer token
// and a minimal kubeconfig is synthesised from it.
func (s *KubernetesStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ns := tags["namespace"]
	name := tags["secretName"]
	if ns == "" || name == "" {
		// fall back to parsing the path
		var err error
		ns, name, err = parseSecretPath(path)
		if err != nil {
			return nil, err
		}
	}

	client, err := s.buildClient()
	if err != nil {
		return nil, fmt.Errorf("kubernetes store: build client: %w", err)
	}

	secret, err := client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// List available secrets in the namespace to guide the user.
		available := s.listSecretNames(ctx, client, ns)
		if len(available) > 0 {
			return nil, fmt.Errorf("kubernetes store: secret %s/%s not found (available secrets in %s: %s): %w",
				ns, name, ns, strings.Join(available, ", "), err)
		}
		return nil, fmt.Errorf("kubernetes store: get secret %s/%s: %w", ns, name, err)
	}

	raw, ok := secret.Data[s.Config.Path]
	if !ok {
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("kubernetes store: secret %s/%s has no key %q (available keys: %s)",
			ns, name, s.Config.Path, strings.Join(keys, ", "))
	}

	// Full kubeconfig: pass through unchanged.
	if looksLikeKubeconfig(raw) {
		return raw, nil
	}

	// Bearer token: synthesise a minimal kubeconfig.
	return tokenToKubeconfig(string(raw), name), nil
}

// listSecretNames returns the names of all secrets in the given namespace,
// used to enrich "secret not found" error messages.
func (s *KubernetesStore) listSecretNames(ctx context.Context, client kubernetes.Interface, namespace string) []string {
	list, err := client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(list.Items))
	for _, sec := range list.Items {
		names = append(names, sec.Name)
	}
	sort.Strings(names)
	return names
}

// listSecrets returns secrets matched by name or label selector.
func (s *KubernetesStore) listSecrets(ctx context.Context) ([]corev1.Secret, error) {
	client, err := s.buildClient()
	if err != nil {
		return nil, err
	}

	if s.Config.SecretName != "" {
		secret, err := client.CoreV1().Secrets(s.Config.Namespace).Get(ctx, s.Config.SecretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return []corev1.Secret{*secret}, nil
	}

	list, err := client.CoreV1().Secrets(s.Config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: s.Config.SecretLabelSelector,
	})
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		// List all secrets in the namespace so the user knows what is there.
		all, _ := client.CoreV1().Secrets(s.Config.Namespace).List(ctx, metav1.ListOptions{})
		var names []string
		for _, sec := range all.Items {
			names = append(names, sec.Name)
		}
		sort.Strings(names)
		if len(names) > 0 {
			return nil, fmt.Errorf("no secrets matched label selector %q in namespace %q (available secrets: %s)",
				s.Config.SecretLabelSelector, s.Config.Namespace, strings.Join(names, ", "))
		}
		return nil, fmt.Errorf("no secrets matched label selector %q in namespace %q (namespace is empty)",
			s.Config.SecretLabelSelector, s.Config.Namespace)
	}
	return list.Items, nil
}

// buildClient constructs a Kubernetes client.
// When peerKubeconfig is set (populated by ResolvePeerStores) it is used to
// connect to the target cluster; otherwise the ambient kubeconfig / current
// context is used.
func (s *KubernetesStore) buildClient() (kubernetes.Interface, error) {
	if s.clientOverride != nil {
		return s.clientOverride, nil
	}

	if len(s.peerKubeconfig) > 0 {
		restCfg, err := clientcmd.RESTConfigFromKubeConfig(s.peerKubeconfig)
		if err != nil {
			return nil, fmt.Errorf("kubernetes store: parse peer kubeconfig: %w", err)
		}
		return kubernetes.NewForConfig(restCfg)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to build rest config: %w", err)
	}

	return kubernetes.NewForConfig(restCfg)
}

// parseStoreRef parses a store reference of the form store://<store-id>/<context-path>.
func parseStoreRef(ref string) (storeID, contextPath string, err error) {
	if !strings.HasPrefix(ref, storeRefPrefix) {
		return "", "", fmt.Errorf("store reference must start with %q, got %q", storeRefPrefix, ref)
	}
	rest := strings.TrimPrefix(ref, storeRefPrefix)
	// Split on first "/" to separate store-id from context-path.
	idx := strings.Index(rest, "/")
	if idx <= 0 {
		return "", "", fmt.Errorf("store reference %q must have format store://<store-id>/<context-path>", ref)
	}
	storeID = rest[:idx]
	contextPath = rest[idx+1:]
	if contextPath == "" {
		return "", "", fmt.Errorf("store reference %q must have a non-empty context path after the store ID", ref)
	}
	return storeID, contextPath, nil
}

// secretPath encodes namespace and name into a single opaque path string.
func secretPath(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

// parseSecretPath is the inverse of secretPath.
func parseSecretPath(path string) (namespace, name string, err error) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("kubernetes store: invalid secret path %q", path)
	}
	return parts[0], parts[1], nil
}

// looksLikeKubeconfig returns true when the bytes appear to be a kubeconfig
// YAML (contains the mandatory "apiVersion:" field).
func looksLikeKubeconfig(data []byte) bool {
	return strings.Contains(string(data), "apiVersion:")
}

// tokenToKubeconfig builds a minimal kubeconfig that uses the given bearer
// token. The cluster name is derived from the secret name so that the context
// is identifiable in the TUI.
func tokenToKubeconfig(token, secretName string) []byte {
	tmpl := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: ""
  name: %s
users:
- name: %s
  user:
    token: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
`
	return []byte(fmt.Sprintf(tmpl,
		secretName, secretName, token,
		secretName, secretName, secretName,
		secretName,
	))
}

// availableStoreIDs returns a comma-separated list of store IDs for error messages.
func availableStoreIDs(stores map[string]storetypes.KubeconfigStore) string {
	ids := make([]string, 0, len(stores))
	for id := range stores {
		ids = append(ids, id)
	}
	return strings.Join(ids, ", ")
}
