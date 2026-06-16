package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/MichaelSp/kswitch/types"
)

func TestLoadConfigFromFile_NotFound(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "missing.yaml")
	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %#v", cfg)
	}
}

func TestLoadConfigFromFile_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.yaml")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil empty config")
	}
	if cfg.Version != "" || cfg.Kind != "" || len(cfg.KubeconfigStores) != 0 {
		t.Fatalf("expected zero-value config, got %#v", cfg)
	}
}

func TestLoadConfigFromFile_ValidV1(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yml := `kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
  - kind: filesystem
    paths:
      - /tmp/kube
`
	if err := os.WriteFile(path, []byte(yml), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}
	if cfg.Kind != "SwitchConfig" {
		t.Fatalf("Kind: got %q", cfg.Kind)
	}
	if cfg.Version != "v1alpha1" {
		t.Fatalf("Version: got %q", cfg.Version)
	}
	if len(cfg.KubeconfigStores) != 1 {
		t.Fatalf("expected 1 store, got %d", len(cfg.KubeconfigStores))
	}
	if cfg.KubeconfigStores[0].Kind != types.StoreKindFilesystem {
		t.Fatalf("store kind: got %q", cfg.KubeconfigStores[0].Kind)
	}
	if len(cfg.KubeconfigStores[0].Paths) != 1 || cfg.KubeconfigStores[0].Paths[0] != "/tmp/kube" {
		t.Fatalf("paths: got %v", cfg.KubeconfigStores[0].Paths)
	}
}

func TestLoadConfigFromFile_OldFormatTriggersMigration(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "old.yaml")
	yml := `kind: SwitchConfig
kubeconfigName: config
vaultAPIAddress: https://vault.example.com
kubeconfigPaths:
  - path: /home/me/.kube
    store: filesystem
  - path: secret/kubeconfigs
    store: vault
`
	if err := os.WriteFile(path, []byte(yml), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil migrated config")
	}
	if cfg.Version != "v1alpha1" {
		t.Fatalf("expected migrated version v1alpha1, got %q", cfg.Version)
	}
	if cfg.Kind != "SwitchConfig" {
		t.Fatalf("expected migrated kind SwitchConfig, got %q", cfg.Kind)
	}
	if len(cfg.KubeconfigStores) != 2 {
		t.Fatalf("expected 2 stores after migration, got %d", len(cfg.KubeconfigStores))
	}

	// Backup file should exist
	backupPath := path + ".old"
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup at %s: %v", backupPath, err)
	}

	// Original file should now be in new format
	newBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if !strings.Contains(string(newBytes), "v1alpha1") {
		t.Fatalf("expected migrated file to contain v1alpha1, got:\n%s", string(newBytes))
	}
	if !strings.Contains(string(newBytes), "kubeconfigStores:") {
		t.Fatalf("expected migrated file to contain kubeconfigStores key, got:\n%s", string(newBytes))
	}
}

func TestLoadConfigFromFile_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid.yaml")
	// Truly invalid YAML - unclosed bracket
	if err := os.WriteFile(path, []byte("kind: [unclosed\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := LoadConfigFromFile(path)
	if err == nil {
		t.Fatalf("expected error for invalid YAML, got cfg=%#v", cfg)
	}
}

func TestMigrateConfig_WritesBackupAndNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "switch.yaml")

	// Write some original content so we can verify backup
	originalYAML := `kind: SwitchConfig
kubeconfigName: original
kubeconfigPaths:
  - path: /a
    store: filesystem
`
	if err := os.WriteFile(path, []byte(originalYAML), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	old := types.ConfigOld{
		Kind:           "SwitchConfig",
		KubeconfigName: "original",
		KubeconfigPaths: []types.KubeconfigPath{
			{Path: "/a", Store: types.StoreKindFilesystem},
		},
	}

	cfg, err := MigrateConfig(old, path)
	if err != nil {
		t.Fatalf("MigrateConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}
	if cfg.Version != "v1alpha1" {
		t.Fatalf("expected v1alpha1, got %q", cfg.Version)
	}
	if cfg.KubeconfigName == nil || *cfg.KubeconfigName != "original" {
		t.Fatalf("expected migrated kubeconfigName 'original', got %v", cfg.KubeconfigName)
	}
	if len(cfg.KubeconfigStores) != 1 {
		t.Fatalf("expected 1 store, got %d", len(cfg.KubeconfigStores))
	}

	// Backup file exists and is valid YAML of ConfigOld
	backupBytes, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	var backup types.ConfigOld
	if err := yaml.Unmarshal(backupBytes, &backup); err != nil {
		t.Fatalf("backup is not valid YAML: %v", err)
	}
	if backup.KubeconfigName != "original" {
		t.Fatalf("backup kubeconfigName: got %q", backup.KubeconfigName)
	}

	// Original file is now in new format
	newBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	var newCfg types.Config
	if err := yaml.Unmarshal(newBytes, &newCfg); err != nil {
		t.Fatalf("new file is not valid YAML: %v", err)
	}
	if newCfg.Version != "v1alpha1" {
		t.Fatalf("new file version: got %q", newCfg.Version)
	}
}
