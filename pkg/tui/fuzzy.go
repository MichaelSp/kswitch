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

	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// filterItems returns items matching query using fuzzy matching.
// If query is empty, all items are returned in original order.
// Matching runs on the full search string (primary + suffix) so users can
// search by either the display name or the technical name in parentheses.
func filterItems(query string, items []item) []item {
	if query == "" {
		out := make([]item, len(items))
		copy(out, items)
		for i := range out {
			out[i].matchedIndexes = nil
		}
		return out
	}

	names := make([]string, len(items))
	for i, it := range items {
		if it.dimSuffix != "" {
			names[i] = it.displayName + " " + it.dimSuffix
		} else {
			names[i] = it.displayName
		}
	}

	matches := fuzzy.Find(query, names)
	out := make([]item, 0, len(matches))
	for _, m := range matches {
		it := items[m.Index]
		it.matchedIndexes = m.MatchedIndexes
		out = append(out, it)
	}
	return out
}

// highlightMatches renders s with matched character positions highlighted using
// hlStyle, and the rest of the text rendered with baseStyle.
func highlightMatches(s string, indexes []int, baseStyle, hlStyle lipgloss.Style) string {
	if len(indexes) == 0 {
		return baseStyle.Render(s)
	}

	matched := make(map[int]bool, len(indexes))
	for _, idx := range indexes {
		matched[idx] = true
	}

	runes := []rune(s)
	var sb strings.Builder
	inMatch := false
	segStart := 0

	flush := func(end int, wasMatch bool) {
		if segStart >= end {
			return
		}
		seg := string(runes[segStart:end])
		if wasMatch {
			sb.WriteString(hlStyle.Render(seg))
		} else {
			sb.WriteString(baseStyle.Render(seg))
		}
	}

	for i := range runes {
		isMatch := matched[i]
		if i == 0 {
			inMatch = isMatch
			continue
		}
		if isMatch != inMatch {
			flush(i, inMatch)
			segStart = i
			inMatch = isMatch
		}
	}
	flush(len(runes), inMatch)
	return sb.String()
}
