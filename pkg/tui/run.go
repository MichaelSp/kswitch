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
	"errors"
	"fmt"
	"os"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	tea "github.com/charmbracelet/bubbletea"
)

// ErrAbort is returned when the user exits the TUI without selecting a context.
var ErrAbort = errors.New("abort")

// ContextItem is the input type that callers feed into Run.
type ContextItem struct {
	// ContextName is the actual kubeconfig context name (or alias) used for lookup.
	ContextName string
	// Alias is the human-friendly alias for this context, if any.
	Alias string
	// StoreKind identifies the backing store so the display can be formatted accordingly.
	StoreKind string
	Path      string
	Tags      map[string]string
	StoreID   string
	// LabelDisplayKeys lists tag keys whose values should appear in the display suffix.
	// Values are shown in the order given and become part of the fuzzy-search string.
	LabelDisplayKeys []string
}

// Run launches the interactive bubbletea TUI and blocks until the user selects
// a context or aborts. itemCh must be closed by the caller when discovery ends.
// Returns the path and display name of the selected item, or ErrAbort if the
// user cancelled. dynamicStore is non-nil when the selection is a k0smotron
// sub-cluster whose kubeconfig lives in an in-memory store.
func Run(
	itemCh <-chan ContextItem,
	storeIDToStore map[string]storetypes.KubeconfigStore,
	showPreview bool,
) (kubeconfigPath string, selectedContext string, dynamicStore storetypes.KubeconfigStore, err error) {
	model := NewModel(storeIDToStore, showPreview)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(os.Stderr))

	go func() {
		var batch []item
		for ci := range itemCh {
			primary, suffix := FormatDisplayName(types.StoreKind(ci.StoreKind), ci.Path, ci.ContextName, ci.Alias, ci.Tags, ci.LabelDisplayKeys)
			batch = append(batch, item{
				displayName: primary,
				dimSuffix:   suffix,
				contextName: ci.ContextName,
				path:        ci.Path,
				tags:        ci.Tags,
				storeID:     ci.StoreID,
			})
			if len(batch) >= 50 {
				p.Send(itemsMsg(batch))
				batch = nil
			}
		}
		if len(batch) > 0 {
			p.Send(itemsMsg(batch))
		}
		p.Send(discoveryDoneMsg{})
	}()

	finalModel, runErr := p.Run()
	if runErr != nil {
		return "", "", nil, fmt.Errorf("tui error: %w", runErr)
	}

	m, ok := finalModel.(Model)
	if !ok {
		return "", "", nil, fmt.Errorf("unexpected model type")
	}

	if m.Aborted || m.Selected == nil {
		return "", "", nil, ErrAbort
	}

	var dynStore storetypes.KubeconfigStore
	if ds, ok := m.dynamicStores[m.Selected.storeID]; ok {
		dynStore = ds
	}

	return m.Selected.path, m.Selected.contextName, dynStore, nil
}
