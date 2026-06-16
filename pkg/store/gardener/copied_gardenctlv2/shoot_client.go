/*
SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors

SPDX-License-Identifier: Apache-2.0
*/

package gardenclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Masterminds/semver/v3"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientauthenticationv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func init() {
	utilruntime.Must(seedmanagementv1alpha1.AddToScheme(scheme.Scheme))
}

const (
	// ShootProjectSecretSuffixCACluster is a constant for a shoot project secret with suffix 'ca-cluster'.
	ShootProjectSecretSuffixCACluster = "ca-cluster"
	// DataKeyCertificateCA is the key in a secret data holding the CA certificate.
	DataKeyCertificateCA = "ca.crt"
)

// shootKubeconfigRequest is a struct which holds information about a Kubeconfig to be generated.
type shootKubeconfigRequest struct {
	// cluster holds all the cluster on which the kube-apiserver can be reached
	clusters []cluster
	// namespace is the namespace where the shoot resides
	namespace string
	// shootName is the name of the shoot
	shootName string
	// gardenClusterIdentity is the cluster identifier of the garden cluster.
	gardenClusterIdentity string
}

// cluster holds the data to describe and connect to a kubernetes cluster
type cluster struct {
	// name is the name of the shoot advertised address, usually "external", "internal" or "unmanaged"
	name string
	// apiServerHost is the host of the kube-apiserver
	apiServerHost string

	// caCert holds the ca certificate for the cluster
	//+optional
	caCert []byte
}

// execPluginConfig contains a reference to the garden and shoot cluster
type execPluginConfig struct {
	// ShootRef references the shoot cluster
	ShootRef shootRef `json:"shootRef"`
	// GardenClusterIdentity is the cluster identifier of the garden cluster.
	// See cluster-identity ConfigMap in kube-system namespace of the garden cluster
	GardenClusterIdentity string `json:"gardenClusterIdentity"`
}

// shootRef references the shoot cluster by namespace and name
type shootRef struct {
	// Namespace is the namespace of the shoot cluster
	Namespace string `json:"namespace"`
	// Name is the name of the shoot cluster
	Name string `json:"name"`
}

func (e *execPluginConfig) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (e *execPluginConfig) DeepCopyObject() runtime.Object {
	return &execPluginConfig{
		ShootRef: shootRef{
			Namespace: e.ShootRef.Namespace,
			Name:      e.ShootRef.Name,
		},
		GardenClusterIdentity: e.GardenClusterIdentity,
	}
}

// validate validates the kubeconfig request by ensuring that all required fields are set
func (k *shootKubeconfigRequest) validate() error {
	if len(k.clusters) == 0 {
		return errors.New("missing clusters")
	}

	for n, cluster := range k.clusters {
		if cluster.name == "" {
			return fmt.Errorf("no name defined for cluster[%d]", n)
		}

		if cluster.apiServerHost == "" {
			return fmt.Errorf("no api server host defined for cluster[%d]", n)
		}
	}

	if k.namespace == "" {
		return errors.New("no namespace defined for kubeconfig request")
	}

	if k.shootName == "" {
		return errors.New("no shoot name defined for kubeconfig request")
	}

	if k.gardenClusterIdentity == "" {
		return errors.New("no garden cluster identity defined for kubeconfig request")
	}

	return nil
}

// generate generates a Kubernetes kubeconfig for communicating with the kube-apiserver
// by exec'ing the gardenlogin plugin, which fetches a client certificate.
// If legacy is false, the shoot reference and garden cluster identity is passed via the cluster extensions,
// which is supported starting with kubectl version v1.20.0.
// If legacy is true, the shoot reference and garden cluster identity are passed as command line flags to the plugin
func (k *shootKubeconfigRequest) generate(legacy bool) (*clientcmdapi.Config, error) {
	var extension *execPluginConfig

	args := []string{
		"gardenlogin",
		"get-client-certificate",
	}

	if legacy {
		args = append(
			args,
			fmt.Sprintf("--name=%s", k.shootName),
			fmt.Sprintf("--namespace=%s", k.namespace),
			fmt.Sprintf("--garden-cluster-identity=%s", k.gardenClusterIdentity),
		)
	} else {
		extension = &execPluginConfig{
			ShootRef: shootRef{
				Namespace: k.namespace,
				Name:      k.shootName,
			},
			GardenClusterIdentity: k.gardenClusterIdentity,
		}
	}

	config := clientcmdapi.NewConfig()

	authInfo := clientcmdapi.NewAuthInfo()
	authInfo.Exec = &clientcmdapi.ExecConfig{
		Command:            "kubectl",
		Args:               args,
		Env:                nil,
		APIVersion:         clientauthenticationv1beta1.SchemeGroupVersion.String(),
		InstallHint:        "",
		ProvideClusterInfo: true,

		// gardenlogin kubectl auth plugin does not require stdin itself,
		// but relies on the provided garden kubeconfig which could include auth plugins that require stdin.
		// E.g. kubelogin with --grant-type=authcode-keyboard flag, which will then prompt for the code
		InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode,
	}
	authName := fmt.Sprintf("%s--%s", k.namespace, k.shootName) // TODO instead of namespace, use project? But this would require an additional call
	config.AuthInfos[authName] = authInfo

	for i, c := range k.clusters {
		name := fmt.Sprintf("%s-%s", authName, c.name)
		if i == 0 {
			config.CurrentContext = name
		}

		cluster := clientcmdapi.NewCluster()
		cluster.CertificateAuthorityData = c.caCert
		cluster.Server = fmt.Sprintf("https://%s", c.apiServerHost)

		if !legacy {
			cluster.Extensions["client.authentication.k8s.io/exec"] = extension
		}

		config.Clusters[name] = cluster

		context := clientcmdapi.NewContext()
		context.Cluster = name
		context.AuthInfo = authName
		context.Namespace = "default" // TODO leave hardcoded? Or omit?
		config.Contexts[name] = context
	}

	return config, nil
}

func (g *clientImpl) GetShootClientConfig(ctx context.Context, namespace, name string, shoot gardencorev1beta1.Shoot, memoizedCACM corev1.ConfigMap) (clientcmd.ClientConfig, error) {
	if len(g.name) == 0 {
		return nil, errors.New("garden name must not be empty")
	}

	// fetch Shoot
	if shoot.Name == "" {
		key := types.NamespacedName{Namespace: namespace, Name: name}

		if err := g.c.Get(ctx, key, &shoot); err != nil {
			return nil, err
		}
	}

	if len(shoot.Status.AdvertisedAddresses) == 0 {
		return nil, errors.New("no advertised addresses listed in the Shoot status for the Shoot Kube API server")
	}

	// fetch cluster CA from the public ConfigMap (<shoot>.ca-cluster)
	caClusterName := fmt.Sprintf("%s.%s", name, ShootProjectSecretSuffixCACluster)
	var caCert []byte
	if memoizedCACM.Name == "" {
		cm := &corev1.ConfigMap{}
		if err := g.c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: caClusterName}, cm); err == nil {
			caCert = []byte(cm.Data[DataKeyCertificateCA])
		}
	} else {
		caCert = []byte(memoizedCACM.Data[DataKeyCertificateCA])
	}

	kubeconfigRequest := shootKubeconfigRequest{
		namespace:             shoot.Namespace,
		shootName:             shoot.Name,
		gardenClusterIdentity: g.name,
	}

	// Classify advertised addresses by how their TLS certificate is verified.
	//
	// "internal" goes through the seed ingress and is signed by the seed's CA, not the
	// shoot's ca-cluster CA, so it must be excluded to avoid x509 verification failures.
	// "service-account-issuer" is not a kube-apiserver endpoint at all.
	// "ingress/*" addresses are monitoring/logging dashboards (prometheus, plutono), not API endpoints.
	//
	// "wildcard-tls-seed-bound" is a real API server endpoint but its TLS certificate is a
	// publicly-trusted wildcard cert (e.g. Let's Encrypt) issued for the seed's ingress domain.
	// Using the shoot's ca-cluster CA for it causes x509 failures because the cert is not
	// signed by that CA. Setting caCert to nil lets kubectl fall back to the system root CA
	// pool, which correctly validates the public cert.
	excludedNames := map[string]bool{
		"internal":               true,
		"service-account-issuer": true,
	}
	// publicCertNames contains addresses whose TLS cert is publicly trusted; they must not
	// have certificate-authority-data set in the kubeconfig.
	publicCertNames := map[string]bool{
		"wildcard-tls-seed-bound": true,
	}

	// When no CA cert is available (secret was inaccessible), only include the external
	// address. wildcard-tls-seed-bound uses an environment-specific wildcard cert that may
	// not be trusted by the system root CA pool, making it unreliable without the shoot CA.
	if len(caCert) == 0 {
		excludedNames["wildcard-tls-seed-bound"] = true
	}

	for _, address := range shoot.Status.AdvertisedAddresses {
		if excludedNames[address.Name] {
			continue
		}
		// "ingress/*" addresses (prometheus, plutono, …) are not API server endpoints.
		if strings.HasPrefix(address.Name, "ingress/") {
			continue
		}

		u, err := url.Parse(address.URL)
		if err != nil {
			return nil, fmt.Errorf("could not parse shoot server url: %w", err)
		}

		var clusterCACert []byte
		if !publicCertNames[address.Name] {
			clusterCACert = caCert
		}

		kubeconfigRequest.clusters = append(kubeconfigRequest.clusters, cluster{
			name:          address.Name,
			apiServerHost: u.Host,
			caCert:        clusterCACert,
		})
	}

	// Fall back to external address only if filtering removed everything.
	if len(kubeconfigRequest.clusters) == 0 {
		for _, address := range shoot.Status.AdvertisedAddresses {
			if address.Name == "external" || address.Name == "unmanaged" {
				u, err := url.Parse(address.URL)
				if err != nil {
					return nil, fmt.Errorf("could not parse shoot server url: %w", err)
				}
				kubeconfigRequest.clusters = append(kubeconfigRequest.clusters, cluster{
					name:          address.Name,
					apiServerHost: u.Host,
					caCert:        caCert,
				})
			}
		}
	}

	if err := kubeconfigRequest.validate(); err != nil {
		return nil, fmt.Errorf("validation failed for kubeconfig request: %w", err)
	}

	// parse kubernetes version to determine if a legacy kubeconfig should be created.
	constraint, err := semver.NewConstraint("< v1.20.0")
	if err != nil {
		return nil, fmt.Errorf("failed to parse constraint: %w", err)
	}

	version, err := semver.NewVersion(shoot.Spec.Kubernetes.Version)
	if err != nil {
		return nil, fmt.Errorf("could not parse kubernetes version %s of shoot cluster: %w", shoot.Spec.Kubernetes.Version, err)
	}

	legacy := constraint.Check(version)

	config, err := kubeconfigRequest.generate(legacy)
	if err != nil {
		return nil, fmt.Errorf("generation failed for kubeconfig request: %w", err)
	}

	return clientcmd.NewDefaultClientConfig(*config, nil), nil
}
