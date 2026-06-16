// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package memory

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

type mockStore struct {
	id            string
	kind          types.StoreKind
	contextPrefix string
	verifyErr     error
	verifyCalls   int
	getCalls      int
	getKubeconfig []byte
	getErr        error
	lastPath      string
	lastTags      map[string]string
}

func (m *mockStore) GetID() string                            { return m.id }
func (m *mockStore) GetKind() types.StoreKind                 { return m.kind }
func (m *mockStore) GetContextPrefix(string) string           { return m.contextPrefix }
func (m *mockStore) VerifyKubeconfigPaths() error             { m.verifyCalls++; return m.verifyErr }
func (m *mockStore) StartSearch(chan storetypes.SearchResult) {}
func (m *mockStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	m.getCalls++
	m.lastPath = path
	m.lastTags = tags
	return m.getKubeconfig, m.getErr
}
func (m *mockStore) GetLogger() *logrus.Entry              { return logrus.NewEntry(logrus.New()) }
func (m *mockStore) GetStoreConfig() types.KubeconfigStore { return types.KubeconfigStore{} }

type mockStorePreviewer struct {
	mockStore
	previewValue string
	previewErr   error
	previewCalls int
}

func (m *mockStorePreviewer) GetSearchPreview(path string, optionalTags map[string]string) (string, error) {
	m.previewCalls++
	return m.previewValue, m.previewErr
}

func newCache(t *testing.T, upstream storetypes.KubeconfigStore) storetypes.KubeconfigStore {
	t.Helper()
	c, err := New(upstream, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return c
}

func TestGetKubeconfigForPath_CachesResult(t *testing.T) {
	upstream := &mockStore{getKubeconfig: []byte("kube-bytes")}
	c := newCache(t, upstream)

	got, err := c.GetKubeconfigForPath("/some/path", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "kube-bytes" {
		t.Errorf("expected 'kube-bytes', got %q", got)
	}
	if upstream.getCalls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", upstream.getCalls)
	}

	got2, err := c.GetKubeconfigForPath("/some/path", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got2) != "kube-bytes" {
		t.Errorf("expected cached 'kube-bytes', got %q", got2)
	}
	if upstream.getCalls != 1 {
		t.Errorf("expected upstream still called only once after cache hit, got %d", upstream.getCalls)
	}
}

func TestGetKubeconfigForPath_UpstreamErrorNotCached(t *testing.T) {
	wantErr := errors.New("upstream failure")
	upstream := &mockStore{getErr: wantErr}
	c := newCache(t, upstream)

	if _, err := c.GetKubeconfigForPath("/p", nil); !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
	if _, err := c.GetKubeconfigForPath("/p", nil); !errors.Is(err, wantErr) {
		t.Fatalf("expected same error on retry, got %v", err)
	}
	if upstream.getCalls != 2 {
		t.Errorf("expected 2 upstream calls (errors not cached), got %d", upstream.getCalls)
	}
}

func TestGetSearchPreview_NotPreviewerReturnsEmpty(t *testing.T) {
	upstream := &mockStore{}
	c := newCache(t, upstream)

	mc, ok := c.(*memoryCache)
	if !ok {
		t.Fatalf("expected *memoryCache, got %T", c)
	}
	preview, err := mc.GetSearchPreview("/p", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview != "" {
		t.Errorf("expected empty preview, got %q", preview)
	}
}

func TestGetSearchPreview_PreviewerDelegated(t *testing.T) {
	upstream := &mockStorePreviewer{previewValue: "preview-data"}
	c := newCache(t, upstream)

	mc := c.(*memoryCache)
	preview, err := mc.GetSearchPreview("/p", map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview != "preview-data" {
		t.Errorf("expected 'preview-data', got %q", preview)
	}
	if upstream.previewCalls != 1 {
		t.Errorf("expected 1 preview call, got %d", upstream.previewCalls)
	}
}

func TestPassthroughMethods(t *testing.T) {
	upstream := &mockStore{
		id:            "upstream-id",
		kind:          types.StoreKind("kfilesystem"),
		contextPrefix: "ctx-prefix",
	}
	c := newCache(t, upstream)

	if got := c.GetID(); got != "upstream-id" {
		t.Errorf("GetID: expected 'upstream-id', got %q", got)
	}
	if got := c.GetKind(); got != types.StoreKind("kfilesystem") {
		t.Errorf("GetKind: expected 'kfilesystem', got %q", got)
	}
	if got := c.GetContextPrefix("/anything"); got != "ctx-prefix" {
		t.Errorf("GetContextPrefix: expected 'ctx-prefix', got %q", got)
	}
	if err := c.VerifyKubeconfigPaths(); err != nil {
		t.Errorf("VerifyKubeconfigPaths: unexpected error %v", err)
	}
	if upstream.verifyCalls != 1 {
		t.Errorf("expected VerifyKubeconfigPaths upstream call, got %d", upstream.verifyCalls)
	}
}
