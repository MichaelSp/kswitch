package clean

import (
	"os"
	"path/filepath"
	"testing"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
	"github.com/sirupsen/logrus"
)

type fakeStore struct {
	id string
}

func (f *fakeStore) GetID() string                       { return f.id }
func (f *fakeStore) GetKind() types.StoreKind            { return types.StoreKind("fake") }
func (f *fakeStore) GetContextPrefix(path string) string { return "" }
func (f *fakeStore) VerifyKubeconfigPaths() error        { return nil }
func (f *fakeStore) StartSearch(channel chan storetypes.SearchResult) {
	close(channel)
}
func (f *fakeStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	return nil, nil
}
func (f *fakeStore) GetLogger() *logrus.Entry              { return logrus.NewEntry(logrus.New()) }
func (f *fakeStore) GetStoreConfig() types.KubeconfigStore { return types.KubeconfigStore{} }

type flushableStore struct {
	fakeStore
	flushCount  int
	flushErr    error
	flushCalled bool
}

func (f *flushableStore) Flush() (int, error) {
	f.flushCalled = true
	return f.flushCount, f.flushErr
}

func setupTempKubeconfigDir(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	tempDir := filepath.Join(homeDir, ".kube", ".switch_tmp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return tempDir
}

func TestClean_EmptyStores_RemovesTempDir(t *testing.T) {
	tempDir := setupTempKubeconfigDir(t)

	dummyFile := filepath.Join(tempDir, "config1")
	if err := os.WriteFile(dummyFile, []byte("foo"), 0644); err != nil {
		t.Fatalf("write dummy file: %v", err)
	}

	if err := Clean(nil); err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("expected temp dir to be removed, stat err=%v", err)
	}
}

func TestClean_NonFlushableStore(t *testing.T) {
	setupTempKubeconfigDir(t)

	stores := []storetypes.KubeconfigStore{
		&fakeStore{id: "fake.default"},
	}
	if err := Clean(stores); err != nil {
		t.Fatalf("Clean error: %v", err)
	}
}

func TestClean_FlushableStore(t *testing.T) {
	setupTempKubeconfigDir(t)

	flushable := &flushableStore{
		fakeStore:  fakeStore{id: "fake.cached"},
		flushCount: 7,
	}
	stores := []storetypes.KubeconfigStore{flushable}

	if err := Clean(stores); err != nil {
		t.Fatalf("Clean error: %v", err)
	}
	if !flushable.flushCalled {
		t.Errorf("expected Flush to be called on flushable store")
	}
}

func TestClean_TempDirDoesNotExist(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	// Do not create the temp dir; Clean should still succeed.

	if err := Clean(nil); err != nil {
		t.Fatalf("Clean error: %v", err)
	}
}
