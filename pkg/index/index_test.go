// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MichaelSp/kswitch/types"
	"github.com/sirupsen/logrus"
)

const testStoreID = "store1"

func newLogger() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	return l.WithField("test", "test")
}

func TestNew_CreatesStateDir(t *testing.T) {
	parent := t.TempDir()
	stateDir := filepath.Join(parent, "newdir")

	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx == nil {
		t.Fatal("expected SearchIndex, got nil")
	}

	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("expected stateDir to be created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file")
	}
}

func TestNew_NoIndexFile(t *testing.T) {
	stateDir := t.TempDir()

	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx.HasContent() {
		t.Errorf("expected no content for fresh index")
	}
	if idx.HasKind(types.StoreKindFilesystem) {
		t.Errorf("expected HasKind=false on empty index")
	}
	pathMap, tagsMap := idx.GetContent()
	if pathMap != nil || tagsMap != nil {
		t.Errorf("expected nil content, got %v, %v", pathMap, tagsMap)
	}
}

func TestWriteAndLoad_RoundTrip(t *testing.T) {
	stateDir := t.TempDir()

	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	toWrite := types.Index{
		Kind: types.StoreKindFilesystem,
		ContextToPathMapping: map[string]string{
			"ctx1": "/path/one",
			"ctx2": "/path/two",
		},
		ContextToTags: map[string]map[string]string{
			"ctx1": {"env": "dev"},
		},
	}
	if err := idx.Write(toWrite); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Re-load by creating a new SearchIndex pointing at the same store
	idx2, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error on reload: %v", err)
	}
	if !idx2.HasContent() {
		t.Fatal("expected content after write")
	}
	if !idx2.HasKind(types.StoreKindFilesystem) {
		t.Errorf("expected HasKind=true")
	}
	if idx2.HasKind(types.StoreKindVault) {
		t.Errorf("HasKind for unrelated kind should be false")
	}
	pathMap, tagsMap := idx2.GetContent()
	if pathMap["ctx1"] != "/path/one" || pathMap["ctx2"] != "/path/two" {
		t.Errorf("unexpected path mapping: %v", pathMap)
	}
	if tagsMap["ctx1"]["env"] != "dev" {
		t.Errorf("unexpected tags: %v", tagsMap)
	}
}

func TestShouldBeUsed_NoState(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	used, err := idx.ShouldBeUsed(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used {
		t.Errorf("expected ShouldBeUsed=false when no state")
	}
}

func TestShouldBeUsed_FreshStateWithinWindow(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := idx.WriteState(types.IndexState{
		Kind:           types.StoreKindFilesystem,
		LastUpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	refresh := time.Hour
	used, err := idx.ShouldBeUsed(nil, &refresh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !used {
		t.Errorf("expected ShouldBeUsed=true for fresh state within window")
	}
}

func TestShouldBeUsed_ExpiredState(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := idx.WriteState(types.IndexState{
		Kind:           types.StoreKindFilesystem,
		LastUpdateTime: time.Now().UTC().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	refresh := time.Hour
	used, err := idx.ShouldBeUsed(nil, &refresh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used {
		t.Errorf("expected ShouldBeUsed=false for expired state")
	}
}

func TestShouldBeUsed_StateButNoRefreshConfigured(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := idx.WriteState(types.IndexState{
		Kind:           types.StoreKindFilesystem,
		LastUpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	used, err := idx.ShouldBeUsed(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used {
		t.Errorf("expected ShouldBeUsed=false when no refresh duration is configured")
	}
}

func TestShouldBeUsed_KindMismatch(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// State kind differs from index kind
	if err := idx.WriteState(types.IndexState{
		Kind:           types.StoreKindVault,
		LastUpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	refresh := time.Hour
	used, err := idx.ShouldBeUsed(nil, &refresh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used {
		t.Errorf("expected ShouldBeUsed=false when state kind mismatches")
	}
}

func TestShouldBeUsed_ConfigRefreshUsedWhenStoreLocalNil(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := idx.WriteState(types.IndexState{
		Kind:           types.StoreKindFilesystem,
		LastUpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	refresh := time.Hour
	cfg := &types.Config{RefreshIndexAfter: &refresh}
	used, err := idx.ShouldBeUsed(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !used {
		t.Errorf("expected ShouldBeUsed=true when config refresh duration is fresh")
	}
}

func TestDelete_RemovesBothFiles(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := idx.Write(types.Index{
		Kind:                 types.StoreKindFilesystem,
		ContextToPathMapping: map[string]string{"a": "/p"},
	}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := idx.WriteState(types.IndexState{
		Kind:           types.StoreKindFilesystem,
		LastUpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	if err := idx.Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := os.Stat(idx.indexFilepath); !os.IsNotExist(err) {
		t.Errorf("expected index file removed, stat err=%v", err)
	}
	if _, err := os.Stat(idx.indexStateFilepath); !os.IsNotExist(err) {
		t.Errorf("expected state file removed, stat err=%v", err)
	}
}

func TestNew_EmptyIndexFile(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Truncate to empty file
	f, err := os.Create(idx.indexFilepath)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	_ = f.Close()

	idx2, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !idx2.HasContent() {
		t.Errorf("expected content (empty Index struct) when file exists but is empty")
	}
}

func TestNew_CorruptIndexFile(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.WriteFile(idx.indexFilepath, []byte("::: not yaml :::"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID); err == nil {
		t.Fatal("expected error from corrupt index file")
	}
}

func TestShouldBeUsed_CorruptStateFile(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.WriteFile(idx.indexStateFilepath, []byte("::: not yaml :::"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := idx.ShouldBeUsed(nil, nil); err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}

func TestDelete_NoFiles_NoError(t *testing.T) {
	stateDir := t.TempDir()
	idx, err := New(newLogger(), types.StoreKindFilesystem, stateDir, testStoreID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := idx.Delete(); err != nil {
		t.Fatalf("Delete should not error on missing state file: %v", err)
	}
}
