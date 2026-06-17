package file

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	"github.com/sirupsen/logrus"
)

const minimalKubeconfigYAML = `apiVersion: v1
kind: Config
contexts: []
current-context: ""
`

type mockStore struct {
	id              string
	kind            types.StoreKind
	getKubeconfig   func(path string, tags map[string]string) ([]byte, error)
	getKubeconfigCt int
}

func (m *mockStore) GetID() string                               { return m.id }
func (m *mockStore) GetKind() types.StoreKind                    { return m.kind }
func (m *mockStore) GetContextPrefix(path string) string         { return "" }
func (m *mockStore) VerifyKubeconfigPaths() error                { return nil }
func (m *mockStore) StartSearch(ch chan storetypes.SearchResult) {}
func (m *mockStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	m.getKubeconfigCt++
	if m.getKubeconfig != nil {
		return m.getKubeconfig(path, tags)
	}
	return []byte(minimalKubeconfigYAML), nil
}
func (m *mockStore) GetLogger() *logrus.Entry              { return logrus.NewEntry(logrus.New()) }
func (m *mockStore) GetStoreConfig() types.KubeconfigStore { return types.KubeconfigStore{} }

func TestNew_NilCacheConfig(t *testing.T) {
	_, err := New(&mockStore{id: "test"}, nil)
	if err == nil {
		t.Fatalf("expected error for nil ccfg")
	}
	if !strings.Contains(err.Error(), "cache config must be provided") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_MissingPath(t *testing.T) {
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: map[string]any{},
	}
	_, err := New(&mockStore{id: "test"}, ccfg)
	if err == nil {
		t.Fatalf("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "path for filesystem cache was not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_NilConfigField(t *testing.T) {
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: nil,
	}
	_, err := New(&mockStore{id: "test"}, ccfg)
	if err == nil {
		t.Fatalf("expected error for nil config field")
	}
}

func TestNew_CreatesNonExistingDir(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "subdir", "cache")
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: map[string]any{"path": cachePath},
	}
	store, err := New(&mockStore{id: "test"}, ccfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatalf("expected store, got nil")
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("expected cache dir to be created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file")
	}
}

func TestGetKubeconfigForPath_CacheMissThenHit(t *testing.T) {
	tmp := t.TempDir()
	upstream := &mockStore{
		id:   "upstream-1",
		kind: types.StoreKindFilesystem,
	}
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: map[string]any{"path": tmp},
	}
	store, err := New(upstream, ccfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	const path = "some/cluster/path"

	// First call - cold cache, upstream should be called
	b, err := store.GetKubeconfigForPath(path, nil)
	if err != nil {
		t.Fatalf("first GetKubeconfigForPath failed: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected non-empty kubeconfig")
	}
	if upstream.getKubeconfigCt != 1 {
		t.Fatalf("expected upstream to be called once, got %d", upstream.getKubeconfigCt)
	}

	// Verify a cache file with the expected suffix was created
	files, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	expectedSuffix := ".upstream-1.cache"
	found := false
	for _, f := range files {
		if strings.HasSuffix(f.Name(), expectedSuffix) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cache file with suffix %q, files=%v", expectedSuffix, files)
	}

	// Second call - warm cache, upstream should NOT be called again
	b2, err := store.GetKubeconfigForPath(path, nil)
	if err != nil {
		t.Fatalf("second GetKubeconfigForPath failed: %v", err)
	}
	if len(b2) == 0 {
		t.Fatalf("expected non-empty kubeconfig from cache")
	}
	if upstream.getKubeconfigCt != 1 {
		t.Fatalf("expected upstream call count to remain 1, got %d", upstream.getKubeconfigCt)
	}
}

func TestGetKubeconfigForPath_UpstreamError(t *testing.T) {
	tmp := t.TempDir()
	upstream := &mockStore{
		id:   "upstream-err",
		kind: types.StoreKindFilesystem,
		getKubeconfig: func(path string, tags map[string]string) ([]byte, error) {
			return nil, errors.New("upstream failed")
		},
	}
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: map[string]any{"path": tmp},
	}
	store, err := New(upstream, ccfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	_, err = store.GetKubeconfigForPath("p", nil)
	if err == nil {
		t.Fatalf("expected error from upstream")
	}

	files, _ := os.ReadDir(tmp)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".upstream-err.cache") {
			t.Fatalf("expected no cache file to be created on upstream error, found %q", f.Name())
		}
	}
}

func TestFlush_DeletesOnlyMatchingSuffix(t *testing.T) {
	tmp := t.TempDir()
	upstream := &mockStore{id: "store-a", kind: types.StoreKindFilesystem}
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: map[string]any{"path": tmp},
	}
	storeIface, err := New(upstream, ccfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	suffix := ".store-a.cache"
	matchingFiles := []string{
		"abc" + suffix,
		"def" + suffix,
	}
	otherFiles := []string{
		"abc.store-b.cache",
		"random.txt",
	}
	for _, name := range append(matchingFiles, otherFiles...) {
		full := filepath.Join(tmp, name)
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatalf("write file %s: %v", full, err)
		}
	}

	// Add a directory; should be skipped
	if err := os.Mkdir(filepath.Join(tmp, "asubdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	type flusher interface {
		Flush() (int, error)
	}
	fc, ok := storeIface.(flusher)
	if !ok {
		t.Fatalf("store does not implement Flush")
	}
	deleted, err := fc.Flush()
	if err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if deleted != len(matchingFiles) {
		t.Fatalf("expected %d deleted, got %d", len(matchingFiles), deleted)
	}

	// matching files should be gone
	for _, name := range matchingFiles {
		if _, err := os.Stat(filepath.Join(tmp, name)); !os.IsNotExist(err) {
			t.Fatalf("expected file %s to be deleted", name)
		}
	}
	// other files should remain
	for _, name := range otherFiles {
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			t.Fatalf("expected file %s to remain: %v", name, err)
		}
	}
}

func TestPassthroughs(t *testing.T) {
	tmp := t.TempDir()
	upstream := &mockStore{id: "pass-1", kind: types.StoreKindFilesystem}
	ccfg := &types.Cache{
		Kind:   "filesystem",
		Config: map[string]any{"path": tmp},
	}
	store, err := New(upstream, ccfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if got := store.GetID(); got != "pass-1" {
		t.Fatalf("GetID: got %q", got)
	}
	if got := store.GetKind(); got != types.StoreKindFilesystem {
		t.Fatalf("GetKind: got %q", got)
	}
	if got := store.GetContextPrefix("/p"); got != "" {
		t.Fatalf("GetContextPrefix: got %q", got)
	}
	if err := store.VerifyKubeconfigPaths(); err != nil {
		t.Fatalf("VerifyKubeconfigPaths: %v", err)
	}
	if store.GetLogger() == nil {
		t.Fatalf("GetLogger returned nil")
	}
	_ = store.GetStoreConfig()

	ch := make(chan storetypes.SearchResult, 1)
	store.StartSearch(ch)
}
