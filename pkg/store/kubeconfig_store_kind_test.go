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
	"strings"
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

// fakeKindBinary writes a shell script that emulates a subset of `kind`
// behaviour and returns the path to the script.
func fakeKindBinary(t *testing.T, clusters []string, kubeconfigContents map[string]string) string {
	t.Helper()

	var clusterListBuilder strings.Builder
	for _, c := range clusters {
		clusterListBuilder.WriteString(c)
		clusterListBuilder.WriteByte('\n')
	}
	clusterList := clusterListBuilder.String()

	var caseBranchBuilder strings.Builder
	for name, kc := range kubeconfigContents {
		caseBranchBuilder.WriteString("    " + name + ")\n      echo '" + kc + "'\n      ;;\n")
	}
	caseBranches := caseBranchBuilder.String()

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
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kind binary: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewKindStore: %v", err)
	}
	return s
}

func TestNewKindStore_DefaultBinary(t *testing.T) {
	store := types.KubeconfigStore{Kind: types.StoreKindKind}
	s, err := NewKindStore(store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Config.KindBinary != "kind" {
		t.Errorf("KindBinary = %q, want %q", s.Config.KindBinary, "kind")
	}
}

func TestNewKindStore_CustomBinary(t *testing.T) {
	store := types.KubeconfigStore{
		Kind:   types.StoreKindKind,
		Config: types.StoreConfigKind{KindBinary: "/usr/local/bin/kind"},
	}
	s, err := NewKindStore(store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Config.KindBinary != "/usr/local/bin/kind" {
		t.Errorf("KindBinary = %q, want %q", s.Config.KindBinary, "/usr/local/bin/kind")
	}
}

func TestKindStore_GetContextPrefix(t *testing.T) {
	bin := fakeKindBinary(t, nil, nil)
	s := newKindStoreWithBinary(t, bin)
	if got := s.GetContextPrefix("anything"); got != "kind" {
		t.Errorf("GetContextPrefix() = %q, want %q", got, "kind")
	}
}

func TestKindStore_GetContextPrefix_ShowPrefixFalse(t *testing.T) {
	showPrefix := false
	store := types.KubeconfigStore{
		Kind:       types.StoreKindKind,
		ShowPrefix: &showPrefix,
		Config:     types.StoreConfigKind{KindBinary: fakeKindBinary(t, nil, nil)},
	}
	s, err := NewKindStore(store)
	if err != nil {
		t.Fatalf("NewKindStore: %v", err)
	}
	if got := s.GetContextPrefix("anything"); got != "" {
		t.Errorf("GetContextPrefix() = %q, want empty string", got)
	}
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

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, want := range clusters {
		if results[i].KubeconfigPath != want {
			t.Errorf("results[%d].KubeconfigPath = %q, want %q", i, results[i].KubeconfigPath, want)
		}
		if results[i].Error != nil {
			t.Errorf("results[%d].Error = %v, want nil", i, results[i].Error)
		}
		if results[i].Tags["name"] != want {
			t.Errorf("results[%d].Tags[name] = %q, want %q", i, results[i].Tags["name"], want)
		}
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
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %v", len(results), results)
	}
}

func TestKindStore_StartSearch_BinaryNotFound(t *testing.T) {
	store := types.KubeconfigStore{
		Kind:   types.StoreKindKind,
		Config: types.StoreConfigKind{KindBinary: "/nonexistent/kind"},
	}
	s, err := NewKindStore(store)
	if err != nil {
		t.Fatalf("NewKindStore: %v", err)
	}

	ch := make(chan storetypes.SearchResult, 10)
	s.StartSearch(ch)
	close(ch)

	var results []storetypes.SearchResult
	for r := range ch {
		results = append(results, r)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 error result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error, got nil")
	}
}

func TestKindStore_GetKubeconfigForPath(t *testing.T) {
	const wantKubeconfig = "apiVersion: v1\nkind: Config"
	bin := fakeKindBinary(t, []string{"my-cluster"}, map[string]string{
		"my-cluster": wantKubeconfig,
	})
	s := newKindStoreWithBinary(t, bin)

	got, err := s.GetKubeconfigForPath("my-cluster", map[string]string{"name": "my-cluster"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "apiVersion: v1") {
		t.Errorf("kubeconfig does not contain expected content, got: %s", got)
	}
}

func TestKindStore_GetKubeconfigForPath_UsesTagName(t *testing.T) {
	const wantKubeconfig = "apiVersion: v1\nkind: Config"
	bin := fakeKindBinary(t, []string{"tag-cluster"}, map[string]string{
		"tag-cluster": wantKubeconfig,
	})
	s := newKindStoreWithBinary(t, bin)

	// path differs from tag name — tag should win
	got, err := s.GetKubeconfigForPath("some-path", map[string]string{"name": "tag-cluster"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "apiVersion: v1") {
		t.Errorf("kubeconfig does not contain expected content, got: %s", got)
	}
}

func TestKindStore_GetKubeconfigForPath_UnknownCluster(t *testing.T) {
	bin := fakeKindBinary(t, []string{"existing"}, map[string]string{
		"existing": "apiVersion: v1",
	})
	s := newKindStoreWithBinary(t, bin)

	_, err := s.GetKubeconfigForPath("missing", map[string]string{"name": "missing"})
	if err == nil {
		t.Error("expected error for unknown cluster, got nil")
	}
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
		got := splitLines(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitLines(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// TestKindStore_Integration is skipped unless `kind` is in PATH.
func TestKindStore_Integration(t *testing.T) {
	if _, err := exec.LookPath("kind"); err != nil {
		t.Skip("kind binary not in PATH")
	}

	store := types.KubeconfigStore{Kind: types.StoreKindKind}
	s, err := NewKindStore(store)
	if err != nil {
		t.Fatalf("NewKindStore: %v", err)
	}

	ch := make(chan storetypes.SearchResult, 100)
	s.StartSearch(ch)
	close(ch)

	for r := range ch {
		if r.Error != nil {
			t.Logf("search error (may be benign): %v", r.Error)
		}
	}
}
