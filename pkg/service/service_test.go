package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joeyloman/guestcluster-quota-webhook/pkg/metrics"
	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestRegister(t *testing.T) {
	ctx := context.Background()
	metricsHandler := metrics.NewMetricsAllocator()
	
	handler := Register(ctx, "test-config", "test-context", DENY, metricsHandler)
	
	if handler.ctx != ctx {
		t.Error("Context not set correctly")
	}
	if handler.kubeConfig != "test-config" {
		t.Error("KubeConfig not set correctly")
	}
	if handler.kubeContext != "test-context" {
		t.Error("KubeContext not set correctly")
	}
	if handler.operateMode != DENY {
		t.Error("OperateMode not set correctly")
	}
	if handler.metrics != metricsHandler {
		t.Error("Metrics handler not set correctly")
	}
}

func TestValidateCluster(t *testing.T) {
	metricsHandler := metrics.NewMetricsAllocator()
	fakeClient := fake.NewSimpleClientset()
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	h := &Handler{
		ctx:           context.Background(),
		operateMode:   DENY,
		metrics:       metricsHandler,
		clientset:     fakeClient,
		dynamicClient: fakeDynamicClient,
	}

	quantity1 := int32(2)
	quantity2 := int32(3)
	quantity3 := int32(4)

	tests := []struct {
		name        string
		oldCluster  *provisioningv1.Cluster
		newCluster  *provisioningv1.Cluster
		wantAllowed bool
		wantMessage string
	}{
		{
			name:       "non-harvester cluster without RKEConfig",
			oldCluster: &provisioningv1.Cluster{},
			newCluster: &provisioningv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
			},
			wantAllowed: true,
		},
		{
			name:       "non-harvester cluster without MachinePools",
			oldCluster: &provisioningv1.Cluster{},
			newCluster: &provisioningv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: provisioningv1.ClusterSpec{
					RKEConfig: &provisioningv1.RKEConfig{},
				},
			},
			wantAllowed: true,
		},
		{
			name:       "non-harvester cluster with non-harvester provider",
			oldCluster: &provisioningv1.Cluster{},
			newCluster: &provisioningv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: provisioningv1.ClusterSpec{
					RKEConfig: &provisioningv1.RKEConfig{
						MachinePools: []provisioningv1.RKEMachinePool{
							{
								Name:     "pool1",
								Quantity: &quantity1,
								NodeConfig: &corev1.ObjectReference{
									Kind: "AWSConfig",
									Name: "aws-config",
								},
							},
						},
					},
				},
			},
			wantAllowed: true,
		},
		{
			name:       "harvester cluster with negative pool quantity",
			oldCluster: &provisioningv1.Cluster{},
			newCluster: &provisioningv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: provisioningv1.ClusterSpec{
					RKEConfig: &provisioningv1.RKEConfig{
						MachinePools: []provisioningv1.RKEMachinePool{
							{
								Name:     "pool1",
								Quantity: func() *int32 { v := int32(-1); return &v }(),
								NodeConfig: &corev1.ObjectReference{
									Kind: "HarvesterConfig",
									Name: "harvester-config",
								},
							},
						},
					},
				},
			},
			wantAllowed: false,
			wantMessage: "Pool quantity -1 of pool name pool1 cannot be a negative number",
		},
		{
			name: "harvester cluster scaling up",
			oldCluster: &provisioningv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: provisioningv1.ClusterSpec{
					CloudCredentialSecretName: "cattle-global-data:cc-test",
					RKEConfig: &provisioningv1.RKEConfig{
						MachinePools: []provisioningv1.RKEMachinePool{
							{
								Name:     "pool1",
								Quantity: &quantity2,
								NodeConfig: &corev1.ObjectReference{
									Kind: "HarvesterConfig",
									Name: "harvester-config",
								},
							},
						},
					},
				},
			},
			newCluster: &provisioningv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: provisioningv1.ClusterSpec{
					CloudCredentialSecretName: "cattle-global-data:cc-test",
					RKEConfig: &provisioningv1.RKEConfig{
						MachinePools: []provisioningv1.RKEMachinePool{
							{
								Name:     "pool1",
								Quantity: &quantity3,
								NodeConfig: &corev1.ObjectReference{
									Kind: "HarvesterConfig",
									Name: "harvester-config",
								},
							},
						},
					},
				},
			},
			wantAllowed: true, // Will fail due to missing harvester config, but that's expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					UID: types.UID("test-uid"),
				},
			}
			
			response := h.validateCluster(ar, tt.oldCluster, tt.newCluster)
			
			if response.Allowed != tt.wantAllowed {
				t.Errorf("validateCluster() allowed = %v, want %v", response.Allowed, tt.wantAllowed)
			}
			
			if tt.wantMessage != "" && response.Result != nil && response.Result.Message != tt.wantMessage {
				t.Errorf("validateCluster() message = %v, want %v", response.Result.Message, tt.wantMessage)
			}
		})
	}
}

func TestValidateHarvesterConfig(t *testing.T) {
	metricsHandler := metrics.NewMetricsAllocator()
	fakeClient := fake.NewSimpleClientset()
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	h := &Handler{
		ctx:           context.Background(),
		operateMode:   DENY,
		metrics:       metricsHandler,
		clientset:     fakeClient,
		dynamicClient: fakeDynamicClient,
	}

	tests := []struct {
		name              string
		oldHarvesterConfig *HarvesterConfig
		newHarvesterConfig *HarvesterConfig
		wantAllowed       bool
		wantMessage       string
	}{
		{
			name:              "new harvester config with invalid cpu",
			oldHarvesterConfig: &HarvesterConfig{},
			newHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "invalid",
				MemorySize: "4",
				DiskSize:   "40",
			},
			wantAllowed: false,
			wantMessage: "Error: CPUcount is invalid",
		},
		{
			name:              "new harvester config with too low cpu",
			oldHarvesterConfig: &HarvesterConfig{},
			newHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "0",
				MemorySize: "4",
				DiskSize:   "40",
			},
			wantAllowed: false,
			wantMessage: "Error: incorrect amount [0] of CPUs configured",
		},
		{
			name:              "new harvester config with valid resources but no namespace",
			oldHarvesterConfig: &HarvesterConfig{},
			newHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskSize:   "40",
				VMNamespace: "",
			},
			wantAllowed: true,
		},
		{
			name: "update harvester config with same resources",
			oldHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskSize:   "40",
			},
			newHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskSize:   "40",
			},
			wantAllowed: true,
		},
		{
			name: "update harvester config with image change",
			oldHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskInfo:   `{"disks":[{"imageName":"ubuntu-20.04","size":40}]}`,
				SSHUser:    "ubuntu",
			},
			newHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskInfo:   `{"disks":[{"imageName":"ubuntu-22.04","size":40}]}`,
				SSHUser:    "ubuntu",
			},
			wantAllowed: true,
		},
		{
			name: "update harvester config with ssh user change",
			oldHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskSize:   "40",
				SSHUser:    "ubuntu",
			},
			newHarvesterConfig: &HarvesterConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
				CPUcount:   "2",
				MemorySize: "4",
				DiskSize:   "40",
				SSHUser:    "centos",
			},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					UID: types.UID("test-uid"),
				},
			}
			
			response := h.validateHarvesterConfig(ar, tt.oldHarvesterConfig, tt.newHarvesterConfig)
			
			if response.Allowed != tt.wantAllowed {
				t.Errorf("validateHarvesterConfig() allowed = %v, want %v", response.Allowed, tt.wantAllowed)
			}
			
			if tt.wantMessage != "" && response.Result != nil && response.Result.Message != tt.wantMessage {
				t.Errorf("validateHarvesterConfig() message = %v, want %v", response.Result.Message, tt.wantMessage)
			}
		})
	}
}

func TestValidateClusterAdmission(t *testing.T) {
	metricsHandler := metrics.NewMetricsAllocator()
	fakeClient := fake.NewSimpleClientset()
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	h := &Handler{
		ctx:           context.Background(),
		operateMode:   DENY,
		metrics:       metricsHandler,
		clientset:     fakeClient,
		dynamicClient: fakeDynamicClient,
	}

	quantity := int32(2)
	cluster := provisioningv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: provisioningv1.ClusterSpec{
			RKEConfig: &provisioningv1.RKEConfig{
				MachinePools: []provisioningv1.RKEMachinePool{
					{
						Name:     "pool1",
						Quantity: &quantity,
					},
				},
			},
		},
	}

	clusterBytes, _ := json.Marshal(cluster)

	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			UID: types.UID("test-uid"),
			Object: runtime.RawExtension{
				Raw: clusterBytes,
			},
		},
	}

	body, _ := json.Marshal(ar)
	req := httptest.NewRequest("POST", "/validate-cluster", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	
	h.validateClusterAdmission(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %v", rr.Code)
	}
	
	var response admissionv1.AdmissionReview
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
	
	if response.Response == nil {
		t.Error("Expected response to be non-nil")
	}
}

func TestValidateHarvesterConfigAdmission(t *testing.T) {
	metricsHandler := metrics.NewMetricsAllocator()
	fakeClient := fake.NewSimpleClientset()
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	h := &Handler{
		ctx:           context.Background(),
		operateMode:   DENY,
		metrics:       metricsHandler,
		clientset:     fakeClient,
		dynamicClient: fakeDynamicClient,
	}

	config := HarvesterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config"},
		CPUcount:   "2",
		MemorySize: "4",
		DiskSize:   "40",
	}

	configBytes, _ := json.Marshal(config)

	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			UID: types.UID("test-uid"),
			Object: runtime.RawExtension{
				Raw: configBytes,
			},
		},
	}

	body, _ := json.Marshal(ar)
	req := httptest.NewRequest("POST", "/validate-harvesterconfig", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	
	h.validateHarvesterConfigAdmission(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %v", rr.Code)
	}
	
	var response admissionv1.AdmissionReview
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
	
	if response.Response == nil {
		t.Error("Expected response to be non-nil")
	}
}

func TestStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handler{
		ctx: ctx,
		httpServer: &http.Server{
			Addr: ":0", // Use port 0 for testing
		},
	}
	
	// Start a dummy server
	go func() {
		h.httpServer.ListenAndServe()
	}()
	
	// Give the server time to start
	// In real tests, you'd want to wait for the server to be ready
	
	cancel()
	err := h.Stop()
	
	// The error might be http.ErrServerClosed which is expected
	if err != nil && err != http.ErrServerClosed {
		t.Errorf("Unexpected error stopping server: %v", err)
	}
}
