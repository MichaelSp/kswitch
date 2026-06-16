package state

import (
	"os"
	"path/filepath"
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
	expectedPath := filepath.Join(dir, "switch.alias")
	if a.aliasFilepath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, a.aliasFilepath)
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

	if _, err := os.Stat(filepath.Join(dir, "switch.alias")); err != nil {
		t.Fatalf("expected file to exist: %v", err)
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
