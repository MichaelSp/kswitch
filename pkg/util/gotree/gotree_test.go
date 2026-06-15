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

package gotree

import "testing"

// TestPrintMatchesDisiqueiraFormat verifies the output is byte-for-byte
// compatible with github.com/disiqueira/gotree v1.0.0, which kswitch's
// existing user-facing tree displays were built against. The format is
// preserved exactly (including original quirks) to avoid any visual
// regression for users.
func TestPrintMatchesDisiqueiraFormat(t *testing.T) {
	root := New("root")
	a := root.Add("a")
	a.Add("a1")
	a.Add("a2")
	root.Add("b")

	got := root.Print()
	// This output matches disiqueira/gotree v1.0.0 byte-for-byte.
	want := "root\n" +
		"└── a\n" +
		"│   ├── a1\n" +
		"│   ├── a2\n" +
		"└── b\n"

	if got != want {
		t.Fatalf("Print() output mismatch.\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAddTree(t *testing.T) {
	root := New("root")
	sub := New("sub")
	sub.Add("leaf")
	root.AddTree(sub)

	got := root.Print()
	// Matches disiqueira/gotree v1.0.0 output.
	want := "root\n" +
		"└── sub\n" +
		"    └── leaf\n"

	if got != want {
		t.Fatalf("AddTree output mismatch.\n got:\n%s\nwant:\n%s", got, want)
	}
}
