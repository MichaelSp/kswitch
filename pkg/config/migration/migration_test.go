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

package migration_test

import (
	"time"

	"github.com/MichaelSp/kswitch/pkg/config/migration"
	"github.com/MichaelSp/kswitch/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// . "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ValidateConfig", func() {
	var config types.ConfigOld

	BeforeEach(func() {
		config = types.ConfigOld{}
	})

	It("should successfully migrate an empty config", func() {
		newEmpty := types.Config{
			Kind:    "SwitchConfig",
			Version: "v1alpha1",
		}

		c := migration.ConvertConfiguration(config)
		Expect(c).To(Equal(newEmpty))
	})

	It("should successfully migrate config with multiple paths", func() {
		refreshIndexAfter := time.Second * 10
		hooks := []types.Hook{
			{
				Name: "name",
				Type: "type",
				Path: new("my-path"),
			},
		}

		new := types.Config{
			Kind:              "SwitchConfig",
			Version:           "v1alpha1",
			KubeconfigName:    new("name"),
			RefreshIndexAfter: &refreshIndexAfter,
			Hooks:             hooks,
			KubeconfigStores: []types.KubeconfigStore{
				{
					ID:    new("default"),
					Kind:  types.StoreKindFilesystem,
					Paths: []string{"path", "other-path"},
				},
				{
					ID:     new("default"),
					Kind:   types.StoreKindVault,
					Paths:  []string{"path", "other-path"},
					Config: types.StoreConfigVault{VaultAPIAddress: "vault-api"},
				},
			},
		}

		config := types.ConfigOld{
			KubeconfigName:                "name",
			KubeconfigRediscoveryInterval: &refreshIndexAfter,
			VaultAPIAddress:               "vault-api",
			Hooks:                         hooks,
			KubeconfigPaths: []types.KubeconfigPath{
				{
					Path:  "path",
					Store: types.StoreKindFilesystem,
				},
				{
					Path:  "other-path",
					Store: types.StoreKindFilesystem,
				},
				{
					Path:  "path",
					Store: types.StoreKindVault,
				},
				{
					Path:  "other-path",
					Store: types.StoreKindVault,
				},
			},
		}

		c := migration.ConvertConfiguration(config)
		Expect(c).To(Equal(new))
	})
})
