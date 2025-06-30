package util

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	fileExistsFunc = FileExists
	inClusterConfigFunc = rest.InClusterConfig
)

func GetKubeConfig(kubeConfig string, kubeContext string) (config *rest.Config, err error) {
	if !fileExistsFunc(kubeConfig) {
		return inClusterConfigFunc()
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{}, CurrentContext: kubeContext},
	).ClientConfig()
}
