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

package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newTestModel returns a Model pre-sized so View() does not return a blank screen.
func newTestModel(items ...string) Model {
	m := NewModel(nil, false)
	m.width = 80
	m.height = 24
	for _, name := range items {
		m.allItems = append(m.allItems, item{displayName: name, contextName: name})
	}
	m.filtered = filterItems("", m.allItems)
	return m
}

// sendKey is a convenience helper that feeds a synthetic KeyPressMsg into the
// model and returns the updated model.
func sendKey(t *testing.T, m Model, keyStr string) Model {
	t.Helper()
	// Build a KeyPressMsg whose String() returns keyStr.
	// For printable single-character keys we set Code + Text; for everything
	// else (multi-rune names like "ctrl+v") String() looks at Code+Mod but
	// the simplest approach is to use a rune code that matches the string.
	msg := tea.KeyPressMsg{}
	switch keyStr {
	case "ctrl+u":
		msg.Code = 'u'
		msg.Mod = tea.ModCtrl
	case "ctrl+v":
		msg.Code = 'v'
		msg.Mod = tea.ModCtrl
	case "ctrl+w":
		msg.Code = 'w'
		msg.Mod = tea.ModCtrl
	case "enter":
		msg.Code = tea.KeyEnter
	case "esc":
		msg.Code = tea.KeyEsc
	default:
		// single printable character
		r := []rune(keyStr)
		if len(r) == 1 {
			msg.Code = r[0]
			msg.Text = keyStr
		}
	}
	updated, _ := m.Update(msg)
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned unexpected type %T", updated)
	}
	return m2
}

// ----- stray-"t" fix: placeholder must not appear in View output ------------

// TestNoStrayT_EmptyQuery checks that when the search expression is empty the
// rendered input line does NOT show "t" as the first visible character of the
// cursor/placeholder (the old bug: bubbles rendered the first rune of the
// placeholder text as a cursor character that appeared as a literal "t" in
// some terminals).
func TestNoStrayT_EmptyQuery(t *testing.T) {
	m := newTestModel("prod", "staging", "dev")
	view := m.renderLeft(80)

	// The input line is the last line of renderLeft's output.
	lines := strings.Split(view, "\n")
	inputLine := lines[len(lines)-1]

	// The input line should start with the styled prompt ("> "), NOT with "t".
	// Strip ANSI escapes for a clean check.
	plain := stripANSI(inputLine)
	if strings.HasPrefix(plain, "t") {
		t.Errorf("input line starts with 't' (stray placeholder char): %q", plain)
	}
}

// TestHintTextAppended verifies that when the query is empty the hint
// "type to filter..." appears in the input line (rendered as dim text, not as
// the textinput placeholder).
func TestHintTextAppended_EmptyQuery(t *testing.T) {
	m := newTestModel("prod")
	view := m.renderLeft(80)
	if !strings.Contains(stripANSI(view), "type to filter...") {
		t.Error("expected 'type to filter...' hint in view when query is empty")
	}
}

// TestHintTextGone_NonEmptyQuery verifies the hint disappears once the user
// types something (to avoid cluttering the line with both typed text and the hint).
func TestHintTextGone_NonEmptyQuery(t *testing.T) {
	m := newTestModel("prod", "staging")
	m = sendKey(t, m, "p")
	view := m.renderLeft(80)
	if strings.Contains(stripANSI(view), "type to filter...") {
		t.Error("hint 'type to filter...' should not appear after user has typed")
	}
}

// ----- paste fix: tea.PasteMsg must reach the textinput ---------------------

// TestPasteMsg_UpdatesQuery verifies that a tea.PasteMsg (terminal
// bracketed-paste event, e.g. CMD+V on macOS) inserts its content into the
// search query and triggers re-filtering.
func TestPasteMsg_UpdatesQuery(t *testing.T) {
	m := newTestModel("production", "staging", "dev")

	pasteText := "prod"
	updated, _ := m.Update(tea.PasteMsg{Content: pasteText})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned unexpected type %T", updated)
	}
	m = m2

	if m.query != pasteText {
		t.Errorf("query after paste: got %q, want %q", m.query, pasteText)
	}
}

// TestPasteMsg_TriggersRefilter verifies that after a bracketed paste the
// filtered list reflects the new query.
func TestPasteMsg_TriggersRefilter(t *testing.T) {
	m := newTestModel("production", "staging", "dev")

	updated, _ := m.Update(tea.PasteMsg{Content: "prod"})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned unexpected type %T", updated)
	}
	m = m2

	for _, it := range m.filtered {
		if !strings.Contains(strings.ToLower(it.displayName), "prod") {
			// fuzzy match — just ensure "staging" alone (no "prod") is absent
			if it.displayName == "staging" {
				t.Errorf("'staging' should not match query 'prod', filtered list: %+v", m.filtered)
			}
		}
	}
	if len(m.filtered) == 0 {
		t.Error("expected at least one match after pasting 'prod'")
	}
}

// TestPasteMsg_EmptyContent is a no-op guard: pasting an empty string must
// not crash and must leave the query unchanged.
func TestPasteMsg_EmptyContent(t *testing.T) {
	m := newTestModel("prod")
	updated, _ := m.Update(tea.PasteMsg{Content: ""})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned unexpected type %T", updated)
	}
	m = m2
	if m.query != "" {
		t.Errorf("empty paste should not change query, got %q", m.query)
	}
}

// TestPasteMsg_AppendsToExistingQuery verifies that pasting while there is
// already typed text appends to it (the textinput handles the concatenation).
func TestPasteMsg_AppendsToExistingQuery(t *testing.T) {
	m := newTestModel("production", "staging")
	// First type "pro" manually
	m = sendKey(t, m, "p")
	m = sendKey(t, m, "r")
	m = sendKey(t, m, "o")
	// Then paste "d"
	updated, _ := m.Update(tea.PasteMsg{Content: "d"})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned unexpected type %T", updated)
	}
	m = m2

	if !strings.HasPrefix(m.query, "pro") {
		t.Errorf("expected query to start with 'pro' after typing then pasting, got %q", m.query)
	}
	if !strings.Contains(m.query, "d") {
		t.Errorf("expected pasted 'd' in query, got %q", m.query)
	}
}

// stripANSI removes ANSI escape sequences from s so test assertions are
// independent of terminal colour codes.
func stripANSI(s string) string {
	var out strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			// ESC sequences end at a letter in the range 0x40–0x7E
			if s[i] >= 0x40 && s[i] <= 0x7E {
				inEscape = false
			}
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}
