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
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// item represents a discovered kubeconfig context entry.
type item struct {
	displayName string // rich display name shown in the TUI list
	contextName string // actual context name (or alias) used for kubeconfig lookup
	path        string
	tags        map[string]string
	storeID     string
}

// itemsMsg is sent by the discovery goroutine with a batch of new items.
type itemsMsg []item

// discoveryDoneMsg signals that all stores have finished searching.
type discoveryDoneMsg struct{}

// previewMsg carries the fetched preview text for the currently selected item.
type previewMsg struct {
	content string
	forPath string
}

// Model is the bubbletea model for the kswitch TUI.
type Model struct {
	stores      map[string]storetypes.KubeconfigStore
	showPreview bool

	input textinput.Model
	query string

	allItems []item
	filtered []item
	cursor   int

	previewContent string
	previewForPath string // path the current preview belongs to

	loading  bool
	Aborted  bool
	Selected *item

	width  int
	height int
}

var (
	stylePrompt   = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	styleCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleCount    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleBorder   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stylePreview  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	styleLoading  = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Italic(true)
)

// NewModel creates an initial TUI model.
func NewModel(stores map[string]storetypes.KubeconfigStore, showPreview bool) Model {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()
	ti.Prompt = "> "
	ti.PromptStyle = stylePrompt
	ti.TextStyle = lipgloss.NewStyle().Bold(true)

	return Model{
		stores:      stores,
		showPreview: showPreview,
		input:       ti,
		loading:     true,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case itemsMsg:
		m.allItems = append(m.allItems, []item(msg)...)
		m.filtered = filterItems(m.query, m.allItems)
		m.clampCursor()
		return m, m.fetchPreviewCmd()

	case discoveryDoneMsg:
		m.loading = false
		return m, nil

	case previewMsg:
		if msg.forPath == m.previewForPath {
			m.previewContent = msg.content
		}
		return m, nil

	case tea.KeyMsg:
		// Navigation keys take priority over textinput
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC, tea.KeyCtrlD:
			m.Aborted = true
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				sel := m.filtered[m.cursor]
				m.Selected = &sel
			}
			return m, tea.Quit

		case tea.KeyUp, tea.KeyCtrlK, tea.KeyCtrlP:
			m.moveCursor(1)
			return m, m.fetchPreviewCmd()

		case tea.KeyDown, tea.KeyCtrlJ, tea.KeyCtrlN:
			m.moveCursor(-1)
			return m, m.fetchPreviewCmd()

		case tea.KeyPgUp:
			m.moveCursor(m.listHeight())
			return m, m.fetchPreviewCmd()

		case tea.KeyPgDown:
			m.moveCursor(-m.listHeight())
			return m, m.fetchPreviewCmd()

		case tea.KeyCtrlU:
			m.input.SetValue("")
			m.query = ""
			m.filtered = filterItems("", m.allItems)
			m.cursor = 0
			return m, m.fetchPreviewCmd()

		case tea.KeyCtrlW:
			m.deleteWord()
			m.refilter()
			return m, m.fetchPreviewCmd()

		case tea.KeyBackspace:
			if msg.Alt {
				m.deleteWord()
				m.refilter()
				return m, m.fetchPreviewCmd()
			}
		}

		// Delegate to textinput for character input & standard backspace
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		newQuery := m.input.Value()
		if newQuery != m.query {
			m.query = newQuery
			m.refilter()
			return m, tea.Batch(cmd, m.fetchPreviewCmd())
		}
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	listW := m.width
	if m.showPreview && m.width > 60 {
		listW = m.width * 2 / 5
	}

	left := m.renderLeft(listW)

	if !m.showPreview || m.width <= 60 {
		return left
	}

	previewW := m.width - listW
	right := m.renderRight(previewW)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderLeft(width int) string {
	lh := m.listHeight()

	// fzf-style: cursor (best match) sits at the bottom, less-relevant items
	// above it. Items with higher indices in filtered[] are displayed higher up.
	// Window: show items [cursor .. cursor+lh-1], rendered top-to-bottom in
	// reverse order so cursor lands on the bottom row.
	start := m.cursor
	end := start + lh
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	// build rows in reverse (highest index first = topmost row)
	rows := make([]string, 0, lh)
	for i := end - 1; i >= start; i-- {
		it := m.filtered[i]
		name := truncate(it.displayName, width-3)
		if i == m.cursor {
			rows = append(rows, styleCursor.Render("> ")+styleSelected.Render(name))
		} else {
			rows = append(rows, styleDim.Render("  ")+name)
		}
	}

	// Pad at the top so items are bottom-aligned when fewer than lh exist
	for len(rows) < lh {
		rows = append([]string{""}, rows...)
	}

	// Separator line
	sep := styleBorder.Render(strings.Repeat("─", width))

	// Input line with count
	countStr := styleCount.Render(fmt.Sprintf("%d/%d", len(m.filtered), len(m.allItems)))
	loadStr := ""
	if m.loading {
		loadStr = styleLoading.Render(" …")
	}
	inputLine := m.input.View() + "  " + countStr + loadStr

	return strings.Join(rows, "\n") + "\n" + sep + "\n" + inputLine
}

func (m Model) renderRight(width int) string {
	if m.previewContent == "" {
		if len(m.filtered) == 0 {
			return ""
		}
		return styleDim.Render("loading preview…")
	}

	lines := strings.Split(m.previewContent, "\n")
	maxLines := m.height - 1
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	// Truncate each line to fit
	for i, l := range lines {
		lines[i] = truncate(l, width-2)
	}

	border := styleBorder.Render("│")
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = border + " " + stylePreview.Render(l)
	}
	return strings.Join(out, "\n")
}

// listHeight returns the number of rows available for the item list.
// Layout: listHeight rows + 1 separator + 1 input = m.height
func (m Model) listHeight() int {
	h := m.height - 2 // subtract separator and input line
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *Model) refilter() {
	m.filtered = filterItems(m.query, m.allItems)
	m.cursor = 0
}

// deleteWord removes the last word from the textinput (Alt+Backspace / ctrl+w).
func (m *Model) deleteWord() {
	v := m.input.Value()
	if len(v) == 0 {
		return
	}
	// Trim trailing spaces, then find the last space boundary
	trimmed := strings.TrimRight(v, " ")
	idx := strings.LastIndex(trimmed, " ")
	var newVal string
	if idx < 0 {
		newVal = ""
	} else {
		newVal = v[:idx+1]
	}
	m.input.SetValue(newVal)
	m.input.CursorEnd()
	m.query = newVal
}

func (m *Model) fetchPreviewCmd() tea.Cmd {
	if !m.showPreview || len(m.filtered) == 0 {
		return nil
	}
	sel := m.filtered[m.cursor]
	if sel.path == m.previewForPath {
		return nil // already showing correct preview
	}
	m.previewForPath = sel.path
	m.previewContent = ""
	return fetchPreview(m.stores, sel)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 2 {
		return string(r[:max])
	}
	return string(r[:max-2]) + ".."
}
