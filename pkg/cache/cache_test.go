// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package cache

import (
	"errors"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

type fakeStore struct{ id string }

func (f *fakeStore) GetID() string                            { return f.id }
func (f *fakeStore) GetKind() types.StoreKind                 { return "" }
func (f *fakeStore) GetContextPrefix(string) string           { return "" }
func (f *fakeStore) VerifyKubeconfigPaths() error             { return nil }
func (f *fakeStore) StartSearch(chan storetypes.SearchResult) {}
func (f *fakeStore) GetKubeconfigForPath(string, map[string]string) ([]byte, error) {
	return nil, nil
}
func (f *fakeStore) GetLogger() *logrus.Entry              { return logrus.NewEntry(logrus.New()) }
func (f *fakeStore) GetStoreConfig() types.KubeconfigStore { return types.KubeconfigStore{} }

func TestNew_RegisteredFactoryReturnsResult(t *testing.T) {
	const kind = "test-cache-1"
	want := &fakeStore{id: "wrapped"}
	called := false
	Register(kind, func(s storetypes.KubeconfigStore, cfg *types.Cache) (storetypes.KubeconfigStore, error) {
		called = true
		return want, nil
	})

	upstream := &fakeStore{id: "upstream"}
	got, err := New(kind, upstream, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Errorf("factory was not invoked")
	}
	if got != want {
		t.Errorf("expected returned store %v, got %v", want, got)
	}
}

func TestNew_UnknownKindReturnsError(t *testing.T) {
	_, err := New("does-not-exist-xyz", &fakeStore{}, nil)
	if err == nil {
		t.Fatalf("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "no cache factory registered") {
		t.Errorf("expected error to contain 'no cache factory registered', got: %v", err)
	}
}

func TestNew_FactoryErrorPropagates(t *testing.T) {
	const kind = "test-cache-2"
	wantErr := errors.New("boom")
	Register(kind, func(s storetypes.KubeconfigStore, cfg *types.Cache) (storetypes.KubeconfigStore, error) {
		return nil, wantErr
	})

	got, err := New(kind, &fakeStore{}, nil)
	if got != nil {
		t.Errorf("expected nil store on error, got %v", got)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}
