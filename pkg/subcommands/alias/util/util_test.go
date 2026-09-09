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

package util

import "testing"

func TestGetContextForAlias_Found(t *testing.T) {
	mapping := map[string]string{
		"alias1": "context1",
		"alias2": "context2",
	}
	if got := GetContextForAlias("alias1", mapping); got != "context1" {
		t.Errorf("expected context1, got %q", got)
	}
	if got := GetContextForAlias("alias2", mapping); got != "context2" {
		t.Errorf("expected context2, got %q", got)
	}
}

func TestGetContextForAlias_NotFound(t *testing.T) {
	mapping := map[string]string{"alias1": "context1"}
	if got := GetContextForAlias("missing", mapping); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetContextForAlias_EmptyMap(t *testing.T) {
	if got := GetContextForAlias("anything", map[string]string{}); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetContextForAlias_NilMap(t *testing.T) {
	if got := GetContextForAlias("anything", nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
