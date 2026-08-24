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

package pkg

import (
	"fmt"
	"strings"
	"sync"
	"time"

	historyutil "github.com/MichaelSp/kswitch/pkg/subcommands/history/util"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"

	"github.com/MichaelSp/kswitch/pkg/index"
	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	aliasutil "github.com/MichaelSp/kswitch/pkg/subcommands/alias/util"
	"github.com/MichaelSp/kswitch/pkg/tui"
	kubeconfigutil "github.com/MichaelSp/kswitch/pkg/util/kubectx_copied"
	"github.com/MichaelSp/kswitch/types"
)

var (
	aliasToContext     = make(map[string]string)
	aliasToContextLock = sync.RWMutex{}

	// aggregated errors that were suppressed during the search
	searchError error

	logger = logrus.New()
)

func Switcher(stores []storetypes.KubeconfigStore, config *types.Config, stateDir string, noIndex, showPreview bool) (*string, *string, error) {
	// reset accumulated search errors from any previous invocation in the same
	// process to avoid re-logging stale warnings on subsequent runs.
	searchError = nil

	c, err := DoSearch(stores, config, stateDir, noIndex)
	if err != nil {
		return nil, nil, err
	}

	// remember the store for later kubeconfig retrieval
	kindToStore := map[string]storetypes.KubeconfigStore{}
	for _, s := range stores {
		kindToStore[s.GetID()] = s
	}

	// Collect alias data from the discovery channel so that after the TUI
	// returns we can look up the original context name behind an alias.
	// We also feed a ContextItem channel to tui.Run so the TUI can stream items
	// as they are discovered.
	tuiCh := make(chan tui.ContextItem)
	go func() {
		defer close(tuiCh)

		// Collect all discovered contexts, then deduplicate before feeding the TUI.
		// This ensures kind store entries take precedence over filesystem entries
		// that point to the same cluster (kind writes into ~/.kube/config by default).
		var all []DiscoveredContext
		for dc := range *c {
			if dc.Error != nil {
				logger.Debugf("%v", dc.Error)
				searchError = multierror.Append(searchError, dc.Error)
				continue
			}
			if dc.Store == nil {
				logger.Debugf("store returned from search is nil. This should not happen")
				continue
			}
			all = append(all, dc)
		}

		deduplicated := applyContextFilters(all, contextFilters...)

		for _, dc := range deduplicated {
			kubeconfigStore := *dc.Store
			contextName := dc.Name
			if len(dc.Alias) > 0 {
				contextName = dc.Alias
				writeToAliasToContext(dc.Alias, dc.Name)
			}

			tuiCh <- tui.ContextItem{
				ContextName:      contextName,
				Alias:            dc.Alias,
				StoreKind:        string(kubeconfigStore.GetKind()),
				Path:             dc.Path,
				Tags:             dc.Tags,
				StoreID:          kubeconfigStore.GetID(),
				LabelDisplayKeys: labelDisplayKeys(kubeconfigStore),
			}
		}
	}()

	defer logSearchErrors()

	kubeconfigPath, selectedContext, storeID, tags, dynamicStore, err := tui.Run(tuiCh, kindToStore, showPreview)
	if err != nil {
		return nil, nil, err
	}

	if kubeconfigPath == "" {
		return nil, nil, nil
	}

	store, err := resolveStore(kindToStore, storeID, dynamicStore)
	if err != nil {
		return nil, nil, err
	}

	// use the store to get the kubeconfig for the selected kubeconfig path
	kubeconfigData, err := store.GetKubeconfigForPath(kubeconfigPath, tags)
	if err != nil {
		return nil, nil, err
	}

	kubeconfig, err := kubeconfigutil.NewKubeconfig(kubeconfigData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse selected kubeconfig. Please check if this file is a valid kubeconfig: %w", err)
	}

	contextForHistory := selectedContext

	if len(store.GetContextPrefix(kubeconfigPath)) > 0 && strings.HasPrefix(selectedContext, store.GetContextPrefix(kubeconfigPath)) {
		selectedContext = strings.TrimPrefix(selectedContext, fmt.Sprintf("%s/", store.GetContextPrefix(kubeconfigPath)))
	}

	if err := kubeconfig.SetContext(selectedContext, aliasutil.GetContextForAlias(selectedContext, aliasToContext), store.GetContextPrefix(kubeconfigPath)); err != nil {
		return nil, nil, err
	}

	if err := kubeconfig.SetKswitchContext(contextForHistory); err != nil {
		return nil, nil, err
	}

	tempKubeconfigPath, err := kubeconfig.WriteKubeconfigFile()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write temporary kubeconfig file: %w", err)
	}

	ns, err := kubeconfig.NamespaceOfContext(kubeconfig.GetCurrentContext())
	if err != nil {
		logger.Warnf("failed to append context to history file: failed to get namespace of current context: %v", err)
	} else if err := historyutil.AppendToHistory(contextForHistory, ns); err != nil {
		logger.Warnf("failed to append context to history file: %v", err)
	}

	return &tempKubeconfigPath, &selectedContext, nil
}

// resolveStore returns the store that holds the kubeconfig for the selected item.
// It uses the dynamic store (k0smotron sub-cluster) if present, else the store the
// selected item was discovered from. Resolving via the selected item's own
// storeID (rather than a global path->store lookup) matters because "path"
// is only unique within a single store: two different stores can each have
// a cluster with the same path (e.g. both named "prod"), which would
// otherwise resolve to whichever store happened to be indexed last.
func resolveStore(kindToStore map[string]storetypes.KubeconfigStore, storeID string, dynamicStore storetypes.KubeconfigStore) (storetypes.KubeconfigStore, error) {
	if dynamicStore != nil {
		return dynamicStore, nil
	}

	store, ok := kindToStore[storeID]
	if !ok || store == nil {
		return nil, fmt.Errorf("failed to find the kubeconfig store %q the selected context was discovered from", storeID)
	}
	return store, nil
}

// writeIndex tries to write the Index file for the kubeconfig store
func writeIndex(store storetypes.KubeconfigStore, searchIndex *index.SearchIndex, ctxToPathMapping map[string]string, ctxToTagsMapping map[string]map[string]string) {
	index := types.Index{
		Kind:                 store.GetKind(),
		ContextToPathMapping: ctxToPathMapping,
		ContextToTags:        ctxToTagsMapping,
	}

	if err := searchIndex.Write(index); err != nil {
		store.GetLogger().Warnf("failed to write kubeconfig store index file: %v", err)
		return
	}

	indexStateToWrite := types.IndexState{
		Kind:           store.GetKind(),
		LastUpdateTime: time.Now().UTC(),
	}

	if err := searchIndex.WriteState(indexStateToWrite); err != nil {
		store.GetLogger().Warnf("failed to write index state file: %v", err)
	}
}

func writeToAliasToContext(key, value string) {
	aliasToContextLock.Lock()
	defer aliasToContextLock.Unlock()
	aliasToContext[key] = value
}

func logSearchErrors() {
	if searchError != nil {
		logger.Warnf("Suppressed warnings during the search: %v", searchError.Error())
	}
}
