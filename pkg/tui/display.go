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
//
// For Gardener contexts the format is:
//
//	gardener/<landscape>/<namespace>/<shoot-name> (alias-or-context)
//
// For all other store kinds the raw contextName is returned (with the alias
// appended in parentheses when it differs).
func FormatDisplayName(storeKind types.StoreKind, path, contextName, alias string) string {
	if storeKind == types.StoreKindGardener {
		if display, ok := formatGardener(path, contextName, alias); ok {
			return display
		}
	}

	// Generic fallback: show context name; if it has an alias that differs,
	// append it so users can still search by either name.
	if alias != "" && alias != contextName {
		return fmt.Sprintf("%s (%s)", alias, contextName)
	}
	return contextName
}

// formatGardener parses a Gardener identifier and returns the pretty display.
// Returns (display, true) on success or ("", false) if the path cannot be parsed.
func formatGardener(path, contextName, alias string) (string, bool) {
	landscape, resource, name, namespace, _, err := gardenerstore.ParseIdentifier(path)
	if err != nil {
		return "", false
	}

	var suffix string
	switch resource {
	case gardenerstore.GardenerResourceShoot:
		suffix = fmt.Sprintf("%s/%s/%s/%s", "gardener", landscape, namespace, name)
	case gardenerstore.GardenerResourceSeed:
		suffix = fmt.Sprintf("%s/%s/garden/%s", "gardener", landscape, name)
	default:
		return "", false
	}

	// Collect all "other names" the user might know this cluster by.
	// These are appended in parentheses to aid fuzzy search and recognition.
	others := collectOtherNames(name, contextName, alias)
	if len(others) > 0 {
		return fmt.Sprintf("%s (%s)", suffix, strings.Join(others, ", ")), true
	}
	return suffix, true
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
