// Copyright 2025 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package merge_to_default

import (
	"fmt"
	"os"
	"path/filepath"

	kubeconfigutil "github.com/MichaelSp/kswitch/pkg/util/kubectx_copied"
	"gopkg.in/yaml.v3"
)

// MergeResult holds information about a successful merge for display purposes.
type MergeResult struct {
	Context     string
	Destination string
}

// MergeToDefault merges the currently selected KUBECONFIG into ~/.kube/config.
// Contexts, clusters, and users with the same name are overwritten.
// current-context is set to the context from the selected KUBECONFIG.
func MergeToDefault() (*MergeResult, error) {
	src, err := kubeconfigutil.LoadCurrentKubeconfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load current KUBECONFIG: %w", err)
	}

	srcBytes, err := src.GetBytes()
	if err != nil {
		return nil, err
	}

	targetPath := ResolveWriteTarget()
	return MergeDataToTarget(srcBytes, targetPath)
}

// ResolveWriteTarget returns the real kubeconfig path to write to:
// if $KUBECONFIG points to a kswitch tmp file (or is unset), returns ~/.kube/config;
// otherwise returns $KUBECONFIG.
func ResolveWriteTarget() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" && !isKswitchTmp(kc) {
		return kc
	}
	return defaultKubeconfigPath()
}

// isKswitchTmp reports whether path is inside the kswitch temporary directory.
func isKswitchTmp(path string) bool {
	tmpDir := os.ExpandEnv(kubeconfigutil.TemporaryKubeconfigDir)
	return len(path) > len(tmpDir) && path[:len(tmpDir)] == tmpDir
}

// MergeDataToTarget merges srcData into the kubeconfig at targetPath.
// The target file is created if it does not exist.
func MergeDataToTarget(srcData []byte, targetPath string) (*MergeResult, error) {
	if err := ensureDefaultExists(targetPath); err != nil {
		return nil, err
	}

	dstBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", targetPath, err)
	}

	merged, err := mergeKubeconfigs(dstBytes, srcData)
	if err != nil {
		return nil, fmt.Errorf("failed to merge kubeconfigs: %w", err)
	}

	if err := os.WriteFile(targetPath, merged, 0600); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", targetPath, err)
	}

	var srcDoc yaml.Node
	_ = yaml.Unmarshal(srcData, &srcDoc)
	ctx := ""
	if len(srcDoc.Content) > 0 {
		ctx = scalarValue(srcDoc.Content[0], "current-context")
	}
	return &MergeResult{Context: ctx, Destination: targetPath}, nil
}

func defaultKubeconfigPath() string {
	home := os.Getenv("HOME")
	return filepath.Join(home, ".kube", "config")
}

func ensureDefaultExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		empty := []byte("apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		return os.WriteFile(path, empty, 0600)
	}
	return nil
}

// mergeKubeconfigs merges src into dst YAML bytes, returning the merged result.
// Entries in clusters/contexts/users with matching names are replaced by src's version.
// current-context is set to src's current-context.
func mergeKubeconfigs(dstBytes, srcBytes []byte) ([]byte, error) {
	var dstDoc, srcDoc yaml.Node
	if err := yaml.Unmarshal(dstBytes, &dstDoc); err != nil || len(dstDoc.Content) == 0 {
		return nil, fmt.Errorf("invalid dst kubeconfig YAML")
	}
	if err := yaml.Unmarshal(srcBytes, &srcDoc); err != nil || len(srcDoc.Content) == 0 {
		return nil, fmt.Errorf("invalid src kubeconfig YAML")
	}
	dstRoot := dstDoc.Content[0]
	srcRoot := srcDoc.Content[0]

	for _, section := range []string{"clusters", "contexts", "users"} {
		mergeSection(dstRoot, srcRoot, section)
	}

	if cc := scalarValue(srcRoot, "current-context"); cc != "" {
		setScalar(dstRoot, "current-context", cc)
	}

	return yaml.Marshal(&dstDoc)
}

// mergeSection upserts named entries from srcRoot into dstRoot for the given section.
func mergeSection(dstRoot, srcRoot *yaml.Node, section string) {
	srcSeq := nodeValue(srcRoot, section)
	if srcSeq == nil || srcSeq.Kind != yaml.SequenceNode {
		return
	}

	dstSeq := nodeValue(dstRoot, section)
	if dstSeq == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: section, Tag: "!!str"}
		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		dstRoot.Content = append(dstRoot.Content, keyNode, seqNode)
		dstSeq = seqNode
	}

	for _, srcEntry := range srcSeq.Content {
		srcName := nodeValue(srcEntry, "name")
		if srcName == nil {
			continue
		}
		replaced := false
		for i, dstEntry := range dstSeq.Content {
			if dstName := nodeValue(dstEntry, "name"); dstName != nil && dstName.Value == srcName.Value {
				dstSeq.Content[i] = srcEntry
				replaced = true
				break
			}
		}
		if !replaced {
			dstSeq.Content = append(dstSeq.Content, srcEntry)
		}
	}
}

func setScalar(mapNode *yaml.Node, key, value string) {
	for i, ch := range mapNode.Content {
		if i%2 == 0 && ch.Kind == yaml.ScalarNode && ch.Value == key {
			mapNode.Content[i+1].Value = value
			return
		}
	}
	mapNode.Content = append(mapNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value, Tag: "!!str"},
	)
}

func scalarValue(mapNode *yaml.Node, key string) string {
	n := nodeValue(mapNode, key)
	if n == nil {
		return ""
	}
	return n.Value
}

func nodeValue(mapNode *yaml.Node, key string) *yaml.Node {
	if mapNode.Kind != yaml.MappingNode {
		return nil
	}
	for i, ch := range mapNode.Content {
		if i%2 == 0 && ch.Kind == yaml.ScalarNode && ch.Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}
