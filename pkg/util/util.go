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

package util

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	kubeconfigutil "github.com/MichaelSp/kswitch/pkg/util/kubectx_copied"
	"github.com/MichaelSp/kswitch/types"
)

// GetContextsNamesFromKubeconfig parses the kubeconfig bytes and returns the
// list of context names, applying the given prefix to each name.
func GetContextsNamesFromKubeconfig(kubeconfigBytes []byte, contextPrefix string) ([]string, error) {
	// parse into struct that does not contain the credentials
	config, err := ParseSanitizedKubeconfig(kubeconfigBytes)
	if err != nil {
		return nil, fmt.Errorf("could not parse Kubeconfig: %w", err)
	}

	return getContextNames(config, contextPrefix), nil
}

// ParseSanitizedKubeconfig parses the kubeconfig bytes into a kubeconfig struct without credentials
func ParseSanitizedKubeconfig(data []byte) (*types.KubeConfig, error) {
	config := types.KubeConfig{}

	// unmarshal in a form that does not include the credentials
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal kubeconfig: %w", err)
	}
	return &config, nil
}

// getContextNames gets all the context names from the kubeconfig file
// and sets the parent folder name to each context in the kubeconfig file.
// When the kubeconfig has a current-context set, only that context is returned —
// this avoids surfacing secondary contexts (e.g. wildcard-tls-seed-bound) that
// are generated alongside the primary endpoint but may not be reachable.
func getContextNames(config *types.KubeConfig, prefix string) []string {
	// add a trailing slash if prefix is set (for path-like formatting)
	if len(prefix) != 0 {
		prefix = fmt.Sprintf("%s/", prefix)
	}

	if config.CurrentContext != "" {
		return []string{fmt.Sprintf("%s%s", prefix, config.CurrentContext)}
	}

	var contextNames []string
	for _, context := range config.Contexts {
		contextNames = append(contextNames, fmt.Sprintf("%s%s", prefix, context.Name))
	}
	return contextNames
}

// ExpandEnv takes a string and replaces all environment variables with their values
// ~ is expanded to the user's home directory
func ExpandEnv(path string) string {
	path = strings.ReplaceAll(path, "~", "$HOME")
	return os.ExpandEnv(path)
}

// GetCurrentContext returns "current-context" value of current kubeconfig
func GetCurrentContext() (string, error) {
	kc, err := kubeconfigutil.LoadCurrentKubeconfig()
	if err != nil {
		return "", err
	}
	currCtx := kc.GetCurrentContext()
	if currCtx == "" {
		return "", fmt.Errorf("current-context is not set")
	}
	return currCtx, nil
}

func SliceFindIndex[T string | int](slice []T, search T) int {
	for k, v := range slice {
		if v == search {
			return k
		}
	}
	return -1
}

func getAdditionalArgs() []string {
	additionalArgsIndex := SliceFindIndex(os.Args, "--")
	var additionalArgs []string
	if additionalArgsIndex > 0 {
		additionalArgs = os.Args[additionalArgsIndex+1:]
	}
	return additionalArgs
}

func SplitAdditionalArgs(args *[]string) []string {
	additionalArgs := getAdditionalArgs()
	length := len(additionalArgs)
	if length > 0 {
		tmp := *args
		tmp = tmp[0 : len(*args)-length]
		*args = tmp
	}
	return additionalArgs
}
