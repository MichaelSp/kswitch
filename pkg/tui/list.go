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
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

// listEntry is a filtered list row that remembers the index it came from in
// the original (unfiltered) items slice. We track the index so that selection
// is unambiguous even when two entries share the same display label.
type listEntry struct {
	displayName    string
	origIndex      int
	matchedIndexes []int // positions in displayName matched by the fuzzy query
}

// listModel is a minimal bubbletea model for simple string-list selection.
type listModel struct {
	input         textinput.Model
	query         string
	items         []string
	filtered      []listEntry
	cursor        int
	width         int
	height        int
	Aborted       bool
	Selected      string
	SelectedIndex int
}

func newListModel(items []string) listModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()
	ti.Prompt = "> "
	ti.PromptStyle = stylePrompt
	ti.TextStyle = stderrRenderer.NewStyle().Bold(true)

	filtered := make([]listEntry, len(items))
	for i, s := range items {
		filtered[i] = listEntry{displayName: s, origIndex: i}
	}

	return listModel{
		input:         ti,
		items:         items,
		filtered:      filtered,
		SelectedIndex: -1,
	}
}

func (m listModel) Init() tea.Cmd { return textinput.Blink }

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC, tea.KeyCtrlD:
			m.Aborted = true
			return m, tea.Quit
		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.Selected = m.filtered[m.cursor].displayName
				m.SelectedIndex = m.filtered[m.cursor].origIndex
			}
			return m, tea.Quit
		case tea.KeyUp, tea.KeyCtrlK, tea.KeyCtrlP:
			m.moveCursor(-1)
			return m, nil
		case tea.KeyDown, tea.KeyCtrlJ, tea.KeyCtrlN:
			m.moveCursor(1)
			return m, nil
		case tea.KeyCtrlU:
			m.input.SetValue("")
			m.query = ""
			m.filtered = filterStringItems("", m.items)
			m.cursor = 0
			return m, nil
		case tea.KeyCtrlW:
			m.deleteWord()
			return m, nil
		case tea.KeyBackspace:
			if msg.Alt {
				m.deleteWord()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if v := m.input.Value(); v != m.query {
		m.query = v
		m.filtered = filterStringItems(v, m.items)
		m.cursor = 0
	}
	return m, cmd
}

func (m listModel) View() string {
	if m.width == 0 {
		return ""
	}
	lh := max(m.height-2, 1)

	// fzf-style: cursor at bottom, higher-index items above.
	start := max(m.cursor, 0)
	end := min(start+lh, len(m.filtered))

	rows := make([]string, 0, lh)
	for i := end - 1; i >= start; i-- {
		name := truncate(m.filtered[i].displayName, m.width-3)
		if i == m.cursor {
			rows = append(rows, styleCursor.Render("> ")+highlightMatches(name, m.filtered[i].matchedIndexes, styleSelected, styleSelected))
		} else {
			rows = append(rows, styleDim.Render("  ")+highlightMatches(name, m.filtered[i].matchedIndexes, stderrRenderer.NewStyle(), styleMatch))
		}
	}
	for len(rows) < lh {
		rows = append([]string{""}, rows...)
	}

	sep := styleBorder.Render(strings.Repeat("─", m.width))
	countStr := styleCount.Render(fmt.Sprintf("%d/%d", len(m.filtered), len(m.items)))
	inputLine := m.input.View() + "  " + countStr

	return strings.Join(rows, "\n") + "\n" + sep + "\n" + inputLine
}

func (m *listModel) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *listModel) deleteWord() {
	v := m.input.Value()
	trimmed := strings.TrimRight(v, " ")
	idx := strings.LastIndex(trimmed, " ")
	var nv string
	if idx >= 0 {
		nv = v[:idx+1]
	}
	m.input.SetValue(nv)
	m.input.CursorEnd()
	m.query = nv
	m.filtered = filterStringItems(nv, m.items)
	m.cursor = 0
}

func filterStringItems(query string, items []string) []listEntry {
	if query == "" {
		out := make([]listEntry, len(items))
		for i, s := range items {
			out[i] = listEntry{displayName: s, origIndex: i}
		}
		return out
	}
	matches := fuzzy.Find(query, items)
	out := make([]listEntry, 0, len(matches))
	for _, m := range matches {
		out = append(out, listEntry{displayName: items[m.Index], origIndex: m.Index, matchedIndexes: m.MatchedIndexes})
	}
	return out
}

// RunList shows a simple interactive fuzzy list for the given string slice.
// Returns the selected string index (into the original items slice) and the
// selected string, or ErrAbort if the user cancelled.
func RunList(items []string, labelFunc func(i int) string) (int, error) {
	labels := make([]string, len(items))
	for i := range items {
		if labelFunc != nil {
			labels[i] = labelFunc(i)
		} else {
			labels[i] = items[i]
		}
	}

	model := newListModel(labels)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(os.Stderr))

	final, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("tui list error: %w", err)
	}

	m, ok := final.(listModel)
	if !ok {
		return 0, fmt.Errorf("unexpected model type")
	}
	if m.Aborted || m.SelectedIndex < 0 {
		return 0, ErrAbort
	}

	return m.SelectedIndex, nil
}
