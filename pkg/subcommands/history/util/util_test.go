package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHistoryEntry_ContextOnly(t *testing.T) {
	ctx, ns, err := ParseHistoryEntry("mycontext")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil || *ctx != "mycontext" {
		t.Errorf("expected context=mycontext, got %v", ctx)
	}
	if ns != nil {
		t.Errorf("expected namespace=nil, got %v", *ns)
	}
}

func TestParseHistoryEntry_ContextAndNamespace(t *testing.T) {
	ctx, ns, err := ParseHistoryEntry("mycontext:: mynamespace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == nil || *ctx != "mycontext" {
		t.Errorf("expected context=mycontext, got %v", ctx)
	}
	if ns == nil || *ns != "mynamespace" {
		t.Errorf("expected namespace=mynamespace, got %v", ns)
	}
}

func TestParseHistoryEntry_UnrecognizedFormat(t *testing.T) {
	ctx, ns, err := ParseHistoryEntry("a::b::c")
	if err == nil {
		t.Fatal("expected error for unrecognized format")
	}
	if ctx != nil || ns != nil {
		t.Errorf("expected nil ctx and ns, got %v, %v", ctx, ns)
	}
}

func TestAppendAndReadHistory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, ".kube"), 0755); err != nil {
		t.Fatalf("failed to create .kube dir: %v", err)
	}

	if err := AppendToHistory("ctx1", "ns1"); err != nil {
		t.Fatalf("AppendToHistory error: %v", err)
	}
	if err := AppendToHistory("ctx2", "ns2"); err != nil {
		t.Fatalf("AppendToHistory error: %v", err)
	}

	lines, err := ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	// reverse order: most recent first
	if lines[0] != "ctx2:: ns2" {
		t.Errorf("expected first line 'ctx2:: ns2', got %q", lines[0])
	}
	if lines[1] != "ctx1:: ns1" {
		t.Errorf("expected second line 'ctx1:: ns1', got %q", lines[1])
	}
}

func TestAppendToHistory_DeduplicatesIdenticalConsecutive(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, ".kube"), 0755); err != nil {
		t.Fatalf("failed to create .kube dir: %v", err)
	}

	if err := AppendToHistory("ctx1", "ns1"); err != nil {
		t.Fatalf("first AppendToHistory error: %v", err)
	}
	if err := AppendToHistory("ctx1", "ns1"); err != nil {
		t.Fatalf("second AppendToHistory error: %v", err)
	}

	lines, err := ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory error: %v", err)
	}
	if len(lines) != 1 {
		t.Errorf("expected 1 line (dedup), got %d: %v", len(lines), lines)
	}
}

func TestReadHistory_NoFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, ".kube"), 0755); err != nil {
		t.Fatalf("failed to create .kube dir: %v", err)
	}

	_, err := ReadHistory()
	if err == nil {
		t.Fatal("expected error when history file does not exist")
	}
}
