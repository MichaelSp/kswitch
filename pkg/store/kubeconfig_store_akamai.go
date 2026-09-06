// Copyright 2024 The Kswitch authors
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

package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/oauth2"

	"github.com/linode/linodego/v2"

	storetypes "github.com/MichaelSp/kswitch/pkg/store/types"
	"github.com/MichaelSp/kswitch/types"
)

func init() {
	Register(types.StoreKindAkamai, func(s types.KubeconfigStore, deps Dependencies) (storetypes.KubeconfigStore, error) {
		return NewAkamaiStore(s)
	})
}

var _ storetypes.KubeconfigStore = (*AkamaiStore)(nil)

func NewAkamaiStore(store types.KubeconfigStore) (*AkamaiStore, error) {
	akamaiStoreConfig, err := ParseStoreConfig[types.StoreConfigAkamai](store)
	if err != nil {
		return nil, err
	}

	return &AkamaiStore{
		BaseStore: NewBaseStore(types.StoreKindAkamai, store),
		Config:    akamaiStoreConfig,
	}, nil
}

// InitializeAkamaiStore the Akamai client
func (s *AkamaiStore) InitializeAkamaiStore() error {
	// use environment variables if token is not set
	token := s.Config.LinodeToken
	if token == "" {
		envToken, ok := os.LookupEnv("LINODE_TOKEN")
		if !ok {
			return fmt.Errorf("linode token not set")
		}
		token = envToken
	}

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
			Base: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
	}

	linodeClient, err := linodego.NewClient(oauth2Client)
	if err != nil {
		return fmt.Errorf("failed to create linode client: %w", err)
	}

	s.Client = &linodeClient

	return nil
}

func (s *AkamaiStore) GetContextPrefix(path string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}
	return fmt.Sprintf("%s/%s", s.GetKind(), path)
}

func (s *AkamaiStore) StartSearch(channel chan storetypes.SearchResult) {
	s.Logger.Debug("Akamai: start search")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.InitializeAkamaiStore(); err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          err,
		}
		return
	}

	// list linode instances
	instances, err := s.Client.ListLKEClusters(ctx, nil)
	if err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          err,
		}
		return
	}

	for _, instance := range instances {
		channel <- storetypes.SearchResult{
			KubeconfigPath: instance.Label,
			Tags: map[string]string{
				"clusterID": strconv.Itoa(instance.ID),
				"region":    instance.Region,
			},
		}
	}
}

func (s *AkamaiStore) GetKubeconfigForPath(path string, tags map[string]string) ([]byte, error) {
	s.Logger.Debugf("Akamai: get kubeconfig for path %s", path)

	// initialize client
	if err := s.InitializeAkamaiStore(); err != nil {
		return nil, err
	}

	clusterID, err := strconv.Atoi(tags["clusterID"])
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterID: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// get kubeconfig
	LKEkubeconfig, err := s.Client.GetLKEClusterKubeconfig(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	// decode base64 kubeconfig
	kubeconfig, err := base64.StdEncoding.DecodeString(LKEkubeconfig.KubeConfig)
	if err != nil {
		return nil, err
	}

	return kubeconfig, nil
}
