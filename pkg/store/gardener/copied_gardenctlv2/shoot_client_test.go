/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors

SPDX-License-Identifier: Apache-2.0
*/

package gardenclient_test

import (
	"context"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardenclient "github.com/MichaelSp/kswitch/pkg/store/gardener/copied_gardenctlv2"
)

func TestGardenClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GardenClient Suite")
}

var _ = Describe("GetShootClientConfig address filtering", func() {
	const (
		gardenIdentity = "test-garden"
		shootNamespace = "garden-myproject"
		shootName      = "myshoot"
		caData         = "fake-ca-cert"
	)

	var (
		ctx   context.Context
		shoot gardencorev1beta1.Shoot
		caCM  corev1.ConfigMap
	)

	BeforeEach(func() {
		ctx = context.Background()
		caCM = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      shootName + ".ca-cluster",
				Namespace: shootNamespace,
			},
			Data: map[string]string{
				"ca.crt": caData,
			},
		}
		shoot = gardencorev1beta1.Shoot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      shootName,
				Namespace: shootNamespace,
			},
			Spec: gardencorev1beta1.ShootSpec{
				Kubernetes: gardencorev1beta1.Kubernetes{Version: "1.28.0"},
			},
		}
	})

	buildClient := func(objs ...runtime.Object) gardenclient.Client {
		scheme := runtime.NewScheme()
		Expect(gardencorev1beta1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
		return gardenclient.NewGardenClient(fakeClient, gardenIdentity)
	}

	getClusters := func(cfg *clientcmdapi.Config) map[string]*clientcmdapi.Cluster {
		return cfg.Clusters
	}

	getShootConfig := func(addresses []gardencorev1beta1.ShootAdvertisedAddress) *clientcmdapi.Config {
		shoot.Status.AdvertisedAddresses = addresses
		client := buildClient(&shoot, &caCM)
		cc, err := client.GetShootClientConfig(ctx, shootNamespace, shootName, shoot, caCM)
		Expect(err).NotTo(HaveOccurred())
		raw, err := cc.RawConfig()
		Expect(err).NotTo(HaveOccurred())
		return &raw
	}

	addr := func(name, url string) gardencorev1beta1.ShootAdvertisedAddress {
		return gardencorev1beta1.ShootAdvertisedAddress{Name: name, URL: url}
	}

	Describe("excluded addresses", func() {
		It("excludes internal address from the kubeconfig", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("internal", "https://api.myshoot.myproject.internal.example.com"),
			})
			clusters := getClusters(cfg)
			Expect(clusters).NotTo(HaveKey(shootNamespace + "--" + shootName + "-internal"))
			Expect(clusters).To(HaveKey(shootNamespace + "--" + shootName + "-external"))
		})

		It("excludes service-account-issuer address from the kubeconfig", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("service-account-issuer", "https://discovery.ingress.example.com/issuer"),
			})
			clusters := getClusters(cfg)
			Expect(clusters).NotTo(HaveKey(shootNamespace + "--" + shootName + "-service-account-issuer"))
			Expect(clusters).To(HaveKey(shootNamespace + "--" + shootName + "-external"))
		})

		It("excludes ingress/* addresses from the kubeconfig", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("ingress/oauth2-ingress-abc/0/0", "https://prometheus-shoot.ingress.seed.example.com"),
				addr("ingress/oauth2-ingress-def/0/0", "https://plutono-shoot.ingress.seed.example.com"),
			})
			clusters := getClusters(cfg)
			Expect(clusters).To(HaveLen(1))
			Expect(clusters).To(HaveKey(shootNamespace + "--" + shootName + "-external"))
		})
	})

	Describe("wildcard-tls-seed-bound address", func() {
		It("is included in the kubeconfig", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("wildcard-tls-seed-bound", "https://api-myproject--myshoot.ingress.seed.example.com"),
			})
			clusters := getClusters(cfg)
			Expect(clusters).To(HaveKey(shootNamespace + "--" + shootName + "-wildcard-tls-seed-bound"))
		})

		It("has no certificate-authority-data (uses system root CAs)", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("wildcard-tls-seed-bound", "https://api-myproject--myshoot.ingress.seed.example.com"),
			})
			wildcardCluster := cfg.Clusters[shootNamespace+"--"+shootName+"-wildcard-tls-seed-bound"]
			Expect(wildcardCluster).NotTo(BeNil())
			Expect(wildcardCluster.CertificateAuthorityData).To(BeEmpty(),
				"wildcard-tls-seed-bound uses a public cert; setting the shoot CA would cause x509 failures")
		})

		It("sets certificate-authority-data on external address (uses shoot CA)", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("wildcard-tls-seed-bound", "https://api-myproject--myshoot.ingress.seed.example.com"),
			})
			externalCluster := cfg.Clusters[shootNamespace+"--"+shootName+"-external"]
			Expect(externalCluster).NotTo(BeNil())
			Expect(externalCluster.CertificateAuthorityData).To(Equal([]byte(caData)))
		})
	})

	Describe("current context", func() {
		It("points to the first non-excluded address", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
				addr("wildcard-tls-seed-bound", "https://api-myproject--myshoot.ingress.seed.example.com"),
				addr("internal", "https://api.myshoot.myproject.internal.example.com"),
			})
			Expect(cfg.CurrentContext).To(Equal(shootNamespace + "--" + shootName + "-external"))
		})
	})

	Describe("fallback behaviour", func() {
		It("falls back to external when all other addresses are filtered", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("internal", "https://api.myshoot.myproject.internal.example.com"),
				addr("service-account-issuer", "https://discovery.ingress.example.com/issuer"),
				addr("external", "https://api.myshoot.myproject.shoot.example.com"),
			})
			clusters := getClusters(cfg)
			Expect(clusters).To(HaveLen(1))
			externalCluster := clusters[shootNamespace+"--"+shootName+"-external"]
			Expect(externalCluster).NotTo(BeNil())
			Expect(externalCluster.CertificateAuthorityData).To(Equal([]byte(caData)))
		})
	})

	Describe("real-world address set (canary landscape)", func() {
		It("produces a kubeconfig with only external and wildcard-tls-seed-bound clusters", func() {
			cfg := getShootConfig([]gardencorev1beta1.ShootAdvertisedAddress{
				addr("external", "https://api.myshoot.myproject.shoot.canary.k8s-hana.ondemand.com"),
				addr("wildcard-tls-seed-bound", "https://api-myproject--myshoot.ingress.eu1.gcp-ha.seed.canary.k8s.ondemand.com"),
				addr("internal", "https://api.myshoot.myproject.internal.canary.k8s.ondemand.com"),
				addr("service-account-issuer", "https://discovery.ingress.garden.canary.k8s.ondemand.com/projects/myproject/shoots/abc/issuer"),
				addr("ingress/oauth2-ingress-0-9301c1/0/0", "https://prometheus-shoot-shoot--myproject--myshoot-0.ingress.eu1.gcp-ha.seed.canary.k8s.ondemand.com"),
				addr("ingress/oauth2-ingress-c03740/0/0", "https://plutono-shoot--myproject--myshoot.ingress.eu1.gcp-ha.seed.canary.k8s.ondemand.com"),
			})
			clusters := getClusters(cfg)
			Expect(clusters).To(HaveLen(2))
			Expect(clusters).To(HaveKey(shootNamespace + "--" + shootName + "-external"))
			Expect(clusters).To(HaveKey(shootNamespace + "--" + shootName + "-wildcard-tls-seed-bound"))

			// shoot CA on external, no CA on wildcard
			Expect(clusters[shootNamespace+"--"+shootName+"-external"].CertificateAuthorityData).To(Equal([]byte(caData)))
			Expect(clusters[shootNamespace+"--"+shootName+"-wildcard-tls-seed-bound"].CertificateAuthorityData).To(BeEmpty())
		})
	})
})
