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

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MichaelSp/kswitch/types"
	"gopkg.in/yaml.v3"
)

const (
	legacyAliasFileName = "alias"   // legacy single-file format
	aliasDirName        = "aliases" // per-alias directory
)

type Alias struct {
	aliasDir string
	Content  types.ContextAlias
}

// GetDefaultAlias returns the alias state, loading from the per-alias directory.
// Automatically migrates from the legacy single YAML file on first use.
func GetDefaultAlias(stateDir string) (*Alias, error) {
	a := Alias{
		aliasDir: filepath.Join(stateDir, aliasDirName),
	}

	if err := a.load(stateDir); err != nil {
		return nil, err
	}

	return &a, nil
}

func (a *Alias) load(stateDir string) error {
	if _, err := os.Stat(a.aliasDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return a.migrateFromLegacy(stateDir)
	}
	return a.loadFromDir()
}

func (a *Alias) loadFromDir() error {
	entries, err := os.ReadDir(a.aliasDir)
	if err != nil {
		return fmt.Errorf("failed to read alias directory %q: %w", a.aliasDir, err)
	}

	a.Content.ContextToAliasMapping = make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		aliasName := entry.Name()
		data, err := os.ReadFile(filepath.Join(a.aliasDir, aliasName))
		if err != nil {
			return fmt.Errorf("failed to read alias file %q: %w", aliasName, err)
		}
		if ctx := strings.TrimSpace(string(data)); ctx != "" {
			a.Content.ContextToAliasMapping[ctx] = aliasName
		}
	}
	return nil
}

// migrateFromLegacy reads the old switch.alias YAML and writes it into the
// per-alias directory, then removes the old file.
func (a *Alias) migrateFromLegacy(stateDir string) error {
	legacyPath := filepath.Join(stateDir, "switch."+legacyAliasFileName)
	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return fmt.Errorf("failed to read legacy alias file: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &a.Content); err != nil {
			return fmt.Errorf("failed to parse legacy alias file: %w", err)
		}
	}

	if err := os.MkdirAll(a.aliasDir, 0700); err != nil {
		return fmt.Errorf("failed to create alias directory: %w", err)
	}

	if err := a.WriteAllAliases(); err != nil {
		return err
	}

	_ = os.Remove(legacyPath)
	return nil
}

// WriteAlias persists a single alias→context mapping atomically.
// Returns the previously mapped context name if the alias was already in use.
func (a *Alias) WriteAlias(aliasName, contextName string) (*string, error) {
	if err := validateAliasName(aliasName); err != nil {
		return nil, err
	}

	if a.Content.ContextToAliasMapping == nil {
		a.Content.ContextToAliasMapping = make(map[string]string, 1)
	}

	replaced := a.ContainsAlias(aliasName)
	if replaced != nil {
		delete(a.Content.ContextToAliasMapping, *replaced)
	}

	a.Content.ContextToAliasMapping[contextName] = aliasName

	if err := os.MkdirAll(a.aliasDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create alias directory: %w", err)
	}

	if err := writeAliasFile(a.aliasDir, aliasName, contextName); err != nil {
		return nil, err
	}

	return replaced, nil
}

// ContainsAlias checks if the given alias already exists.
// Returns the context name currently mapped to it, or nil.
func (a *Alias) ContainsAlias(alias string) *string {
	for context, a := range a.Content.ContextToAliasMapping {
		if alias == a {
			return &context
		}
	}
	return nil
}

// WriteAllAliases writes every in-memory alias as an individual file and
// removes any on-disk files that are no longer in the mapping.
func (a *Alias) WriteAllAliases() error {
	if err := os.MkdirAll(a.aliasDir, 0700); err != nil {
		return fmt.Errorf("failed to create alias directory: %w", err)
	}

	desired := make(map[string]struct{}, len(a.Content.ContextToAliasMapping))
	for contextName, aliasName := range a.Content.ContextToAliasMapping {
		// a legacy alias file is the one source of names that never went through
		// WriteAlias, so drop anything unusable rather than failing the migration
		// and leaving the user with no aliases at all
		if err := validateAliasName(aliasName); err != nil {
			continue
		}
		desired[aliasName] = struct{}{}
		if err := writeAliasFile(a.aliasDir, aliasName, contextName); err != nil {
			return err
		}
	}

	entries, err := os.ReadDir(a.aliasDir)
	if err != nil {
		return fmt.Errorf("failed to read alias directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, ok := desired[entry.Name()]; !ok {
			_ = os.Remove(filepath.Join(a.aliasDir, entry.Name()))
		}
	}

	return nil
}

// validateAliasName rejects any alias that is not a single, plain filename. The
// alias becomes a file inside the alias directory, so a name such as
// "../../../.kube/config" would otherwise let the join escape that directory and
// overwrite an unrelated file. Names starting with a dot are refused as well,
// because loadFromDir skips dotfiles and such an alias could never be read back.
func validateAliasName(aliasName string) error {
	switch {
	case aliasName == "":
		return fmt.Errorf("alias name must not be empty")
	case aliasName == "." || aliasName == "..":
		return fmt.Errorf("alias name %q is reserved", aliasName)
	case strings.HasPrefix(aliasName, "."):
		return fmt.Errorf("alias name %q must not start with a dot", aliasName)
	case strings.ContainsAny(aliasName, `/\`):
		return fmt.Errorf("alias name %q must not contain a path separator", aliasName)
	}
	return nil
}

// writeAliasFile atomically writes one alias file: filename=aliasName, content=contextName.
func writeAliasFile(dir, aliasName, contextName string) error {
	if err := validateAliasName(aliasName); err != nil {
		return err
	}

	path := filepath.Join(dir, aliasName)
	tmp, err := os.CreateTemp(dir, ".alias-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp alias file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.WriteString(contextName); err != nil {
		return fmt.Errorf("failed to write alias file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp alias file: %w", err)
	}
	return os.Rename(tmpName, path)
}
