// Copyright 2026 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubeconfigutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const kubeconfigWithToken = `apiVersion: v1
kind: Config
current-context: ctx
contexts:
- name: ctx
  context:
    cluster: cluster
    user: user
users:
- name: user
  user:
    token: a-bearer-token
`

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has mode %04o, want %04o", path, got, want)
	}
}

func TestKubeconfig_WriteKubeconfigFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not enforced on windows")
	}

	t.Run("a newly created kubeconfig is only readable by its owner", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config")

		k, err := New([]byte(kubeconfigWithToken), path, false)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		written, err := k.WriteKubeconfigFile()
		if err != nil {
			t.Fatalf("WriteKubeconfigFile failed: %v", err)
		}

		assertPerm(t, written, 0600)

		// the credential really is in there, so the mode matters
		data, err := os.ReadFile(written)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !strings.Contains(string(data), "a-bearer-token") {
			t.Errorf("expected the kubeconfig to round-trip, got:\n%s", data)
		}
	})

	t.Run("an existing kubeconfig keeps its mode", func(t *testing.T) {
		// kswitch rewrites $KUBECONFIG in place for `ns`, `set-context` and friends.
		// That file belongs to the user, so switching contexts must not quietly
		// re-permission it.
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, []byte(kubeconfigWithToken), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		if err := os.Chmod(path, 0644); err != nil { // defeat the umask
			t.Fatalf("Chmod failed: %v", err)
		}

		k, err := NewKubeconfigForPath(path)
		if err != nil {
			t.Fatalf("NewKubeconfigForPath failed: %v", err)
		}
		if _, err := k.WriteKubeconfigFile(); err != nil {
			t.Fatalf("WriteKubeconfigFile failed: %v", err)
		}

		assertPerm(t, path, 0644)
	})

	t.Run("a temporary kubeconfig and its directory are owner only", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "switch_tmp")

		k, err := New([]byte(kubeconfigWithToken), dir, true)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		written, err := k.WriteKubeconfigFile()
		if err != nil {
			t.Fatalf("WriteKubeconfigFile failed: %v", err)
		}

		assertPerm(t, dir, 0700)
		assertPerm(t, written, 0600)
	})
}
