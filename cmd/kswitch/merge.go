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

package kswitch

import (
	"fmt"

	merge_to_default "github.com/MichaelSp/kswitch/pkg/subcommands/merge-to-default"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var mergeToDefaultCmd = &cobra.Command{
	Use:   "merge-to-default-kubeconfig",
	Short: "Merge current KUBECONFIG into ~/.kube/config",
	Long: `Merge the currently selected KUBECONFIG file into ~/.kube/config.

WARNING: Existing contexts, clusters, and users with the same name will be
overwritten. The current-context in ~/.kube/config will be set to the
context from the selected KUBECONFIG.`,
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := merge_to_default.MergeToDefault()
		if err != nil {
			return err
		}
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		fmt.Printf("✅ Merged context %s into %s\n",
			green.Render(result.Context),
			dim.Render(result.Destination),
		)
		return nil
	},
}

func init() {
	rootCommand.AddCommand(mergeToDefaultCmd)
}
