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

	gardenerstore "github.com/MichaelSp/kswitch/pkg/store/gardener"
	"github.com/MichaelSp/kswitch/types"
)

// FormatDisplayName builds a human-readable display name for the TUI list.
// Returns the primary name and an optional dim suffix (the "(…)" part).
//
// For Gardener contexts the format is:
//
//	primary: gardener/<landscape>/<namespace>/<shoot-name>
//	suffix:  (alias-or-context)
//
// For all other store kinds the raw contextName is returned (with the alias
// appended in parentheses when it differs).
func FormatDisplayName(storeKind types.StoreKind, path, contextName, alias string) (primary, suffix string) {
	if storeKind == types.StoreKindGardener {
		if p, s, ok := formatGardener(path, contextName, alias); ok {
			return p, s
		}
	}

	// Generic fallback: show context name; if it has an alias that differs,
	// append it so users can still search by either name.
	if alias != "" && alias != contextName {
		return alias, fmt.Sprintf("(%s)", contextName)
	}
	return contextName, ""
}

// formatGardener parses a Gardener identifier and returns the pretty display.
// Returns (primary, suffix, true) on success or ("", "", false) if unparseable.
func formatGardener(path, contextName, alias string) (primary, suffix string, ok bool) {
	landscape, resource, name, namespace, _, err := gardenerstore.ParseIdentifier(path)
	if err != nil {
		return "", "", false
	}

	var primary_ string
	switch resource {
	case gardenerstore.GardenerResourceShoot:
		primary_ = fmt.Sprintf("%s/%s/%s/%s", "gardener", landscape, namespace, name)
	case gardenerstore.GardenerResourceSeed:
		primary_ = fmt.Sprintf("%s/%s/garden/%s", "gardener", landscape, name)
	default:
		return "", "", false
	}

	// Collect all "other names" the user might know this cluster by.
	others := collectOtherNames(name, contextName, alias)
	if len(others) > 0 {
		return primary_, fmt.Sprintf("(%s)", strings.Join(others, ", ")), true
	}
	return primary_, "", true
}

// collectOtherNames returns the names that differ from the primary display name
// (the shoot/seed name) so users can also search by alias or context name.
func collectOtherNames(primaryName, contextName, alias string) []string {
	seen := map[string]bool{primaryName: true}
	var others []string
	for _, n := range []string{alias, contextName} {
		if n != "" && !seen[n] {
			seen[n] = true
			others = append(others, n)
		}
	}
	return others
}
