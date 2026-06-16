// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"os"
	"strings"
	"testing"
)

const validKubeconfig = `apiVersion: v1
kind: Config
contexts:
- name: ctx1
  context:
    cluster: cluster1
    user: user1
- name: ctx2
  context:
    cluster: cluster2
    user: user2
current-context: ctx1
`

const emptyContextsKubeconfig = `apiVersion: v1
kind: Config
contexts: []
current-context: ""
`

func TestParseSanitizedKubeconfig(t *testing.T) {
	t.Run("valid kubeconfig", func(t *testing.T) {
		cfg, err := ParseSanitizedKubeconfig([]byte(validKubeconfig))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected config, got nil")
		}
		if cfg.CurrentContext != "ctx1" {
			t.Errorf("expected current-context ctx1, got %q", cfg.CurrentContext)
		}
		if len(cfg.Contexts) != 2 {
			t.Fatalf("expected 2 contexts, got %d", len(cfg.Contexts))
		}
		if cfg.Contexts[0].Name != "ctx1" || cfg.Contexts[1].Name != "ctx2" {
			t.Errorf("unexpected context names: %v", cfg.Contexts)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := ParseSanitizedKubeconfig([]byte("not: : valid: yaml: ::"))
		if err == nil {
			t.Fatal("expected error for invalid yaml")
		}
	})

	t.Run("empty bytes", func(t *testing.T) {
		cfg, err := ParseSanitizedKubeconfig([]byte(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if len(cfg.Contexts) != 0 {
			t.Errorf("expected 0 contexts for empty kubeconfig")
		}
	})
}

func TestGetContextsNamesFromKubeconfig(t *testing.T) {
	t.Run("with prefix - returns only current-context when set", func(t *testing.T) {
		names, err := GetContextsNamesFromKubeconfig([]byte(validKubeconfig), "myprefix")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// validKubeconfig has current-context: ctx1, so only that context is returned
		expected := []string{"myprefix/ctx1"}
		if len(names) != len(expected) {
			t.Fatalf("expected %d names, got %d (%v)", len(expected), len(names), names)
		}
		if names[0] != expected[0] {
			t.Errorf("name[0] = %q, want %q", names[0], expected[0])
		}
	})

	t.Run("without prefix - returns only current-context when set", func(t *testing.T) {
		names, err := GetContextsNamesFromKubeconfig([]byte(validKubeconfig), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// validKubeconfig has current-context: ctx1, so only that context is returned
		expected := []string{"ctx1"}
		if len(names) != len(expected) {
			t.Fatalf("expected %d names, got %d (%v)", len(expected), len(names), names)
		}
		if names[0] != expected[0] {
			t.Errorf("name[0] = %q, want %q", names[0], expected[0])
		}
	})

	t.Run("no current-context - returns all contexts", func(t *testing.T) {
		noCurrentCtxKubeconfig := `apiVersion: v1
kind: Config
contexts:
- name: ctx1
  context:
    cluster: cluster1
    user: user1
- name: ctx2
  context:
    cluster: cluster2
    user: user2
current-context: ""
`
		names, err := GetContextsNamesFromKubeconfig([]byte(noCurrentCtxKubeconfig), "p")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 2 {
			t.Fatalf("expected 2 names when no current-context, got %d (%v)", len(names), names)
		}
	})

	t.Run("empty contexts", func(t *testing.T) {
		names, err := GetContextsNamesFromKubeconfig([]byte(emptyContextsKubeconfig), "p")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("expected 0 names for empty contexts, got %v", names)
		}
	})

	t.Run("invalid kubeconfig", func(t *testing.T) {
		_, err := GetContextsNamesFromKubeconfig([]byte("not: : valid: ::"), "")
		if err == nil {
			t.Fatal("expected error from invalid kubeconfig")
		}
		if !strings.Contains(err.Error(), "could not parse Kubeconfig") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestExpandEnv(t *testing.T) {
	t.Run("expands tilde to HOME", func(t *testing.T) {
		t.Setenv("HOME", "/home/testuser")
		got := ExpandEnv("~/foo/bar")
		want := "/home/testuser/foo/bar"
		if got != want {
			t.Errorf("ExpandEnv(~/foo/bar) = %q, want %q", got, want)
		}
	})

	t.Run("expands env var", func(t *testing.T) {
		t.Setenv("MY_TEST_VAR", "/some/path")
		got := ExpandEnv("$MY_TEST_VAR/sub")
		want := "/some/path/sub"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("no env vars no tilde", func(t *testing.T) {
		got := ExpandEnv("/no/expansion/here")
		want := "/no/expansion/here"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("undefined env var becomes empty", func(t *testing.T) {
		got := ExpandEnv("$THIS_VAR_DOES_NOT_EXIST_ABC123/x")
		want := "/x"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestSliceFindIndex(t *testing.T) {
	t.Run("string found", func(t *testing.T) {
		got := SliceFindIndex([]string{"a", "b", "c"}, "b")
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("string not found", func(t *testing.T) {
		got := SliceFindIndex([]string{"a", "b", "c"}, "z")
		if got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})

	t.Run("int found at start", func(t *testing.T) {
		got := SliceFindIndex([]int{10, 20, 30}, 10)
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("int not found", func(t *testing.T) {
		got := SliceFindIndex([]int{1, 2, 3}, 99)
		if got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got := SliceFindIndex([]string{}, "x")
		if got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})
}

func TestSplitAdditionalArgs(t *testing.T) {
	t.Run("no double dash - returns empty, args unchanged", func(t *testing.T) {
		orig := os.Args
		defer func() { os.Args = orig }()
		os.Args = []string{"cmd", "arg1", "arg2"}

		args := []string{"arg1", "arg2"}
		extra := SplitAdditionalArgs(&args)
		if len(extra) != 0 {
			t.Errorf("expected no extra args, got %v", extra)
		}
		if len(args) != 2 {
			t.Errorf("expected args unchanged (len 2), got %v", args)
		}
	})

	t.Run("double dash present - splits args and returns extra", func(t *testing.T) {
		orig := os.Args
		defer func() { os.Args = orig }()
		os.Args = []string{"cmd", "a", "--", "extra1", "extra2"}

		args := []string{"a", "extra1", "extra2"}
		extra := SplitAdditionalArgs(&args)
		if len(extra) != 2 || extra[0] != "extra1" || extra[1] != "extra2" {
			t.Errorf("expected [extra1 extra2], got %v", extra)
		}
		if len(args) != 1 || args[0] != "a" {
			t.Errorf("expected args=[a], got %v", args)
		}
	})
}
