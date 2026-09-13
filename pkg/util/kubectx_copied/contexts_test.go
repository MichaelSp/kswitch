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

package kubeconfigutil

import (
	"slices"
	"testing"
)

func TestKubeconfig_GetContextNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kubeconfig string
		want       []string
		wantErr    bool
	}{
		{
			name: "every context is named",
			kubeconfig: `apiVersion: v1
kind: Config
contexts:
- name: admin@production
- name: admin@staging
`,
			want: []string{"admin@production", "admin@staging"},
		},
		{
			// ResolveContextName runs this on the kubeconfig of every selected context,
			// so a hand-edited entry without a name must not take the process down
			name: "a context entry without a name is skipped",
			kubeconfig: `apiVersion: v1
kind: Config
contexts:
- cluster: production
- name: admin@staging
`,
			want: []string{"admin@staging"},
		},
		{
			name: "a context entry that is not a mapping is skipped",
			kubeconfig: `apiVersion: v1
kind: Config
contexts:
- admin@production
- name: admin@staging
`,
			want: []string{"admin@staging"},
		},
		{
			name: "no contexts entry at all",
			kubeconfig: `apiVersion: v1
kind: Config
`,
			wantErr: true,
		},
		{
			name: "the contexts entry is not a sequence",
			kubeconfig: `apiVersion: v1
kind: Config
contexts: admin@production
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kubeconfig, err := New([]byte(tt.kubeconfig), "kubeconfig", false)
			if err != nil {
				t.Fatalf("failed to parse the kubeconfig: %v", err)
			}

			got, err := kubeconfig.GetContextNames()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetContextNames() = %v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetContextNames failed: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("GetContextNames() = %v, want %v", got, tt.want)
			}
		})
	}
}
