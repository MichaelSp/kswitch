// Copyright 2024 The Kswitch authors
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
	"testing"

	"github.com/MichaelSp/kswitch/types"
)

func TestFormatDisplayName_GardenerWithLabels(t *testing.T) {
	path := "canary--shoot--mcpd--crate"
	tags := map[string]string{
		"gardener.clusters.openmcp.cloud/cluster-name": "onboarding",
	}
	labelKeys := []string{"gardener.clusters.openmcp.cloud/cluster-name"}

	primary, suffix := FormatDisplayName(types.StoreKindGardener, path, "ctx-external", "", tags, labelKeys)

	if primary != "gardener/canary/garden-mcpd/crate" {
		t.Errorf("unexpected primary: %q", primary)
	}
	if suffix == "" {
		t.Fatal("expected a suffix")
	}
	// label value must appear first in suffix
	if suffix != "(onboarding, ctx-external)" {
		t.Errorf("unexpected suffix: %q", suffix)
	}
}

func TestFormatDisplayName_GardenerNoLabels(t *testing.T) {
	path := "canary--shoot--mcpd--crate"
	primary, suffix := FormatDisplayName(types.StoreKindGardener, path, "canary-shoot-mcpd-crate/garden-mcpd--crate-external", "", nil, nil)

	if primary != "gardener/canary/garden-mcpd/crate" {
		t.Errorf("unexpected primary: %q", primary)
	}
	if suffix != "(external)" {
		t.Errorf("unexpected suffix: %q", suffix)
	}
}

func TestFormatDisplayName_GardenerLabelEqualsShootName(t *testing.T) {
	// label value same as shoot name → should not appear twice
	path := "canary--shoot--mcpd--crate"
	tags := map[string]string{"some.label": "crate"}
	primary, suffix := FormatDisplayName(types.StoreKindGardener, path, "crate", "", tags, []string{"some.label"})

	if primary != "gardener/canary/garden-mcpd/crate" {
		t.Errorf("unexpected primary: %q", primary)
	}
	// "crate" is the shoot name (primary), so it must not appear in suffix
	if suffix != "" {
		t.Errorf("expected empty suffix when label value equals shoot name, got %q", suffix)
	}
}

func TestFormatDisplayName_NonGardener(t *testing.T) {
	primary, suffix := FormatDisplayName(types.StoreKindFilesystem, "some/path", "my-context", "my-alias", nil, nil)
	if primary != "my-alias" {
		t.Errorf("unexpected primary: %q", primary)
	}
	if suffix != "(my-context)" {
		t.Errorf("unexpected suffix: %q", suffix)
	}
}

func TestCollectOtherNames_LabelValueSearchable(t *testing.T) {
	tags := map[string]string{"foo/label": "onboarding"}
	others := collectOtherNames("crate", "ctx-external", "", tags, []string{"foo/label"})
	found := false
	for _, o := range others {
		if o == "onboarding" {
			found = true
		}
	}
	if !found {
		t.Errorf("label value 'onboarding' not in others: %v", others)
	}
}

func TestCollectOtherNames_DeduplicatesLabelValue(t *testing.T) {
	// label value same as alias → appear only once
	tags := map[string]string{"foo/label": "my-alias"}
	others := collectOtherNames("crate", "ctx", "my-alias", tags, []string{"foo/label"})
	count := 0
	for _, o := range others {
		if o == "my-alias" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'my-alias' exactly once, got %d times in %v", count, others)
	}
}

func TestShortGardenerContextName(t *testing.T) {
	got := shortGardenerContextName(
		"live-shoot-laascs-c37nifihj77yt7e/garden-laascs--c37nifihj77yt7e-external",
		"garden-laascs",
		"c37nifihj77yt7e",
	)
	if got != "external" {
		t.Errorf("unexpected short context name: %q", got)
	}
}
