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

package tui

import (
	"fmt"
	"strings"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/pkg/util"
	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

// fetchPreview returns a tea.Cmd that asynchronously loads the preview for the given item.
func fetchPreview(stores map[string]storetypes.KubeconfigStore, it item) tea.Cmd {
	return func() tea.Msg {
		store, ok := stores[it.storeID]
		if !ok {
			return previewMsg{forPath: it.path, content: ""}
		}

		data, err := store.GetKubeconfigForPath(it.path, it.tags)
		if err != nil {
			return previewMsg{forPath: it.path, content: fmt.Sprintf("error loading preview: %v", err)}
		}

		config, err := util.ParseSanitizedKubeconfig(data)
		if err != nil {
			return previewMsg{forPath: it.path, content: fmt.Sprintf("error parsing kubeconfig: %v", err)}
		}

		kubeconfigBytes, err := yaml.Marshal(config)
		if err != nil {
			return previewMsg{forPath: it.path, content: fmt.Sprintf("error marshaling kubeconfig: %v", err)}
		}

		content := string(kubeconfigBytes)

		// Append store-specific preview if supported
		previewer, ok := store.(storetypes.Previewer)
		if ok {
			storePreview, err := previewer.GetSearchPreview(it.path, it.tags)
			if err == nil && storePreview != "" {
				sep := strings.Repeat("─", 40)
				content = content + "\n" + sep + "\n" + storePreview
			}
		}

		return previewMsg{forPath: it.path, content: content}
	}
}
