// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/MichaelSp/kswitch/types"
)

func newTestLogger() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	return logrus.NewEntry(l)
}

func TestGetHookState_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nonexistent.yaml")

	state, err := GetHookState(newTestLogger(), missing)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state for missing file, got: %#v", state)
	}
}

func TestGetHookState_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	state, err := GetHookState(newTestLogger(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatalf("expected non-nil empty state")
	}
	if state.HookName != "" {
		t.Errorf("expected empty HookName, got %q", state.HookName)
	}
	if !state.LastExecutionTime.IsZero() {
		t.Errorf("expected zero LastExecutionTime, got %v", state.LastExecutionTime)
	}
}

func TestGetHookState_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	original := &types.HookState{
		HookName:          "my-hook",
		LastExecutionTime: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
	}
	bytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	state, err := GetHookState(newTestLogger(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatalf("expected non-nil state")
	}
	if state.HookName != original.HookName {
		t.Errorf("expected HookName %q, got %q", original.HookName, state.HookName)
	}
	if !state.LastExecutionTime.Equal(original.LastExecutionTime) {
		t.Errorf("expected LastExecutionTime %v, got %v", original.LastExecutionTime, state.LastExecutionTime)
	}
}

func TestUpdateHookState_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	hookName := "test-hook"

	before := time.Now().UTC()
	if err := UpdateHookState(hookName, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now().UTC()

	state, err := GetHookState(newTestLogger(), path)
	if err != nil {
		t.Fatalf("unexpected error reading state: %v", err)
	}
	if state == nil {
		t.Fatalf("expected non-nil state")
	}
	if state.HookName != hookName {
		t.Errorf("expected HookName %q, got %q", hookName, state.HookName)
	}
	if state.LastExecutionTime.Before(before.Add(-time.Second)) || state.LastExecutionTime.After(after.Add(time.Second)) {
		t.Errorf("LastExecutionTime %v not within [%v, %v]", state.LastExecutionTime, before, after)
	}
}

func TestUpdateHookState_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	if err := os.WriteFile(path, []byte("existing junk content that should be replaced"), 0o644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	if err := UpdateHookState("new-hook", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, err := GetHookState(newTestLogger(), path)
	if err != nil {
		t.Fatalf("unexpected error reading state: %v", err)
	}
	if state == nil || state.HookName != "new-hook" {
		t.Errorf("expected hookName 'new-hook', got %#v", state)
	}
}

func TestGetHookState_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: : valid: yaml: ::"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := GetHookState(newTestLogger(), path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
