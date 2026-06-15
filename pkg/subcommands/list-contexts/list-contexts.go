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
	"sort"

	"github.com/sirupsen/logrus"

	"github.com/MichaelSp/kswitch/pkg"
	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

var logger = logrus.New()

func ListContexts(pattern string, stores []storetypes.KubeconfigStore, config *types.Config, stateDir string, noIndex bool) ([]string, error) {
	c, err := pkg.DoSearch(stores, config, stateDir, noIndex)
	if err != nil {
		return nil, fmt.Errorf("cannot list contexts: %v", err)
	}

	m := pattern
	var contexts []string
	for discoveredKubeconfig := range *c {
		if discoveredKubeconfig.Error != nil {
			logger.Warnf("cannot list contexts. Error returned from search: %v", discoveredKubeconfig.Error)
			continue
		}

		name := discoveredKubeconfig.Name
		if len(discoveredKubeconfig.Alias) > 0 {
			name = discoveredKubeconfig.Alias
		}
		// path.Match supports '*' and '?' wildcards over runes, matching the
		// pattern semantics kswitch previously got from becheran/wildmatch-go.
		// path.ErrBadPattern can only be triggered by '[' character classes,
		// which kubeconfig context names cannot contain — log and skip.
		matched, err := path.Match(m, name)
		if err != nil {
			logger.Warnf("invalid pattern %q: %v", m, err)
			continue
		}
		if matched {
			contexts = append(contexts, name)
		}
	}
	// Sort alphabetically
	sort.Strings(contexts)

	return contexts, nil
}
