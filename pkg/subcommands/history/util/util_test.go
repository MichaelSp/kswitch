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

package util

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestAppendToHistory_CreatesOwnerOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not enforced on windows")
	}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, ".kube"), 0700); err != nil {
		t.Fatalf("failed to create .kube dir: %v", err)
	}

	// the history names every cluster and namespace the user works with, which other
	// users on the host have no business enumerating
	if err := AppendToHistory("ctx1", "ns1"); err != nil {
		t.Fatalf("AppendToHistory error: %v", err)
	}

	info, err := os.Stat(filepath.Join(homeDir, ".kube", ".switch_history"))
	if err != nil {
		t.Fatalf("stat history file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("history file has mode %04o, want 0600", got)
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
