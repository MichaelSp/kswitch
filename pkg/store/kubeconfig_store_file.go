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

package store

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindFilesystem, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewFilesystemStore(deps.KubeconfigName, s)
	})
}

var _ storetypes.KubeconfigStore = (*FilesystemStore)(nil)

func NewFilesystemStore(
	kubeconfigName string,
	kubeconfigStore types.KubeconfigStore,
) (*FilesystemStore, error) {
	return &FilesystemStore{
		BaseStore:      NewBaseStore(types.StoreKindFilesystem, kubeconfigStore),
		KubeconfigName: kubeconfigName,
	}, nil
}

func (s *FilesystemStore) GetContextPrefix(path string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}

	// return the name of the parent directory
	return filepath.Base(filepath.Dir(path))
}

func (s *FilesystemStore) StartSearch(channel chan storetypes.SearchResult) {
	for _, path := range s.kubeconfigFilepaths {
		channel <- storetypes.SearchResult{
			KubeconfigPath: path,
			Error:          nil,
		}
	}

	wg := sync.WaitGroup{}
	for _, path := range s.kubeconfigDirectories {
		wg.Add(1)
		go s.searchDirectory(&wg, path, channel)
	}
	wg.Wait()
}

func (s *FilesystemStore) searchDirectory(
	wg *sync.WaitGroup,
	searchPath string,
	channel chan storetypes.SearchResult,
) {
	defer wg.Done()

	if err := walkFollowSymlinks(searchPath, func(osPathname string, _ fs.DirEntry) error {
		fileName := filepath.Base(osPathname)
		matched, err := filepath.Match(s.KubeconfigName, fileName)
		if err != nil {
			return err
		}
		if matched {
			channel <- storetypes.SearchResult{
				KubeconfigPath: osPathname,
				Error:          nil,
			}
		}
		return nil
	}); err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("failed to find kubeconfig files in directory: %w", err),
		}
	}
}

// walkFollowSymlinks walks the file tree rooted at root, following directory symlinks.
// It mirrors the behaviour previously provided by godirwalk.Walk with
// FollowSymbolicLinks=true and Unsorted=false. Symlink loops are guarded by
// tracking visited canonical directory paths.
func walkFollowSymlinks(root string, fn func(path string, d fs.DirEntry) error) error {
	visited := map[string]struct{}{}
	return walkFollowSymlinksInner(root, fn, visited)
}

func walkFollowSymlinksInner(root string, fn func(path string, d fs.DirEntry) error, visited map[string]struct{}) error {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		if _, seen := visited[real]; seen {
			return nil
		}
		visited[real] = struct{}{}
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Follow directory symlinks manually since WalkDir does not.
		if d.Type()&fs.ModeSymlink != 0 {
			info, statErr := os.Stat(path)
			if statErr != nil {
				// Broken symlink: skip silently as godirwalk did by default.
				return nil
			}
			if info.IsDir() {
				if err := walkFollowSymlinksInner(path, fn, visited); err != nil {
					return err
				}
				return nil
			}
		}
		return fn(path, d)
	})
}

func (s *FilesystemStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	return os.ReadFile(path)
}

func (s *FilesystemStore) VerifyKubeconfigPaths() error {
	var (
		duplicatePath              = make(map[string]*struct{})
		validKubeconfigFilepaths   []string
		validKubeconfigDirectories []string
		usr, _                     = user.Current()
		homeDir                    = usr.HomeDir
	)

	for _, path := range s.KubeconfigStore.Paths {
		// do not add duplicate paths
		if duplicatePath[path] != nil {
			continue
		}
		duplicatePath[path] = &struct{}{}

		kubeconfigPath := path
		if kubeconfigPath == "~" {
			kubeconfigPath = homeDir
		} else if strings.HasPrefix(kubeconfigPath, "~/") {
			// Use strings.HasPrefix so we don't match paths like
			// "/something/~/something/"
			kubeconfigPath = filepath.Join(homeDir, kubeconfigPath[2:])
		}

		info, err := os.Stat(kubeconfigPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to read from the configured kubeconfig directory %q: %w", path, err)
		}

		if info.IsDir() {
			validKubeconfigDirectories = append(validKubeconfigDirectories, kubeconfigPath)
			continue
		}
		validKubeconfigFilepaths = append(validKubeconfigFilepaths, kubeconfigPath)
	}

	if len(validKubeconfigDirectories) == 0 && len(validKubeconfigFilepaths) == 0 {
		return fmt.Errorf(
			"none of the %d specified kubeconfig path(s) exist. Either specifiy an existing path via flag '--kubeconfig-path' or in the switch config file",
			len(s.KubeconfigStore.Paths),
		)
	}
	s.kubeconfigDirectories = validKubeconfigDirectories
	s.kubeconfigFilepaths = validKubeconfigFilepaths
	return nil
}
