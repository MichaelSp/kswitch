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

// Package gotree provides a minimal ASCII tree printer.
//
// It is a drop-in replacement for the small subset of the dormant
// github.com/disiqueira/gotree API used by kswitch (New, Add, AddTree, Print).
// The output format is byte-for-byte compatible with disiqueira/gotree v1.0.0
// so existing tests and user-facing displays remain unchanged.
package gotree

import "strings"

const (
	newLine      = "\n"
	emptySpace   = "    "
	middleItem   = "├── "
	continueItem = "│   "
	lastItem     = "└── "
)

// Tree is the interface for a printable tree node.
type Tree interface {
	Add(text string) Tree
	AddTree(tree Tree)
	Items() []Tree
	Text() string
	Print() string
}

type tree struct {
	text  string
	items []Tree
}

// New creates a new tree node with the given text.
func New(text string) Tree {
	return &tree{text: text, items: []Tree{}}
}

func (t *tree) Add(text string) Tree {
	n := New(text)
	t.items = append(t.items, n)
	return n
}

func (t *tree) AddTree(child Tree) {
	t.items = append(t.items, child)
}

func (t *tree) Text() string  { return t.text }
func (t *tree) Items() []Tree { return t.items }

func (t *tree) Print() string {
	return t.text + newLine + printItems(t.items, []bool{})
}

func printText(text string, spaces []bool) string {
	var result strings.Builder
	last := true
	for _, space := range spaces {
		if space {
			result.WriteString(emptySpace)
		} else {
			result.WriteString(continueItem)
		}
		last = space
	}
	indicator := middleItem
	if last {
		indicator = lastItem
	}
	return result.String() + indicator + text + newLine
}

func printItems(items []Tree, spaces []bool) string {
	var result strings.Builder
	for i, f := range items {
		last := i == len(items)-1
		result.WriteString(printText(f.Text(), spaces))
		if len(f.Items()) > 0 {
			spacesChild := append(spaces, last)
			result.WriteString(printItems(f.Items(), spacesChild))
		}
	}
	return result.String()
}
