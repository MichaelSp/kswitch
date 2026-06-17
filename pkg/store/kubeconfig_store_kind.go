// Copyright 2024 The Kswitch authors
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

package store

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindKind, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewKindStore(s)
	})
}

var _ storetypes.KubeconfigStore = (*KindStore)(nil)

func NewKindStore(store types.KubeconfigStore) (*KindStore, error) {
	config, err := ParseStoreConfig[types.StoreConfigKind](store)
	if err != nil {
		return nil, err
	}
	if config.KindBinary == "" {
		config.KindBinary = "kind"
	}
	return &KindStore{
		BaseStore: NewBaseStore(types.StoreKindKind, store),
		Config:    config,
	}, nil
}

// GetContextPrefix returns the context prefix for kind clusters.
func (s *KindStore) GetContextPrefix(_ string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}
	return string(types.StoreKindKind)
}

// StartSearch lists all local kind clusters and emits one SearchResult per cluster.
func (s *KindStore) StartSearch(channel chan storetypes.SearchResult) {
	s.Logger.Debug("kind: start search")

	out, err := s.runKind("get", "clusters")
	if err != nil {
		// kind exits non-zero with "No kind clusters found" – treat as empty, not error
		if strings.Contains(out, "No kind clusters found") {
			s.Logger.Debug("kind: no clusters found")
			return
		}
		channel <- storetypes.SearchResult{Error: fmt.Errorf("kind get clusters: %w", err)}
		return
	}

	for _, name := range splitLines(out) {
		s.Logger.Debug("kind: found cluster", "name", name)
		channel <- storetypes.SearchResult{
			KubeconfigPath: name,
			Tags:           map[string]string{"name": name},
		}
	}
}

// GetKubeconfigForPath returns the kubeconfig for the named kind cluster.
func (s *KindStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	name := path
	if n, ok := tags["name"]; ok && n != "" {
		name = n
	}

	s.Logger.Debug("kind: get kubeconfig", "cluster", name)

	out, err := s.runKind("get", "kubeconfig", "--name", name)
	if err != nil {
		return nil, fmt.Errorf("kind get kubeconfig --name %s: %w", name, err)
	}
	return []byte(out), nil
}

func (s *KindStore) runKind(args ...string) (string, error) {
	cmd := exec.Command(s.Config.KindBinary, args...) //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(stdout.String() + stderr.String())
		return combined, fmt.Errorf("%w: %s", err, combined)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func splitLines(s string) []string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			lines = append(lines, t)
		}
	}
	return lines
}
