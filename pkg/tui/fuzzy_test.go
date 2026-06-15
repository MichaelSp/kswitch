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
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// noopStyle renders the string without any escape codes, making test assertions
// independent of the concrete style colours/attributes.
var noopStyle = lipgloss.NewStyle()

// TestStderrRendererUsed guards against the regression where package-level
// styles were created with lipgloss.NewStyle() (default renderer, probes
// os.Stdout) instead of stderrRenderer. When the shell wrapper captures stdout
// the default renderer sees NoColor and strips all ANSI — the TUI loses its
// yellow fuzzy-match highlights.
func TestStderrRendererUsed(t *testing.T) {
	// stderrRenderer must not be the same object as the lipgloss default renderer.
	// lipgloss.DefaultRenderer() returns the package-level default; our renderer
	// is bound to os.Stderr so it must be a distinct instance.
	if stderrRenderer == lipgloss.DefaultRenderer() {
		t.Error("stderrRenderer must be a renderer bound to os.Stderr, not the lipgloss default renderer")
	}
}

// ---- highlightMatches -------------------------------------------------------

func TestHighlightMatches_NoIndexes(t *testing.T) {
	result := highlightMatches("hello", nil, noopStyle, noopStyle)
	if !strings.Contains(result, "hello") {
		t.Errorf("expected output to contain %q, got %q", "hello", result)
	}
}

func TestHighlightMatches_EmptyString(t *testing.T) {
	result := highlightMatches("", []int{0}, noopStyle, noopStyle)
	// Empty string – nothing to render; should not panic.
	_ = result
}

func TestHighlightMatches_AllMatched(t *testing.T) {
	// When every character matches, the whole string is rendered with hlStyle.
	// With noopStyle for both styles the output should still contain all runes.
	result := highlightMatches("abc", []int{0, 1, 2}, noopStyle, noopStyle)
	if !strings.Contains(result, "abc") {
		t.Errorf("expected output to contain %q, got %q", "abc", result)
	}
}

func TestHighlightMatches_FirstCharMatched(t *testing.T) {
	// indexes = [0]: first rune highlighted, rest default
	result := highlightMatches("abc", []int{0}, noopStyle, noopStyle)
	if !strings.Contains(result, "a") || !strings.Contains(result, "bc") {
		t.Errorf("expected both segments in output, got %q", result)
	}
}

func TestHighlightMatches_LastCharMatched(t *testing.T) {
	result := highlightMatches("abc", []int{2}, noopStyle, noopStyle)
	if !strings.Contains(result, "ab") || !strings.Contains(result, "c") {
		t.Errorf("expected both segments in output, got %q", result)
	}
}

func TestHighlightMatches_MiddleCharsMatched(t *testing.T) {
	// "production" with indexes {3,4} → "pro" base, "du" hl, "ction" base
	result := highlightMatches("production", []int{3, 4}, noopStyle, noopStyle)
	for _, want := range []string{"pro", "du", "ction"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected segment %q in output, got %q", want, result)
		}
	}
}

func TestHighlightMatches_NonContiguousIndexes(t *testing.T) {
	// "kubectl" with indexes {0, 3} → alternating highlight/base segments
	result := highlightMatches("kubectl", []int{0, 3}, noopStyle, noopStyle)
	if !strings.Contains(result, "kubectl") {
		// All characters must appear in the output (order preserved)
		t.Errorf("output must contain all original chars, got %q", result)
	}
}

func TestHighlightMatches_UnicodeSafe(t *testing.T) {
	// Multi-byte rune: "héllo" – index 1 ('é', U+00E9) is matched
	result := highlightMatches("héllo", []int{1}, noopStyle, noopStyle)
	if !strings.Contains(result, "h") || !strings.Contains(result, "é") || !strings.Contains(result, "llo") {
		t.Errorf("unicode segments not preserved, got %q", result)
	}
}

// ---- filterItems ------------------------------------------------------------

func makeItems(names ...string) []item {
	out := make([]item, len(names))
	for i, n := range names {
		out[i] = item{displayName: n, contextName: n}
	}
	return out
}

func TestFilterItems_EmptyQuery_ReturnsAll(t *testing.T) {
	items := makeItems("prod", "staging", "dev")
	got := filterItems("", items)
	if len(got) != len(items) {
		t.Fatalf("expected %d items, got %d", len(items), len(got))
	}
}

func TestFilterItems_EmptyQuery_ClearsMatchedIndexes(t *testing.T) {
	items := makeItems("prod", "staging")
	items[0].matchedIndexes = []int{0, 1}
	got := filterItems("", items)
	for _, it := range got {
		if it.matchedIndexes != nil {
			t.Errorf("matchedIndexes should be nil after empty query, got %v", it.matchedIndexes)
		}
	}
}

func TestFilterItems_EmptyQuery_PreservesOrder(t *testing.T) {
	items := makeItems("zeta", "alpha", "beta")
	got := filterItems("", items)
	for i, it := range got {
		if it.displayName != items[i].displayName {
			t.Errorf("order changed at index %d: want %q, got %q", i, items[i].displayName, it.displayName)
		}
	}
}

func TestFilterItems_QueryFiltersResults(t *testing.T) {
	items := makeItems("production", "staging", "dev-production")
	got := filterItems("prod", items)
	if len(got) == 0 {
		t.Fatal("expected at least one match for query 'prod'")
	}
	for _, it := range got {
		if !strings.Contains(strings.ToLower(it.displayName), "prod") {
			// fuzzy matching so just check the matched items have the right field
			if it.matchedIndexes == nil {
				t.Errorf("matched item %q should have non-nil matchedIndexes", it.displayName)
			}
		}
	}
}

func TestFilterItems_QueryPopulatesMatchedIndexes(t *testing.T) {
	items := makeItems("production", "staging")
	got := filterItems("prod", items)
	for _, it := range got {
		if it.displayName == "production" && len(it.matchedIndexes) == 0 {
			t.Errorf("expected matchedIndexes to be populated for %q", it.displayName)
		}
	}
}

func TestFilterItems_NoMatch_ReturnsEmpty(t *testing.T) {
	items := makeItems("apple", "banana")
	got := filterItems("zzz", items)
	if len(got) != 0 {
		t.Errorf("expected no matches, got %d", len(got))
	}
}

func TestFilterItems_DoesNotMutateOriginal(t *testing.T) {
	items := makeItems("production")
	_ = filterItems("prod", items)
	if items[0].matchedIndexes != nil {
		t.Error("filterItems must not mutate the original items slice")
	}
}

// ---- filterStringItems ------------------------------------------------------

func TestFilterStringItems_EmptyQuery_ReturnsAll(t *testing.T) {
	strs := []string{"prod", "staging", "dev"}
	got := filterStringItems("", strs)
	if len(got) != len(strs) {
		t.Fatalf("expected %d entries, got %d", len(strs), len(got))
	}
}

func TestFilterStringItems_EmptyQuery_NilMatchedIndexes(t *testing.T) {
	strs := []string{"prod", "staging"}
	got := filterStringItems("", strs)
	for _, e := range got {
		if e.matchedIndexes != nil {
			t.Errorf("matchedIndexes should be nil for empty query, got %v", e.matchedIndexes)
		}
	}
}

func TestFilterStringItems_EmptyQuery_OrigIndexMatchesPosition(t *testing.T) {
	strs := []string{"a", "b", "c"}
	got := filterStringItems("", strs)
	for i, e := range got {
		if e.origIndex != i {
			t.Errorf("origIndex[%d] = %d, want %d", i, e.origIndex, i)
		}
	}
}

func TestFilterStringItems_QueryPopulatesMatchedIndexes(t *testing.T) {
	strs := []string{"production", "staging", "dev"}
	got := filterStringItems("prod", strs)
	if len(got) == 0 {
		t.Fatal("expected at least one match")
	}
	for _, e := range got {
		if len(e.matchedIndexes) == 0 {
			t.Errorf("matchedIndexes should be non-empty for matched entry %q", e.displayName)
		}
	}
}

func TestFilterStringItems_QueryNoMatch_ReturnsEmpty(t *testing.T) {
	strs := []string{"apple", "banana"}
	got := filterStringItems("zzz", strs)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got))
	}
}
