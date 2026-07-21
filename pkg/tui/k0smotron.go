package tui

import (
	kstore "github.com/MichaelSp/kswitch/pkg/store"
	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	tea "github.com/charmbracelet/bubbletea"
)

// expandK0smotronCmd fires a background command that fetches the kubeconfig for
// the selected item then discovers k0smotron sub-clusters inside it.
func expandK0smotronCmd(
	stores map[string]storetypes.KubeconfigStore,
	dynamicStores map[string]storetypes.KubeconfigStore,
	parent item,
) tea.Cmd {
	return func() tea.Msg {
		store := dynamicStores[parent.storeID]
		if store == nil {
			store = stores[parent.storeID]
		}
		if store == nil {
			return expandResultMsg{parentPath: parent.path}
		}

		kubeconfigData, err := store.GetKubeconfigForPath(parent.path, parent.tags)
		if err != nil {
			return expandResultMsg{parentPath: parent.path, err: err}
		}

		memStore, entries, err := kstore.DiscoverK0smotronClusters(parent.path, kubeconfigData)
		if err != nil {
			return expandResultMsg{parentPath: parent.path, err: err}
		}
		if len(entries) == 0 {
			return expandResultMsg{parentPath: parent.path}
		}

		children := make([]item, 0, len(entries))
		for _, e := range entries {
			children = append(children, item{
				displayName: e.DisplayName,
				dimSuffix:   "(k0smotron)",
				contextName: e.ContextName,
				path:        e.Path,
				tags: map[string]string{
					"namespace": e.Namespace,
					"name":      e.Name,
				},
				storeID:    e.StoreID,
				depth:      parent.depth + 1,
				parentPath: parent.path,
			})
		}

		return expandResultMsg{
			parentPath: parent.path,
			children:   children,
			store:      memStore,
		}
	}
}
