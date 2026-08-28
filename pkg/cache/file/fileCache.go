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

package file

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MichaelSp/kswitch/pkg/cache"
	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/pkg/util"
	kubeconfigutil "github.com/MichaelSp/kswitch/pkg/util/kubectx_copied"
	"github.com/MichaelSp/kswitch/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const cacheKey = "filesystem"
const kubeconfigSuffix = "cache"

// cacheDirPerm restricts the cache directory to its owner. The cache holds
// fully-resolved kubeconfigs, i.e. live credentials. Because a directory has to be
// searchable to reach the files inside it, this also shields entries that earlier
// versions wrote with a permissive file mode.
const cacheDirPerm os.FileMode = 0700

// cacheFilePerm restricts a single cache entry to its owner.
const cacheFilePerm os.FileMode = 0600

func init() {
	cache.Register(cacheKey, New)
}

func New(upstream storetypes.KubeconfigStore, ccfg *types.Cache) (storetypes.KubeconfigStore, error) {
	if ccfg == nil {
		return nil, fmt.Errorf("cache config must be provided for file cache")
	}
	cfg, err := unmarshalFileCacheCfg(ccfg.Config)
	if err != nil {
		return nil, err
	}

	cfgStore := types.KubeconfigStore{}
	if len(cfg.Path) == 0 {
		return nil, fmt.Errorf("path for filesystem cache was not configured")
	}
	path := cfg.Path
	if strings.HasPrefix(path, "~/") {
		homedir, _ := os.UserHomeDir()
		path = filepath.Join(homedir, path[2:])
	}
	if err := ensureCacheDir(path); err != nil {
		return nil, err
	}
	cfgStore.Paths = []string{path}

	log := logrus.New().WithField("store", types.StoreKindFilesystem).WithField("cache", cacheKey)

	return &fileCache{
		upstream: upstream,
		cfg:      cfg,
		logger:   log,
	}, nil
}

// ensureCacheDir creates the cache directory if it does not exist yet and makes sure
// it is not reachable by group or other. Directories left behind by earlier versions
// are tightened as well, so an upgrade repairs an already-exposed cache.
func ensureCacheDir(path string) error {
	if err := os.MkdirAll(path, cacheDirPerm); err != nil {
		return fmt.Errorf("path: %s was not able to be created: %w", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path: %s was not able to be read: %w", path, err)
	}

	if info.Mode().Perm()&0077 != 0 {
		if err := os.Chmod(path, cacheDirPerm); err != nil {
			return fmt.Errorf("path: %s holds kubeconfigs but is accessible by other users and could not be restricted: %w", path, err)
		}
	}
	return nil
}

type fileCache struct {
	upstream storetypes.KubeconfigStore
	cfg      fileCacheCfg
	logger   *logrus.Entry
}

func unmarshalFileCacheCfg(cfg any) (fileCacheCfg, error) {
	var fileCacheCfg fileCacheCfg
	if cfg == nil {
		return fileCacheCfg, fmt.Errorf("cache is not configured")
	}
	buf, err := yaml.Marshal(cfg)
	if err != nil {
		return fileCacheCfg, fmt.Errorf("failed to marshal cache config: %w", err)
	}
	err = yaml.Unmarshal(buf, &fileCacheCfg)
	if err != nil {
		return fileCacheCfg, fmt.Errorf("cache config is invalid: %w", err)
	}
	return fileCacheCfg, nil
}

type fileCacheCfg struct {
	// Path to store the kubeconfigs in.
	Path string `yaml:"path"`
}

// hash for provided path
// the hash does not contain any folders or special characters and is safe to use as filename
// SHA-256 is not needed for its cryptographic strength here, but a blocklisted
// primitive is not worth defending in review. Entries cached under the old MD5
// names are simply missed and rewritten; Flush still removes them, because it
// matches on the filename suffix rather than the digest.
func (c *fileCache) hash(path string) string {
	filename := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", filename)
}

// suffix contains the UID of the Upstream store with a suffix kubeconfigSuffix"
func (c *fileCache) suffix() string {
	return fmt.Sprintf(".%s.%s", c.upstream.GetID(), kubeconfigSuffix)
}

// GetKubeconfigForPath returns the kubeconfig for the given path.
// First, it checks if the kubeconfig is already available in cache.
// If not, it is loaded from the upstream store and stored in cache
func (c *fileCache) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	c.logger.Debugf("Looking for '%s'", path)

	// check if kubeconfig is already available in the cache
	cacheFilename := fmt.Sprintf("%s%s", c.hash(path), c.suffix())
	file := filepath.Join(c.cfg.Path, cacheFilename)
	file = util.ExpandEnv(file)

	k, err := kubeconfigutil.NewKubeconfigForPath(file)
	if err == nil { // return cached kubeconfig if found
		c.logger.Debugf("kubeconfig found in cache '%s'", path)
		// entries written by earlier versions are world-readable, so tighten on use.
		// The directory mode already keeps other users out; this is a second layer,
		// and not worth failing the lookup over.
		if err := os.Chmod(file, cacheFilePerm); err != nil {
			c.logger.Debugf("failed to restrict permissions on cache entry '%s': %v", file, err)
		}
		return k.GetBytes()
	}
	c.logger.Debugf("kubeconfig not found in cache '%s'", path)
	// kubeconfig not found in cache, load from upstream store
	kubeconfig, err := c.upstream.GetKubeconfigForPath(path, tags)
	if err != nil { // if the upstream returns an error, the result is not cached
		return kubeconfig, err
	}

	// store the kubeconfig in the cache
	k, err = kubeconfigutil.New(kubeconfig, file, false)
	if err != nil {
		c.logger.Debugf("failure '%s' , %s", path, err)
		return nil, fmt.Errorf("failed to store kubeconfig in cache: %w", err)
	}
	_, err = k.WriteKubeconfigFile()
	return kubeconfig, err
}

// Flush cache by deleting all files in the cache directory
func (c *fileCache) Flush() (int, error) {
	path := util.ExpandEnv(c.cfg.Path)
	files, _ := os.ReadDir(path)
	deleted := 0
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), c.suffix()) {
			continue
		}
		err := os.Remove(filepath.Join(path, f.Name()))
		if err != nil {
			return deleted, fmt.Errorf("failed to delete file '%s': %w", f.Name(), err)
		}
		deleted++
	}
	return deleted, nil
}

// passthru requests to the upstream store

func (c *fileCache) GetID() string {
	return c.upstream.GetID()
}

func (c *fileCache) GetKind() types.StoreKind {
	return c.upstream.GetKind()
}

func (c *fileCache) GetContextPrefix(path string) string {
	return c.upstream.GetContextPrefix(path)
}

func (c *fileCache) VerifyKubeconfigPaths() error {
	return c.upstream.VerifyKubeconfigPaths()
}

func (c *fileCache) StartSearch(channel chan storetypes.SearchResult) {
	c.upstream.StartSearch(channel)
}

func (c *fileCache) GetLogger() *logrus.Entry {
	return c.upstream.GetLogger()
}
func (c *fileCache) GetStoreConfig() types.KubeconfigStore {
	return c.upstream.GetStoreConfig()
}

func (c *fileCache) GetSearchPreview(path string, optionalTags map[string]string) (string, error) {
	previewer, ok := c.upstream.(storetypes.Previewer)
	if !ok {
		// if the wrapped store is not a previewer, simply return an empty string, hence causing no visual distortion
		return "", nil
	}

	return previewer.GetSearchPreview(path, optionalTags)
}

// GetShootLabelKeys delegates to the upstream store if it supports the method.
func (c *fileCache) GetShootLabelKeys() []string {
	type lkp interface{ GetShootLabelKeys() []string }
	if p, ok := c.upstream.(lkp); ok {
		return p.GetShootLabelKeys()
	}
	return nil
}
