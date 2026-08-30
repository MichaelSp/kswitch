// Copyright 2026 The Kswitch authors
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

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/api/googleapi"

	"github.com/MichaelSp/kswitch/types"
)

func TestMatchesProjectPatterns(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		patterns  []string
		expected  bool
	}{
		{name: "no patterns matches everything", projectID: "acme-prod", expected: true},
		{name: "include match", projectID: "acme-prod", patterns: []string{"acme-*"}, expected: true},
		{name: "include miss", projectID: "other-prod", patterns: []string{"acme-*"}, expected: false},
		{name: "exclude wins over include", projectID: "acme-sandbox-1", patterns: []string{"acme-*", "!acme-sandbox-*"}, expected: false},
		{name: "only excludes keeps the rest", projectID: "acme-prod", patterns: []string{"!*-sandbox"}, expected: true},
		{name: "only excludes drops the match", projectID: "acme-sandbox", patterns: []string{"!*-sandbox"}, expected: false},
		{name: "several includes", projectID: "beta-dev", patterns: []string{"acme-*", "beta-*"}, expected: true},
		{name: "invalid pattern is ignored", projectID: "acme-prod", patterns: []string{"[", "acme-*"}, expected: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchesProjectPatterns(test.projectID, test.patterns); got != test.expected {
				t.Errorf("matchesProjectPatterns(%q, %v) = %v, want %v", test.projectID, test.patterns, got, test.expected)
			}
		})
	}
}

func TestIsPermanentProjectError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "billing disabled", err: &googleapi.Error{Code: 403}, expected: true},
		{name: "not found", err: &googleapi.Error{Code: 404}, expected: true},
		{name: "wrapped api error", err: fmt.Errorf("list clusters: %w", &googleapi.Error{Code: 403}), expected: true},
		{name: "rate limited", err: &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "rateLimitExceeded"}}}, expected: false},
		{name: "server error", err: &googleapi.Error{Code: 500}, expected: false},
		{name: "non api error", err: fmt.Errorf("connection reset"), expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isPermanentProjectError(test.err); got != test.expected {
				t.Errorf("isPermanentProjectError(%v) = %v, want %v", test.err, got, test.expected)
			}
		})
	}
}

func TestProjectCache(t *testing.T) {
	dir := t.TempDir()
	discovered := []*gkeProject{{ID: "acme-prod", Name: "Acme Prod"}, {ID: "acme-dev", Name: "Acme Dev"}}

	c := newProjectCache(dir, "gke.test", time.Hour, time.Hour)
	if c.isFresh() {
		t.Error("expected an empty cache to not be fresh")
	}

	c.setProjects(discovered)
	if !c.isFresh() {
		t.Error("expected the cache to be fresh after discovery")
	}
	if got := len(c.getProjects()); got != 2 {
		t.Errorf("expected 2 cached projects, got %d", got)
	}

	if c.shouldSkip("acme-prod") {
		t.Error("expected a usable project to not be skipped")
	}
	c.markUnusable("acme-prod")
	if !c.shouldSkip("acme-prod") {
		t.Error("expected an unusable project to be skipped")
	}

	if err := c.write(); err != nil {
		t.Fatalf("failed to write the project cache: %v", err)
	}

	// the projects and the unusable marker survive a reload
	reloaded := newProjectCache(dir, "gke.test", time.Hour, time.Hour)
	if !reloaded.isFresh() || len(reloaded.getProjects()) != 2 {
		t.Fatalf("expected 2 fresh projects after reload, got %d (fresh=%v)", len(reloaded.getProjects()), reloaded.isFresh())
	}
	if !reloaded.shouldSkip("acme-prod") {
		t.Error("expected the persisted unusable marker to be honored")
	}

	// a project that serves clusters again is queried again
	reloaded.markUsable("acme-prod")
	if reloaded.shouldSkip("acme-prod") {
		t.Error("expected the cleared project to not be skipped")
	}

	// re-discovery keeps the markers of the projects that are still there
	reloaded.markUnusable("acme-dev")
	reloaded.setProjects([]*gkeProject{{ID: "acme-dev", Name: "Acme Dev"}, {ID: "acme-new", Name: "Acme New"}})
	if !reloaded.shouldSkip("acme-dev") {
		t.Error("expected the unusable marker to survive re-discovery")
	}
	if got := len(reloaded.getProjects()); got != 2 {
		t.Errorf("expected the cache to hold the 2 re-discovered projects, got %d", got)
	}
}

func TestProjectCacheExpiry(t *testing.T) {
	dir := t.TempDir()

	c := newProjectCache(dir, "gke.test", time.Hour, time.Hour)
	c.setProjects([]*gkeProject{{ID: "acme-prod", Name: "Acme Prod"}})
	c.markUnusable("acme-prod")
	if err := c.write(); err != nil {
		t.Fatalf("failed to write the project cache: %v", err)
	}

	// expired projects are listed again, expired markers are probed again
	expired := newProjectCache(dir, "gke.test", time.Nanosecond, time.Nanosecond)
	if expired.isFresh() {
		t.Error("expected the expired cache to not be fresh")
	}
	if expired.shouldSkip("acme-prod") {
		t.Error("expected the expired unusable marker to be dropped")
	}
}

func TestProjectCacheDisabled(t *testing.T) {
	dir := t.TempDir()

	c := newProjectCache(dir, "gke.test", 0, 0)
	c.setProjects([]*gkeProject{{ID: "acme-prod", Name: "Acme Prod"}})
	if c.isFresh() {
		t.Error("expected a disabled cache to never be fresh")
	}
	c.markUnusable("acme-prod")
	if c.shouldSkip("acme-prod") {
		t.Error("expected a disabled skip list to never skip a project")
	}
}

func TestMaxConcurrentProjectRequests(t *testing.T) {
	s := &GKEStore{Config: &types.StoreConfigGKE{}}
	if got := s.maxConcurrentProjectRequests(); got != defaultMaxConcurrentProjectRequests {
		t.Errorf("expected the default of %d, got %d", defaultMaxConcurrentProjectRequests, got)
	}

	configured := 4
	s.Config.MaxConcurrentProjectRequests = &configured
	if got := s.maxConcurrentProjectRequests(); got != configured {
		t.Errorf("expected the configured %d, got %d", configured, got)
	}

	invalid := 0
	s.Config.MaxConcurrentProjectRequests = &invalid
	if got := s.maxConcurrentProjectRequests(); got != defaultMaxConcurrentProjectRequests {
		t.Errorf("expected the default of %d for a non-positive value, got %d", defaultMaxConcurrentProjectRequests, got)
	}
}

func TestActiveAccountFromGcloudConfig(t *testing.T) {
	t.Run("environment variable wins", func(t *testing.T) {
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "env@example.com")
		t.Setenv("CLOUDSDK_CONFIG", t.TempDir())

		account, ok := activeAccountFromGcloudConfig()
		if !ok || account != "env@example.com" {
			t.Errorf("got (%q, %v), want (\"env@example.com\", true)", account, ok)
		}
	})

	t.Run("reads the active configuration", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("CLOUDSDK_CONFIG", dir)
		writeGcloudConfig(t, dir, "work", "[compute]\nregion = europe-west1\n\n[core]\ndisable_usage_reporting = True\naccount = user@example.com\n")

		account, ok := activeAccountFromGcloudConfig()
		if !ok || account != "user@example.com" {
			t.Errorf("got (%q, %v), want (\"user@example.com\", true)", account, ok)
		}
	})

	t.Run("account outside of the core section is ignored", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("CLOUDSDK_CONFIG", dir)
		writeGcloudConfig(t, dir, "work", "[billing]\naccount = billing@example.com\n")

		if account, ok := activeAccountFromGcloudConfig(); ok {
			t.Errorf("expected no account, got %q", account)
		}
	})

	t.Run("missing configuration falls back to gcloud", func(t *testing.T) {
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("CLOUDSDK_CONFIG", t.TempDir())

		if account, ok := activeAccountFromGcloudConfig(); ok {
			t.Errorf("expected no account, got %q", account)
		}
	})

	t.Run("active_config with a path separator is rejected", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("CLOUDSDK_CONFIG", dir)
		if err := os.WriteFile(filepath.Join(dir, "active_config"), []byte("../../etc/passwd"), 0600); err != nil {
			t.Fatal(err)
		}

		if account, ok := activeAccountFromGcloudConfig(); ok {
			t.Errorf("expected no account, got %q", account)
		}
	})

	t.Run("a configuration symlinked outside of the config directory is rejected", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("CLOUDSDK_CONFIG", dir)

		outside := filepath.Join(t.TempDir(), "elsewhere")
		if err := os.WriteFile(outside, []byte("[core]\naccount = leaked@example.com\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "configurations"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "configurations", "config_work")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "active_config"), []byte("work\n"), 0600); err != nil {
			t.Fatal(err)
		}

		if account, ok := activeAccountFromGcloudConfig(); ok {
			t.Errorf("expected no account, got %q", account)
		}
	})
}

func writeGcloudConfig(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, "configurations"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active_config"), []byte(name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configurations", "config_"+name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
