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

package list_contexts

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/MichaelSp/kswitch/pkg"
	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/pkg/tui"
	"github.com/MichaelSp/kswitch/types"
)

var logger = logrus.New()

// ListEntry is a single context with its human-readable display information.
type ListEntry struct {
	// Name is the context name (or alias) used for selection.
	Name string
	// Display is the formatted primary display name (e.g. gardener/canary/ns/shoot).
	Display string
	// Suffix is the dim "(…)" part shown next to the display name.
	Suffix string
}

func ListContexts(pattern string, stores []storetypes.KubeconfigStore, config *types.Config, stateDir string, noIndex bool) ([]string, error) {
	return ListContextsFiltered(pattern, false, stores, config, stateDir, noIndex)
}

// ListContextsRegexp filters contexts using a regular expression instead of a wildcard pattern.
func ListContextsRegexp(pattern string, stores []storetypes.KubeconfigStore, config *types.Config, stateDir string, noIndex bool) ([]string, error) {
	return ListContextsFiltered(pattern, true, stores, config, stateDir, noIndex)
}

func ListContextsFiltered(pattern string, isRegexp bool, stores []storetypes.KubeconfigStore, config *types.Config, stateDir string, noIndex bool) ([]string, error) {
	entries, err := ListContextsVerbose(pattern, isRegexp, stores, config, stateDir, noIndex)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names, nil
}

// ListContextsVerbose returns full display information for each matching context.
func ListContextsVerbose(pattern string, isRegexp bool, stores []storetypes.KubeconfigStore, config *types.Config, stateDir string, noIndex bool) ([]ListEntry, error) {
	var c *chan pkg.DiscoveredContext
	var err error
	if noIndex {
		c, err = pkg.DoSearch(stores, config, stateDir, true)
	} else {
		c, err = pkg.DoSearchFromIndex(stores, config, stateDir)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list contexts: %w", err)
	}

	// collect with dedup + external-prefer filters applied
	var all []pkg.DiscoveredContext
	for dc := range *c {
		if dc.Error != nil {
			logger.Warnf("cannot list contexts. Error returned from search: %v", dc.Error)
			continue
		}
		all = append(all, dc)
	}
	all = pkg.ApplyContextFilters(all)

	var entries []ListEntry
	for _, dc := range all {
		store := *dc.Store
		name := dc.Name
		if dc.Alias != "" {
			name = dc.Alias
		}

		// match against both the resolved name (alias if set, else context name) and
		// the raw context name, so agents can grep for aliases without knowing which
		// form a context was stored under.
		candidates := []string{name}
		if dc.Alias != "" && dc.Alias != dc.Name {
			candidates = append(candidates, dc.Name)
		}

		matched := false
		for _, candidate := range candidates {
			var ok bool
			var err error
			if isRegexp {
				ok, err = matchesRegexp(pattern, candidate)
			} else {
				ok, err = matchesPattern(pattern, candidate)
			}
			if err != nil {
				logger.Warnf("invalid pattern %q: %v", pattern, err)
				break
			}
			if ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		ldk := labelDisplayKeys(store)
		display, suffix := tui.FormatDisplayName(store.GetKind(), dc.Path, dc.Name, dc.Alias, dc.Tags, ldk)
		entries = append(entries, ListEntry{Name: name, Display: display, Suffix: suffix})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Display+entries[i].Suffix < entries[j].Display+entries[j].Suffix
	})
	return entries, nil
}

type labelKeysProvider interface{ GetShootLabelKeys() []string }

func labelDisplayKeys(store storetypes.KubeconfigStore) []string {
	if p, ok := store.(labelKeysProvider); ok {
		return p.GetShootLabelKeys()
	}
	return nil
}

// matchesRegexp reports whether name matches the regular expression pattern against the full name or any '/'-separated segment.
func matchesRegexp(pattern, name string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	if re.MatchString(name) {
		return true, nil
	}
	for seg := range strings.SplitSeq(name, "/") {
		if re.MatchString(seg) {
			return true, nil
		}
	}
	return false, nil
}

// matchesPattern reports whether name matches the wildcard pattern.
// Unlike path.Match, '*' matches across '/' separators so patterns like "*"
// correctly match Gardener context names such as "ns/garden-shoot-external".
func matchesPattern(pattern, name string) (bool, error) {
	if pattern == "*" {
		return true, nil
	}
	// Try direct match first (works for names without slashes or segment patterns).
	if ok, err := path.Match(pattern, name); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	// Also match against each '/'-separated segment so patterns like "*-dev*"
	// work on the last segment of a context name like "ns/ctx-dev-external".
	for seg := range strings.SplitSeq(name, "/") {
		if ok, _ := path.Match(pattern, seg); ok {
			return true, nil
		}
	}
	return false, nil
}
