package service

import (
	"context"
	"testing"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
)

func TestListProvisioningClusters(t *testing.T) {
	tests := []struct {
		name    string
		objects []runtime.Object
		want    int
		wantErr bool
	}{
		{
			name: "returns_clusters",
			objects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "provisioning.cattle.io/v1",
						"kind":       "Cluster",
						"metadata": map[string]interface{}{
							"name":      "test-cluster",
							"namespace": "fleet-default",
						},
						"spec": map[string]interface{}{
							"cloudCredentialSecretName": "test-credential",
							"kubernetesVersion":        "v1.24.3",
							"rkeConfig": map[string]interface{}{
								"controlPlaneConfig": map[string]interface{}{},
								"machinePools": []interface{}{},
							},
						},
						"status": map[string]interface{}{
							"clusterName": "test-cluster",
							"conditions": []interface{}{},
						},
					},
				},
			},
			want:    1,
			wantErr: false,
		},
		{
			name:    "no_clusters",
			objects: []runtime.Object{},
			want:    0,
			wantErr: false,
		},
		{
			name:    "list_error",
			objects: nil,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := provisioningv1.AddToScheme(scheme); err != nil {
				t.Fatalf("failed to add provisioningv1 to scheme: %v", err)
			}

			client := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
				{
					Group:    "provisioning.cattle.io",
					Version:  "v1",
					Resource: "clusters",
				}: "ClusterList",
			}, tt.objects...)

			handler := &Handler{
				dynamicClient: client,
				ctx:           context.Background(),
			}

			clusters, err := handler.ListProvisioningClusters()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, clusters)
				return
			}
			
			assert.NoError(t, err)
			assert.NotNil(t, clusters)
			assert.Equal(t, tt.want, len(clusters.Items))
		})
	}
}

func TestGetProvisioningClusters(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		nameArg   string
		objects   []runtime.Object
		wantErr   bool
	}{
		{
			name:      "returns_cluster",
			namespace: "fleet-default",
			nameArg:   "test-cluster",
			objects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "provisioning.cattle.io/v1",
						"kind":       "Cluster",
						"metadata": map[string]interface{}{
							"name":      "test-cluster",
							"namespace": "fleet-default",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "not_found",
			namespace: "fleet-default",
			nameArg:   "missing-cluster",
			objects:   []runtime.Object{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			provisioningv1.AddToScheme(scheme)

			client := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
				{
					Group:    "provisioning.cattle.io",
					Version:  "v1",
					Resource: "clusters",
				}: "ClusterList",
			}, tt.objects...)

			handler := &Handler{
				dynamicClient: client,
				ctx:           context.Background(),
			}

			cluster, err := handler.GetProvisioningClusters(tt.namespace, tt.nameArg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cluster)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, cluster)
			assert.Equal(t, tt.nameArg, cluster.Name)
			assert.Equal(t, tt.namespace, cluster.Namespace)
		})
	}
}

func TestGetHarvesterConfigs(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		nameArg   string
		objects   []runtime.Object
		wantErr   bool
	}{
		{
			name:      "returns_config",
			namespace: "fleet-default",
			nameArg:   "test-harvester",
			objects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "rke-machine-config.cattle.io/v1",
						"kind":       "HarvesterConfig",
						"metadata": map[string]interface{}{
							"name":      "test-harvester",
							"namespace": "fleet-default",
						},
						"spec": map[string]interface{}{
							"network": "192.168.1.0/24",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "not_found",
			namespace: "fleet-default",
			nameArg:   "missing-harvester",
			objects:   []runtime.Object{},
			wantErr:   true,
		},
		{
			name:      "invalid_json",
			namespace: "fleet-default",
			nameArg:   "bad-harvester",
			objects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "rke-machine-config.cattle.io/v1",
						"kind":       "HarvesterConfig",
						"metadata": map[string]interface{}{
							"name":      "bad-harvester",
							"namespace": "fleet-default",
						},
						"spec": make(chan int), // invalid JSON type
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()

			client := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
				{
					Group:    "rke-machine-config.cattle.io",
					Version:  "v1",
					Resource: "harvesterconfigs",
				}: "HarvesterConfigList",
			}, tt.objects...)

			handler := &Handler{
				dynamicClient: client,
				ctx:           context.Background(),
			}

			config, err := handler.GetHarvesterConfigs(tt.namespace, tt.nameArg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, config)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, config)
			assert.Equal(t, tt.nameArg, config.Name)
			assert.Equal(t, tt.namespace, config.Namespace)
		})
	}
}
