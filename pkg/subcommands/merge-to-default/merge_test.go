// Copyright 2025 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package merge_to_default

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const dstKubeconfig = `apiVersion: v1
kind: Config
current-context: old-ctx
clusters:
- cluster:
    server: https://old-server
  name: old-cluster
contexts:
- context:
    cluster: old-cluster
    user: old-user
  name: old-ctx
users:
- name: old-user
  user:
    token: old-token
`

const srcKubeconfig = `apiVersion: v1
kind: Config
current-context: new-ctx
clusters:
- cluster:
    server: https://new-server
  name: new-cluster
- cluster:
    server: https://updated-server
  name: old-cluster
contexts:
- context:
    cluster: new-cluster
    user: new-user
  name: new-ctx
users:
- name: new-user
  user:
    token: new-token
`

func TestMergeKubeconfigs_AddsNewEntries(t *testing.T) {
	result, err := mergeKubeconfigs([]byte(dstKubeconfig), []byte(srcKubeconfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(result, &doc); err != nil || len(doc.Content) == 0 {
		t.Fatal("result is not valid YAML")
	}
	root := doc.Content[0]

	// current-context should be updated to src's
	if cc := scalarValue(root, "current-context"); cc != "new-ctx" {
		t.Errorf("current-context = %q, want %q", cc, "new-ctx")
	}

	// new-ctx context should exist
	if !containsNamedEntry(root, "contexts", "new-ctx") {
		t.Error("expected new-ctx in contexts")
	}
	// old-ctx should still exist
	if !containsNamedEntry(root, "contexts", "old-ctx") {
		t.Error("expected old-ctx preserved in contexts")
	}
	// new-cluster should exist
	if !containsNamedEntry(root, "clusters", "new-cluster") {
		t.Error("expected new-cluster in clusters")
	}
}

func TestMergeKubeconfigs_OverwritesExistingCluster(t *testing.T) {
	result, err := mergeKubeconfigs([]byte(dstKubeconfig), []byte(srcKubeconfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// old-cluster should have updated server URL
	if !strings.Contains(string(result), "https://updated-server") {
		t.Error("expected old-cluster server to be updated to https://updated-server")
	}
	if strings.Contains(string(result), "https://old-server") {
		t.Error("expected old-cluster old server URL to be replaced")
	}
}

func TestMergeKubeconfigs_EmptyDst(t *testing.T) {
	empty := []byte("apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n")
	result, err := mergeKubeconfigs(empty, []byte(srcKubeconfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(result), "new-ctx") {
		t.Error("expected new-ctx in merged result")
	}
}

func containsNamedEntry(root *yaml.Node, section, name string) bool {
	seq := nodeValue(root, section)
	if seq == nil {
		return false
	}
	for _, entry := range seq.Content {
		if n := nodeValue(entry, "name"); n != nil && n.Value == name {
			return true
		}
	}
	return false
}
