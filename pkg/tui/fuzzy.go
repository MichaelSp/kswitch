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

import "github.com/sahilm/fuzzy"

// filterItems returns items matching query using fuzzy matching.
// If query is empty, all items are returned in original order.
func filterItems(query string, items []item) []item {
	if query == "" {
		out := make([]item, len(items))
		copy(out, items)
		return out
	}

	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.displayName
	}

	matches := fuzzy.Find(query, names)
	out := make([]item, 0, len(matches))
	for _, m := range matches {
		out = append(out, items[m.Index])
	}
	return out
}
