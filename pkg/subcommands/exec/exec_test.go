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

package exec

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		expected string
	}{
		{
			name:     "no arguments",
			command:  nil,
			expected: "",
		},
		{
			name: "a single argument keeps its shell syntax",
			// the documented form: the whole script arrives as one argument and
			// its pipes and redirections are meant to be interpreted
			command:  []string{"kubectl get pods | grep foo"},
			expected: "kubectl get pods | grep foo",
		},
		{
			name:     "several plain arguments",
			command:  []string{"kubectl", "get", "pods"},
			expected: "'kubectl' 'get' 'pods'",
		},
		{
			name:     "an argument containing spaces stays one word",
			command:  []string{"kubectl", "-o", "jsonpath={.items[*].metadata.name}"},
			expected: "'kubectl' '-o' 'jsonpath={.items[*].metadata.name}'",
		},
		{
			name:     "an embedded single quote is escaped",
			command:  []string{"echo", "it's"},
			expected: `'echo' 'it'\''s'`,
		},
		{
			name:     "a semicolon cannot start a new command",
			command:  []string{"echo", "hi; rm -rf /"},
			expected: `'echo' 'hi; rm -rf /'`,
		},
		{
			name:     "expansions are not interpreted",
			command:  []string{"echo", "$HOME", "`id`", "$(id)"},
			expected: "'echo' '$HOME' '`id`' '$(id)'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellCommand(tt.command); got != tt.expected {
				t.Errorf("shellCommand(%q) = %q, want %q", tt.command, got, tt.expected)
			}
		})
	}
}

// TestShellCommand_RoundTripsThroughRealShell is the assertion that actually
// matters: whatever quoting we emit, a real shell has to hand the command back
// the exact argv it was given.
func TestShellCommand_RoundTripsThroughRealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell available")
	}

	args := []string{"a b", "it's", "hi; echo pwned", "$HOME", "`id`", "tab\there"}
	command := append([]string{"printf", `%s\n`}, args...)

	out, err := exec.Command("/bin/sh", "-c", shellCommand(command)).Output()
	if err != nil {
		t.Fatalf("running the generated script failed: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(got) != len(args) {
		t.Fatalf("shell produced %d words, want %d: %q", len(got), len(args), got)
	}
	for i := range args {
		if got[i] != args[i] {
			t.Errorf("argument %d came back as %q, want %q", i, got[i], args[i])
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty string", input: "", expected: "''"},
		{name: "plain word", input: "kubectl", expected: "'kubectl'"},
		{name: "single quote", input: "it's", expected: `'it'\''s'`},
		{name: "only quotes", input: "''", expected: `''\'''\'''`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellQuote(tt.input); got != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
