// Copyright 2026 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestGetDefaultAlias_NoFile(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil Alias")
	}
	if len(a.Content.ContextToAliasMapping) != 0 {
		t.Fatalf("expected empty mapping, got %v", a.Content.ContextToAliasMapping)
	}
}

func TestGetDefaultAlias_LoadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := a.WriteAlias("myalias", "mycontext"); err != nil {
		t.Fatalf("WriteAlias failed: %v", err)
	}

	a2, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error reloading: %v", err)
	}
	if got := a2.Content.ContextToAliasMapping["mycontext"]; got != "myalias" {
		t.Errorf("expected mycontext->myalias, got %q", got)
	}
}

func TestWriteAlias_NewAlias(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prev, err := a.WriteAlias("myalias", "mycontext")
	if err != nil {
		t.Fatalf("WriteAlias error: %v", err)
	}
	if prev != nil {
		t.Errorf("expected nil prev context, got %v", *prev)
	}
	if a.Content.ContextToAliasMapping["mycontext"] != "myalias" {
		t.Errorf("mapping not stored properly")
	}
	if _, err := os.Stat(filepath.Join(dir, aliasDirName, "myalias")); err != nil {
		t.Errorf("expected alias file to exist: %v", err)
	}
}

func TestWriteAlias_CreatesOwnerOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not enforced on windows")
	}
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := a.WriteAlias("myalias", "mycontext"); err != nil {
		t.Fatalf("WriteAlias error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, aliasDirName))
	if err != nil {
		t.Fatalf("stat alias dir: %v", err)
	}
	// an alias file per context names the clusters the user works with
	if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("alias dir has mode %04o, want 0700", got)
	}
}

func TestWriteAlias_ReplacesExistingAlias(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := a.WriteAlias("shared", "ctx-old"); err != nil {
		t.Fatalf("first WriteAlias error: %v", err)
	}

	prev, err := a.WriteAlias("shared", "ctx-new")
	if err != nil {
		t.Fatalf("second WriteAlias error: %v", err)
	}
	if prev == nil {
		t.Fatal("expected previous context name, got nil")
	}
	if *prev != "ctx-old" {
		t.Errorf("expected prev=ctx-old, got %q", *prev)
	}
	if _, exists := a.Content.ContextToAliasMapping["ctx-old"]; exists {
		t.Errorf("expected ctx-old to be removed from mapping")
	}
	if a.Content.ContextToAliasMapping["ctx-new"] != "shared" {
		t.Errorf("expected ctx-new->shared mapping")
	}
}

func TestContainsAlias(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := a.WriteAlias("myalias", "mycontext"); err != nil {
		t.Fatalf("WriteAlias failed: %v", err)
	}

	got := a.ContainsAlias("myalias")
	if got == nil {
		t.Fatal("expected to find context for myalias")
	}
	if *got != "mycontext" {
		t.Errorf("expected mycontext, got %q", *got)
	}

	if a.ContainsAlias("does-not-exist") != nil {
		t.Error("expected nil for missing alias")
	}
}

func TestWriteAllAliases_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := a.WriteAlias("a1", "ctx1"); err != nil {
		t.Fatalf("WriteAlias error: %v", err)
	}
	if _, err := a.WriteAlias("a2", "ctx2"); err != nil {
		t.Fatalf("WriteAlias error: %v", err)
	}
	if err := a.WriteAllAliases(); err != nil {
		t.Fatalf("WriteAllAliases error: %v", err)
	}

	a2, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if a2.Content.ContextToAliasMapping["ctx1"] != "a1" {
		t.Errorf("expected ctx1->a1, got %q", a2.Content.ContextToAliasMapping["ctx1"])
	}
	if a2.Content.ContextToAliasMapping["ctx2"] != "a2" {
		t.Errorf("expected ctx2->a2, got %q", a2.Content.ContextToAliasMapping["ctx2"])
	}
}

func TestWriteAllAliases_RemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := a.WriteAlias("a1", "ctx1"); err != nil {
		t.Fatalf("WriteAlias error: %v", err)
	}
	if _, err := a.WriteAlias("a2", "ctx2"); err != nil {
		t.Fatalf("WriteAlias error: %v", err)
	}

	// Remove a2 from memory and rewrite
	delete(a.Content.ContextToAliasMapping, "ctx2")
	if err := a.WriteAllAliases(); err != nil {
		t.Fatalf("WriteAllAliases error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, aliasDirName, "a2")); !os.IsNotExist(err) {
		t.Error("expected stale alias file a2 to be removed")
	}
}

func TestMigrateFromLegacy(t *testing.T) {
	dir := t.TempDir()

	// Write a legacy switch.alias file
	legacy := "contextToAliasMapping:\n    mycontext: myalias\n    other: otheralias\n"
	if err := os.WriteFile(filepath.Join(dir, "switch.alias"), []byte(legacy), 0644); err != nil {
		t.Fatalf("failed to write legacy file: %v", err)
	}

	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if a.Content.ContextToAliasMapping["mycontext"] != "myalias" {
		t.Errorf("expected mycontext->myalias after migration")
	}
	if a.Content.ContextToAliasMapping["other"] != "otheralias" {
		t.Errorf("expected other->otheralias after migration")
	}

	// Legacy file should be gone
	if _, err := os.Stat(filepath.Join(dir, "switch.alias")); !os.IsNotExist(err) {
		t.Error("expected legacy file to be removed after migration")
	}

	// Per-alias files should exist
	if _, err := os.Stat(filepath.Join(dir, aliasDirName, "myalias")); err != nil {
		t.Errorf("expected alias file myalias to exist: %v", err)
	}
}

func TestConcurrentWrites_NoLostUpdates(t *testing.T) {
	dir := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			a, err := GetDefaultAlias(dir)
			if err != nil {
				t.Errorf("goroutine %d: GetDefaultAlias error: %v", i, err)
				return
			}
			alias := fmt.Sprintf("alias-%d", i)
			ctx := fmt.Sprintf("ctx-%d", i)
			if _, err := a.WriteAlias(alias, ctx); err != nil {
				t.Errorf("goroutine %d: WriteAlias error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	a, err := GetDefaultAlias(dir)
	if err != nil {
		t.Fatalf("final load error: %v", err)
	}
	if len(a.Content.ContextToAliasMapping) != n {
		t.Errorf("expected %d aliases after concurrent writes, got %d", n, len(a.Content.ContextToAliasMapping))
	}
}
