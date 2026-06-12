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
	"github.com/charmbracelet/lipgloss"
)

// listModel is a minimal bubbletea model for simple string-list selection.
type listModel struct {
	input    textinput.Model
	query    string
	items    []string
	filtered []string
	cursor   int
	offset   int
	width    int
	height   int
	Aborted  bool
	Selected string
}

func newListModel(items []string) listModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()
	ti.Prompt = "> "
	ti.PromptStyle = stylePrompt
	ti.TextStyle = lipgloss.NewStyle().Bold(true)

	filtered := make([]string, len(items))
	copy(filtered, items)

	return listModel{
		input:    ti,
		items:    items,
		filtered: filtered,
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
				m.Selected = m.filtered[m.cursor]
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
			m.filtered = append([]string{}, m.items...)
			m.cursor, m.offset = 0, 0
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
		m.cursor, m.offset = 0, 0
	}
	return m, cmd
}

func (m listModel) View() string {
	if m.width == 0 {
		return ""
	}
	lh := m.height - 2
	if lh < 1 {
		lh = 1
	}

	// fzf-style: cursor at bottom, higher-index items above.
	start := m.cursor
	end := start + lh
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	rows := make([]string, 0, lh)
	for i := end - 1; i >= start; i-- {
		name := truncate(m.filtered[i], m.width-3)
		if i == m.cursor {
			rows = append(rows, styleCursor.Render("> ")+styleSelected.Render(name))
		} else {
			rows = append(rows, styleDim.Render("  ")+name)
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
	m.cursor, m.offset = 0, 0
}

func filterStringItems(query string, items []string) []string {
	if query == "" {
		out := make([]string, len(items))
		copy(out, items)
		return out
	}
	tmpItems := make([]item, len(items))
	for i, s := range items {
		tmpItems[i] = item{displayName: s}
	}
	filtered := filterItems(query, tmpItems)
	out := make([]string, len(filtered))
	for i, it := range filtered {
		out[i] = it.displayName
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
	if m.Aborted || m.Selected == "" {
		return 0, ErrAbort
	}

	// Find the index in the original labels slice
	for i, l := range labels {
		if l == m.Selected {
			return i, nil
		}
	}
	return 0, ErrAbort
}
