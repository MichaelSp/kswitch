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
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
	"gopkg.in/yaml.v3"
)

const (
	// defaultRefreshProjectsAfter is how long the discovered GCP projects are reused
	// before they are listed from the Cloud Resource Manager API again.
	defaultRefreshProjectsAfter = 24 * time.Hour
	// defaultSkipUnusableProjectsFor is how long a project that cannot serve GKE clusters
	// is skipped before it is probed again.
	defaultSkipUnusableProjectsFor = 24 * time.Hour
)

// gkeProject is a GCP project that may contain GKE clusters.
type gkeProject struct {
	// ID is the technical project ID used for the API calls
	ID string `yaml:"id"`
	// Name is the human readable project name used to build the kubeconfig path
	Name string `yaml:"name"`
	// UnusableSince is set when listing the clusters of this project failed with an error
	// that does not resolve itself (billing disabled, Kubernetes Engine API not enabled,
	// no permission). Such projects are skipped until the entry expires.
	UnusableSince *time.Time `yaml:"unusableSince,omitempty"`
}

// projectCache holds the GCP projects of a GKE store between invocations.
//
// Both parts of the cache exist because listing the projects of an account that is a member
// of many organizations is expensive: it takes multiple seconds and most of the returned
// projects never contain a GKE cluster, yet each of them costs one API call per search.
type projectCache struct {
	filepath      string
	refreshAfter  time.Duration
	skipUnusable  time.Duration
	mutex         sync.Mutex
	projects      map[string]*gkeProject
	lastRefreshed time.Time
	dirty         bool
}

// projectCacheFile is the on-disk representation of the project cache.
type projectCacheFile struct {
	LastRefreshed time.Time     `yaml:"lastRefreshed"`
	Projects      []*gkeProject `yaml:"projects"`
}

// newProjectCache loads the project cache of the given store from the state directory.
// A missing or corrupt file yields an empty cache, the content can always be recomputed.
func newProjectCache(stateDir, storeID string, refreshAfter, skipUnusable time.Duration) *projectCache {
	c := &projectCache{
		filepath:     fmt.Sprintf("%s/switch.%s.projects", stateDir, storeID),
		refreshAfter: refreshAfter,
		skipUnusable: skipUnusable,
		projects:     map[string]*gkeProject{},
	}

	bytes, err := os.ReadFile(c.filepath)
	if err != nil {
		return c
	}

	content := projectCacheFile{}
	if err := yaml.Unmarshal(bytes, &content); err != nil {
		return c
	}

	for _, project := range content.Projects {
		if project == nil || len(project.ID) == 0 {
			continue
		}
		// drop expired "unusable" markers instead of carrying them over
		if project.UnusableSince != nil && !c.skipStillValid(*project.UnusableSince) {
			project.UnusableSince = nil
		}
		c.projects[project.ID] = project
	}
	c.lastRefreshed = content.LastRefreshed

	return c
}

// isFresh reports whether the cached projects can be used instead of listing them again.
func (c *projectCache) isFresh() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.refreshAfter <= 0 || len(c.projects) == 0 || c.lastRefreshed.IsZero() {
		return false
	}
	return time.Now().UTC().Before(c.lastRefreshed.UTC().Add(c.refreshAfter))
}

// setProjects replaces the cached projects with the freshly discovered ones,
// carrying over the "unusable" markers of projects that are still present.
func (c *projectCache) setProjects(discovered []*gkeProject) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	updated := make(map[string]*gkeProject, len(discovered))
	for _, project := range discovered {
		if previous, ok := c.projects[project.ID]; ok {
			project.UnusableSince = previous.UnusableSince
		}
		updated[project.ID] = project
	}

	c.projects = updated
	c.lastRefreshed = time.Now().UTC()
	c.dirty = true
}

// getProjects returns the cached projects sorted by project ID for a stable search order.
func (c *projectCache) getProjects() []*gkeProject {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	projects := make([]*gkeProject, 0, len(c.projects))
	for _, project := range c.projects {
		projects = append(projects, project)
	}
	slices.SortFunc(projects, func(a, b *gkeProject) int {
		return strings.Compare(a.ID, b.ID)
	})
	return projects
}

// shouldSkip reports whether the project recently failed to serve GKE clusters.
func (c *projectCache) shouldSkip(projectID string) bool {
	if c.skipUnusable <= 0 {
		return false
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	project, ok := c.projects[projectID]
	if !ok || project.UnusableSince == nil {
		return false
	}
	return c.skipStillValid(*project.UnusableSince)
}

// markUnusable remembers that the project cannot serve GKE clusters.
func (c *projectCache) markUnusable(projectID string) {
	if c.skipUnusable <= 0 {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	project, ok := c.projects[projectID]
	if !ok {
		return
	}
	now := time.Now().UTC()
	project.UnusableSince = &now
	c.dirty = true
}

// markUsable clears the "unusable" marker of a project that served clusters again.
func (c *projectCache) markUsable(projectID string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	project, ok := c.projects[projectID]
	if !ok || project.UnusableSince == nil {
		return
	}
	project.UnusableSince = nil
	c.dirty = true
}

// skipStillValid checks an "unusable" timestamp against the configured duration.
// Must be called with the mutex held.
func (c *projectCache) skipStillValid(unusableSince time.Time) bool {
	return time.Now().UTC().Before(unusableSince.UTC().Add(c.skipUnusable))
}

// write persists the cache. It only writes when the content changed.
func (c *projectCache) write() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.dirty {
		return nil
	}

	content := projectCacheFile{LastRefreshed: c.lastRefreshed}
	for _, project := range c.projects {
		content.Projects = append(content.Projects, project)
	}
	slices.SortFunc(content.Projects, func(a, b *gkeProject) int {
		return strings.Compare(a.ID, b.ID)
	})

	bytes, err := yaml.Marshal(content)
	if err != nil {
		return err
	}

	// the cache reveals which projects the account can see, keep it readable by the owner only
	if err := os.WriteFile(c.filepath, bytes, 0600); err != nil {
		return err
	}

	c.dirty = false
	return nil
}

// isPermanentProjectError checks if the error returned for a project is unlikely to
// resolve itself before the next search, so that the project can be skipped for a while.
// Transient conditions (rate limits, server errors) are not permanent.
func isPermanentProjectError(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.Code {
	case 401, 403, 404:
		// billing disabled, Kubernetes Engine API not enabled, project deleted or
		// no permission on the project - all stable until the user changes something
		return !isRateLimitError(apiErr)
	default:
		return false
	}
}

// isRateLimitError distinguishes quota errors, which GCP also reports as 403,
// from permanent permission errors.
func isRateLimitError(apiErr *googleapi.Error) bool {
	for _, e := range apiErr.Errors {
		if e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded" || e.Reason == "quotaExceeded" {
			return true
		}
	}
	return false
}
