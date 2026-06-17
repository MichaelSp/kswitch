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
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKindBinary writes a shell script that emulates a subset of `kind`
// behaviour and returns the path to the script.
func fakeKindBinary(t *testing.T, clusters []string, kubeconfigContents map[string]string) string {
	t.Helper()

	// Build the cluster list output
	clusterList := ""
	for _, c := range clusters {
		clusterList += c + "\n"
	}

	// Build per-cluster kubeconfig case branches
	caseBranches := ""
	for name, kc := range kubeconfigContents {
		caseBranches += "    " + name + ")\n      echo '" + kc + "'\n      ;;\n"
	}

	script := `#!/bin/sh
case "$1" in
  get)
    case "$2" in
      clusters)
        printf '%s' "` + clusterList + `"
        ;;
      kubeconfig)
        # $3 = --name, $4 = cluster name
        case "$4" in
` + caseBranches + `          *)
            echo "unknown cluster $4" >&2
            exit 1
            ;;
        esac
        ;;
    esac
    ;;
  *)
    echo "unknown command $1" >&2
    exit 1
    ;;
esac
`

	dir := t.TempDir()
	bin := filepath.Join(dir, "kind")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin
}

func newKindStoreWithBinary(t *testing.T, binary string) *KindStore {
	t.Helper()
	store := types.KubeconfigStore{
		Kind: types.StoreKindKind,
		Config: types.StoreConfigKind{
			KindBinary: binary,
		},
	}
	s, err := NewKindStore(store)
	require.NoError(t, err)
	return s
}

func TestNewKindStore_DefaultBinary(t *testing.T) {
	store := types.KubeconfigStore{Kind: types.StoreKindKind}
	s, err := NewKindStore(store)
	require.NoError(t, err)
	assert.Equal(t, "kind", s.Config.KindBinary)
}

func TestNewKindStore_CustomBinary(t *testing.T) {
	store := types.KubeconfigStore{
		Kind: types.StoreKindKind,
		Config: types.StoreConfigKind{
			KindBinary: "/usr/local/bin/kind",
		},
	}
	s, err := NewKindStore(store)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/kind", s.Config.KindBinary)
}

func TestKindStore_GetContextPrefix(t *testing.T) {
	bin := fakeKindBinary(t, nil, nil)
	s := newKindStoreWithBinary(t, bin)

	assert.Equal(t, "kind", s.GetContextPrefix("anything"))
}

func TestKindStore_GetContextPrefix_ShowPrefixFalse(t *testing.T) {
	showPrefix := false
	store := types.KubeconfigStore{
		Kind:       types.StoreKindKind,
		ShowPrefix: &showPrefix,
		Config:     types.StoreConfigKind{KindBinary: fakeKindBinary(t, nil, nil)},
	}
	s, err := NewKindStore(store)
	require.NoError(t, err)
	assert.Equal(t, "", s.GetContextPrefix("anything"))
}

func TestKindStore_StartSearch_MultipleClusters(t *testing.T) {
	clusters := []string{"cluster-a", "cluster-b", "cluster-c"}
	bin := fakeKindBinary(t, clusters, nil)
	s := newKindStoreWithBinary(t, bin)

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var results []storetypes.SearchResult
	for r := range ch {
		results = append(results, r)
	}

	require.Len(t, results, 3)
	assert.Equal(t, "cluster-a", results[0].KubeconfigPath)
	assert.Equal(t, "cluster-b", results[1].KubeconfigPath)
	assert.Equal(t, "cluster-c", results[2].KubeconfigPath)
	for _, r := range results {
		assert.NoError(t, r.Error)
		assert.Equal(t, r.KubeconfigPath, r.Tags["name"])
	}
}

func TestKindStore_StartSearch_NoClusters(t *testing.T) {
	bin := fakeKindBinary(t, []string{}, nil)
	s := newKindStoreWithBinary(t, bin)

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var results []storetypes.SearchResult
	for r := range ch {
		results = append(results, r)
	}
	assert.Empty(t, results)
}

func TestKindStore_StartSearch_BinaryNotFound(t *testing.T) {
	store := types.KubeconfigStore{
		Kind:   types.StoreKindKind,
		Config: types.StoreConfigKind{KindBinary: "/nonexistent/kind"},
	}
	s, err := NewKindStore(store)
	require.NoError(t, err)

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var results []storetypes.SearchResult
	for r := range ch {
		results = append(results, r)
	}
	require.Len(t, results, 1)
	assert.Error(t, results[0].Error)
}

func TestKindStore_GetKubeconfigForPath(t *testing.T) {
	const wantKubeconfig = "apiVersion: v1\nkind: Config"
	bin := fakeKindBinary(t, []string{"my-cluster"}, map[string]string{
		"my-cluster": wantKubeconfig,
	})
	s := newKindStoreWithBinary(t, bin)

	got, err := s.GetKubeconfigForPath("my-cluster", map[string]string{"name": "my-cluster"})
	require.NoError(t, err)
	assert.Contains(t, string(got), "apiVersion: v1")
}

func TestKindStore_GetKubeconfigForPath_UsesTagName(t *testing.T) {
	const wantKubeconfig = "apiVersion: v1\nkind: Config"
	bin := fakeKindBinary(t, []string{"tag-cluster"}, map[string]string{
		"tag-cluster": wantKubeconfig,
	})
	s := newKindStoreWithBinary(t, bin)

	// path differs from tag name — tag should win
	got, err := s.GetKubeconfigForPath("some-path", map[string]string{"name": "tag-cluster"})
	require.NoError(t, err)
	assert.Contains(t, string(got), "apiVersion: v1")
}

func TestKindStore_GetKubeconfigForPath_UnknownCluster(t *testing.T) {
	bin := fakeKindBinary(t, []string{"existing"}, map[string]string{
		"existing": "apiVersion: v1",
	})
	s := newKindStoreWithBinary(t, bin)

	_, err := s.GetKubeconfigForPath("missing", map[string]string{"name": "missing"})
	assert.Error(t, err)
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"a\nb\nc\n", []string{"a", "b", "c"}},
		{"  a  \n  b  ", []string{"a", "b"}},
		{"", nil},
		{"\n\n", nil},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, splitLines(tc.input))
	}
}

// TestKindStore_Integration is skipped unless `kind` is in PATH.
func TestKindStore_Integration(t *testing.T) {
	if _, err := exec.LookPath("kind"); err != nil {
		t.Skip("kind binary not in PATH")
	}

	store := types.KubeconfigStore{Kind: types.StoreKindKind}
	s, err := NewKindStore(store)
	require.NoError(t, err)

	ch := make(chan storetypes.SearchResult, 100)
	s.StartSearch(ch)
	close(ch)

	// just verify no panic and no hard error
	for r := range ch {
		if r.Error != nil {
			t.Logf("search error (may be benign): %v", r.Error)
		}
	}
}
