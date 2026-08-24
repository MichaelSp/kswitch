// Copyright 2021 The Kswitch authors
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

package tui

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MichaelSp/kswitch/types"
	tea "github.com/charmbracelet/bubbletea"
)

// ---- itemFor ----

func TestItemFor_CarriesStoreIDAndTags(t *testing.T) {
	tags := map[string]string{"project": "garden-foo"}
	it := itemFor(ContextItem{
		ContextName: "my-context",
		StoreKind:   string(types.StoreKindGKE),
		Path:        "gke_proj--europe-west1--prod",
		Tags:        tags,
		StoreID:     "gke.default",
	})

	if it.contextName != "my-context" {
		t.Errorf("contextName = %q, want my-context", it.contextName)
	}
	if it.path != "gke_proj--europe-west1--prod" {
		t.Errorf("path = %q", it.path)
	}
	if it.storeID != "gke.default" {
		t.Errorf("storeID = %q, want gke.default", it.storeID)
	}
	if !reflect.DeepEqual(it.tags, tags) {
		t.Errorf("tags = %v, want %v", it.tags, tags)
	}
}

func TestItemFor_SamePathFromDifferentStores(t *testing.T) {
	// Two stores can each expose a cluster under the same path (e.g. "prod").
	// Every item must keep the storeID and tags of the store it came from.
	a := itemFor(ContextItem{
		ContextName: "prod",
		StoreKind:   string(types.StoreKindGKE),
		Path:        "prod",
		Tags:        map[string]string{"account": "a"},
		StoreID:     "gke.default",
	})
	b := itemFor(ContextItem{
		ContextName: "prod",
		StoreKind:   string(types.StoreKindEKS),
		Path:        "prod",
		Tags:        map[string]string{"account": "b"},
		StoreID:     "eks.default",
	})

	if a.storeID == b.storeID {
		t.Fatalf("both items resolved to store %q", a.storeID)
	}
	if a.storeID != "gke.default" || b.storeID != "eks.default" {
		t.Errorf("storeIDs = %q / %q, want gke.default / eks.default", a.storeID, b.storeID)
	}
	if a.tags["account"] != "a" || b.tags["account"] != "b" {
		t.Errorf("tags = %v / %v, want account a / account b", a.tags, b.tags)
	}
}

// ---- selectionFor ----

func TestSelectionFor_ReturnsIdentityOfSelectedItem(t *testing.T) {
	tags := map[string]string{"project": "garden-foo"}
	m := NewModel(nil, false)
	m.Selected = &item{
		contextName: "my-context",
		path:        "some/path",
		storeID:     "gardener.default",
		tags:        tags,
	}

	path, ctx, storeID, gotTags, dynStore, err := selectionFor(m)
	if err != nil {
		t.Fatalf("selectionFor() error = %v", err)
	}
	if path != "some/path" {
		t.Errorf("path = %q, want some/path", path)
	}
	if ctx != "my-context" {
		t.Errorf("selectedContext = %q, want my-context", ctx)
	}
	if storeID != "gardener.default" {
		t.Errorf("storeID = %q, want gardener.default", storeID)
	}
	if !reflect.DeepEqual(gotTags, tags) {
		t.Errorf("tags = %v, want %v", gotTags, tags)
	}
	if dynStore != nil {
		t.Errorf("dynamicStore = %v, want nil", dynStore)
	}
}

func TestSelectionFor_Aborted(t *testing.T) {
	m := NewModel(nil, false)
	m.Aborted = true
	m.Selected = &item{path: "p"}

	if _, _, _, _, _, err := selectionFor(m); !errors.Is(err, ErrAbort) {
		t.Errorf("error = %v, want ErrAbort", err)
	}
}

func TestSelectionFor_NothingSelected(t *testing.T) {
	if _, _, _, _, _, err := selectionFor(NewModel(nil, false)); !errors.Is(err, ErrAbort) {
		t.Errorf("error = %v, want ErrAbort", err)
	}
}

func TestSelectionFor_DynamicStore(t *testing.T) {
	dyn := &stubStore{id: "k0smotron.parent"}
	m := NewModel(nil, false)
	m.dynamicStores["k0smotron.parent"] = dyn
	m.Selected = &item{path: "parent/ns/child", storeID: "k0smotron.parent"}

	_, _, storeID, _, dynStore, err := selectionFor(m)
	if err != nil {
		t.Fatalf("selectionFor() error = %v", err)
	}
	if storeID != "k0smotron.parent" {
		t.Errorf("storeID = %q, want k0smotron.parent", storeID)
	}
	if dynStore != dyn {
		t.Errorf("dynamicStore = %v, want the registered store", dynStore)
	}
}

func TestSelectionFor_NoDynamicStoreForRegularItem(t *testing.T) {
	m := NewModel(nil, false)
	m.dynamicStores["k0smotron.parent"] = &stubStore{id: "k0smotron.parent"}
	m.Selected = &item{path: "prod", storeID: "gke.default"}

	if _, _, _, _, dynStore, _ := selectionFor(m); dynStore != nil {
		t.Errorf("dynamicStore = %v, want nil for a non-k0smotron selection", dynStore)
	}
}

// ---- selection through the model, as Run drives it ----

// updateModel feeds msg to the model the way the bubbletea event loop does.
func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	updated, _ := m.Update(msg)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", updated)
	}
	return next
}

// TestEnterSelectsStoreOfCursorItem is the regression test for resolving the
// selection by identity: two stores expose a cluster under the same path, and
// whichever one the cursor sits on must be the one returned — not the one that
// happened to be discovered last.
func TestEnterSelectsStoreOfCursorItem(t *testing.T) {
	items := []ContextItem{
		{ContextName: "gke-prod", StoreKind: string(types.StoreKindGKE), Path: "prod", StoreID: "gke.default", Tags: map[string]string{"account": "a"}},
		{ContextName: "eks-prod", StoreKind: string(types.StoreKindEKS), Path: "prod", StoreID: "eks.default", Tags: map[string]string{"account": "b"}},
	}

	for cursor, want := range items {
		m := NewModel(nil, false)
		m.width, m.height = 80, 24

		batch := make([]item, 0, len(items))
		for _, ci := range items {
			batch = append(batch, itemFor(ci))
		}
		m = updateModel(t, m, itemsMsg(batch))
		m.cursor = cursor
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

		path, ctx, storeID, tags, _, err := selectionFor(m)
		if err != nil {
			t.Fatalf("cursor %d: selectionFor() error = %v", cursor, err)
		}
		if path != want.Path {
			t.Errorf("cursor %d: path = %q, want %q", cursor, path, want.Path)
		}
		if ctx != want.ContextName {
			t.Errorf("cursor %d: selectedContext = %q, want %q", cursor, ctx, want.ContextName)
		}
		if storeID != want.StoreID {
			t.Errorf("cursor %d: storeID = %q, want %q", cursor, storeID, want.StoreID)
		}
		if !reflect.DeepEqual(tags, want.Tags) {
			t.Errorf("cursor %d: tags = %v, want %v", cursor, tags, want.Tags)
		}
	}
}

func TestEnterOnEmptyListAborts(t *testing.T) {
	m := NewModel(nil, false)
	m.width, m.height = 80, 24

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if _, _, _, _, _, err := selectionFor(m); !errors.Is(err, ErrAbort) {
		t.Errorf("error = %v, want ErrAbort", err)
	}
}
