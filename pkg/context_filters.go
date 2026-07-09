// Copyright 2024 The Kswitch authors
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

package pkg

import (
	"strings"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/pkg/util"
	"github.com/MichaelSp/kswitch/types"
)

// labelDisplayKeys extracts the ShootLabelKeys from the store if it implements
// the optional LabelKeysProvider interface (currently only GardenerStore does).
func labelDisplayKeys(store storetypes.KubeconfigStore) []string {
	type labelKeysProvider interface {
		GetShootLabelKeys() []string
	}
	if p, ok := store.(labelKeysProvider); ok {
		return p.GetShootLabelKeys()
	}
	return nil
}

// contextFilter is a function that prunes a slice of discovered contexts.
// Each filter receives the full current list and returns a filtered copy.
type contextFilter func([]DiscoveredContext) []DiscoveredContext

// contextFilters is the ordered list of filters applied before the TUI is shown.
var contextFilters = []contextFilter{
	filterKindDuplicates,
	filterPreferExternal,
}

// ApplyContextFilters runs the registered filters and returns the pruned list.
// Exported so subcommands can apply the same dedup logic as the TUI.
func ApplyContextFilters(contexts []DiscoveredContext) []DiscoveredContext {
	return applyContextFilters(contexts, contextFilters...)
}

// applyContextFilters runs each filter in order and returns the result.
func applyContextFilters(contexts []DiscoveredContext, filters ...contextFilter) []DiscoveredContext {
	for _, f := range filters {
		contexts = f(contexts)
	}
	return contexts
}

// filterKindDuplicates removes filesystem-store entries that point to the same
// cluster as a kind-store entry. kind entries always win because they carry the
// canonical cluster name.
func filterKindDuplicates(contexts []DiscoveredContext) []DiscoveredContext {
	hasKind := false
	for _, dc := range contexts {
		if (*dc.Store).GetKind() == types.StoreKindKind {
			hasKind = true
			break
		}
	}
	if !hasKind {
		return contexts
	}

	kindIdentities := make(map[clusterIdentity]struct{})
	for _, dc := range contexts {
		if (*dc.Store).GetKind() != types.StoreKindKind {
			continue
		}
		if id, ok := clusterIdentityForContext(dc); ok && id.server != "" {
			kindIdentities[id] = struct{}{}
		}
	}
	if len(kindIdentities) == 0 {
		return contexts
	}

	result := make([]DiscoveredContext, 0, len(contexts))
	for _, dc := range contexts {
		if (*dc.Store).GetKind() == types.StoreKindFilesystem {
			if id, ok := clusterIdentityForContext(dc); ok {
				if _, shadowed := kindIdentities[id]; shadowed {
					logger.Debugf("filter: dropping filesystem context %q (shadowed by kind store)", dc.Name)
					continue
				}
			}
		}
		result = append(result, dc)
	}
	return result
}

// filterPreferExternal groups contexts by their kubeconfig path and keeps only
// the ones whose name ends with "-external". If no "-external" context exists
// for a path, all contexts for that path are kept unchanged.
func filterPreferExternal(contexts []DiscoveredContext) []DiscoveredContext {
	// group by path, preserving discovery order
	type entry struct {
		idx int
		dc  DiscoveredContext
	}
	byPath := make(map[string][]entry)
	order := make([]string, 0)
	seen := make(map[string]bool)
	for i, dc := range contexts {
		if !seen[dc.Path] {
			seen[dc.Path] = true
			order = append(order, dc.Path)
		}
		byPath[dc.Path] = append(byPath[dc.Path], entry{i, dc})
	}

	result := make([]DiscoveredContext, 0, len(contexts))
	for _, path := range order {
		entries := byPath[path]
		var externals []entry
		for _, e := range entries {
			if strings.HasSuffix(e.dc.Name, "-external") {
				externals = append(externals, e)
			}
		}
		keep := entries
		if len(externals) > 0 {
			keep = externals
			logger.Debugf("filter: path %q has %d -external context(s), dropping %d others", path, len(externals), len(entries)-len(externals))
		}
		for _, e := range keep {
			result = append(result, e.dc)
		}
	}
	return result
}

// clusterIdentity is a fingerprint derived from the cluster's API server URL
// and CA certificate. Two entries that share the same identity point to the
// same cluster regardless of which store discovered them.
type clusterIdentity struct {
	server string
	ca     string
}

// clusterIdentityForContext fetches the kubeconfig for dc and returns the
// identity of the cluster referenced by its current-context. Returns the zero
// value and false if the kubeconfig cannot be fetched or parsed.
func clusterIdentityForContext(dc DiscoveredContext) (clusterIdentity, bool) {
	store := *dc.Store
	data, err := store.GetKubeconfigForPath(dc.Path, dc.Tags)
	if err != nil {
		return clusterIdentity{}, false
	}
	kubeconf, err := util.ParseSanitizedKubeconfig(data)
	if err != nil {
		return clusterIdentity{}, false
	}

	ctxName := kubeconf.CurrentContext
	if ctxName == "" && len(kubeconf.Contexts) > 0 {
		ctxName = kubeconf.Contexts[0].Name
	}
	clusterName := ""
	for _, ctx := range kubeconf.Contexts {
		if ctx.Name == ctxName {
			clusterName = ctx.Context.Cluster
			break
		}
	}
	for _, cl := range kubeconf.Clusters {
		if cl.Name == clusterName {
			return clusterIdentity{
				server: cl.Cluster.Server,
				ca:     cl.Cluster.CertificateAuthorityData,
			}, true
		}
	}
	return clusterIdentity{}, false
}
