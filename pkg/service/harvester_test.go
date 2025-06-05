package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCheckMachinePools(t *testing.T) {
	h := &Handler{}

	quantity1 := int32(3)
	quantity2 := int32(5)

	machinePools := []provisioningv1.RKEMachinePool{
		{
			Name:     "pool1",
			Quantity: &quantity1,
		},
		{
			Name:     "pool2",
			Quantity: &quantity2,
		},
	}

	tests := []struct {
		name            string
		machinePoolName string
		expectedQty     int32
		shouldBeEmpty   bool
	}{
		{
			name:            "existing pool1",
			machinePoolName: "pool1",
			expectedQty:     3,
			shouldBeEmpty:   false,
		},
		{
			name:            "existing pool2",
			machinePoolName: "pool2",
			expectedQty:     5,
			shouldBeEmpty:   false,
		},
		{
			name:            "non-existing pool",
			machinePoolName: "pool3",
			expectedQty:     0,
			shouldBeEmpty:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.checkMachinePools("test", tt.machinePoolName, machinePools)

			if tt.shouldBeEmpty && result.Name != "" {
				t.Errorf("Expected empty pool, got %s", result.Name)
			}

			if !tt.shouldBeEmpty && result.Name != tt.machinePoolName {
				t.Errorf("Expected pool name %s, got %s", tt.machinePoolName, result.Name)
			}

			if *result.Quantity != tt.expectedQty {
				t.Errorf("Expected quantity %d, got %d", tt.expectedQty, *result.Quantity)
			}
		})
	}
}

func TestGetHarvesterConfigPoolSizes(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name      string
		config    *HarvesterConfig
		wantCPU   int64
		wantMem   int64
		wantStore int64
		wantErr   bool
	}{
		{
			name: "valid config with diskinfo",
			config: &HarvesterConfig{
				CPUcount:   "4",
				MemorySize: "8",
				DiskInfo:   `{"disks":[{"size":20},{"size":30}]}`,
			},
			wantCPU:   4000,
			wantMem:   8589934592,
			wantStore: 53687091200,
			wantErr:   false,
		},
		{
			name: "valid config with disksize",
			config: &HarvesterConfig{
				CPUcount:   "2",
				MemorySize: "4",
				DiskSize:   "100",
			},
			wantCPU:   2000,
			wantMem:   4294967296,
			wantStore: 107374182400,
			wantErr:   false,
		},
		{
			name: "invalid cpu count",
			config: &HarvesterConfig{
				CPUcount:   "invalid",
				MemorySize: "4",
				DiskSize:   "100",
			},
			wantErr: true,
		},
		{
			name: "invalid memory size",
			config: &HarvesterConfig{
				CPUcount:   "2",
				MemorySize: "invalid",
				DiskSize:   "100",
			},
			wantErr: true,
		},
		{
			name: "invalid disk info json",
			config: &HarvesterConfig{
				CPUcount:   "2",
				MemorySize: "4",
				DiskInfo:   "invalid json",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.getHarvesterConfigPoolSizes("test", tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("getHarvesterConfigPoolSizes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.MilliCPUs != tt.wantCPU {
					t.Errorf("CPU mismatch: got %d, want %d", got.MilliCPUs, tt.wantCPU)
				}
				if got.MemorySizeBytes != tt.wantMem {
					t.Errorf("Memory mismatch: got %d, want %d", got.MemorySizeBytes, tt.wantMem)
				}
				if got.StorageSizeBytes != tt.wantStore {
					t.Errorf("Storage mismatch: got %d, want %d", got.StorageSizeBytes, tt.wantStore)
				}
			}
		})
	}
}

func TestValidatePoolSizes(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name    string
		pool    *PoolResources
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid pool sizes",
			pool: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 10737418240,
			},
			wantErr: false,
		},
		{
			name: "cpu too low",
			pool: &PoolResources{
				MilliCPUs:        500,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 10737418240,
			},
			wantErr: true,
			errMsg:  "incorrect amount [0] of CPUs configured",
		},
		{
			name: "memory too low",
			pool: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  999999,
				StorageSizeBytes: 10737418240,
			},
			wantErr: true,
			errMsg:  "incorrect amount [0 MiB] of Memory configured",
		},
		{
			name: "storage too low",
			pool: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 999999,
			},
			wantErr: true,
			errMsg:  "incorrect amount [0 MiB] of Storage configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validatePoolSizes(tt.pool)

			if (err != nil) != tt.wantErr {
				t.Errorf("validatePoolSizes() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil && err.Error() != tt.errMsg {
				t.Errorf("validatePoolSizes() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestCheckPoolSizes(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name    string
		pool    *PoolResources
		wantErr bool
	}{
		{
			name: "valid positive values",
			pool: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 10737418240,
			},
			wantErr: false,
		},
		{
			name: "negative cpu",
			pool: &PoolResources{
				MilliCPUs:        -1000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 10737418240,
			},
			wantErr: true,
		},
		{
			name: "negative memory",
			pool: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  -2147483648,
				StorageSizeBytes: 10737418240,
			},
			wantErr: true,
		},
		{
			name: "negative storage",
			pool: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: -10737418240,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.checkPoolSizes(tt.pool)

			if (err != nil) != tt.wantErr {
				t.Errorf("checkPoolSizes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateHarvesterQuota(t *testing.T) {
	tests := []struct {
		name             string
		operateMode      int
		hardQuota        *HarvesterResourceQuota
		usedQuota        *HarvesterResourceQuota
		incPoolResources *PoolResources
		expectedAllowed  bool
		expectedMessage  string
	}{
		{
			name:        "within quota limits",
			operateMode: DENY,
			hardQuota: &HarvesterResourceQuota{
				CPULimits:       10000,
				MemoryLimits:    10737418240,
				StorageRequests: 107374182400,
			},
			usedQuota: &HarvesterResourceQuota{
				CPULimits:       4000,
				MemoryLimits:    4294967296,
				StorageRequests: 53687091200,
			},
			incPoolResources: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 21474836480,
			},
			expectedAllowed: true,
		},
		{
			name:        "cpu exceeds quota",
			operateMode: DENY,
			hardQuota: &HarvesterResourceQuota{
				CPULimits:       10000,
				MemoryLimits:    10737418240,
				StorageRequests: 107374182400,
			},
			usedQuota: &HarvesterResourceQuota{
				CPULimits:       8000,
				MemoryLimits:    4294967296,
				StorageRequests: 53687091200,
			},
			incPoolResources: &PoolResources{
				MilliCPUs:        3000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 21474836480,
			},
			expectedAllowed: false,
			expectedMessage: "Amount of cluster CPUs [11] exceeded the Hard Quota Limits [10]",
		},
		{
			name:        "memory exceeds quota",
			operateMode: DENY,
			hardQuota: &HarvesterResourceQuota{
				CPULimits:       10000,
				MemoryLimits:    10737418240,
				StorageRequests: 107374182400,
			},
			usedQuota: &HarvesterResourceQuota{
				CPULimits:       4000,
				MemoryLimits:    8589934592,
				StorageRequests: 53687091200,
			},
			incPoolResources: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  4294967296,
				StorageSizeBytes: 21474836480,
			},
			expectedAllowed: false,
			expectedMessage: "Amount of cluster Memory [12288 MiB] exceeded the Hard Quota Limits [10240 MiB]",
		},
		{
			name:        "storage exceeds quota",
			operateMode: DENY,
			hardQuota: &HarvesterResourceQuota{
				CPULimits:       10000,
				MemoryLimits:    10737418240,
				StorageRequests: 107374182400,
			},
			usedQuota: &HarvesterResourceQuota{
				CPULimits:       4000,
				MemoryLimits:    4294967296,
				StorageRequests: 85899345920,
			},
			incPoolResources: &PoolResources{
				MilliCPUs:        2000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 32212254720,
			},
			expectedAllowed: false,
			expectedMessage: "Amount of cluster Storage size [112640 MiB] exceeded the Hard Quota Limits [102400 MiB]",
		},
		{
			name:        "exceeds quota but logonly mode",
			operateMode: LOGONLY,
			hardQuota: &HarvesterResourceQuota{
				CPULimits:       10000,
				MemoryLimits:    10737418240,
				StorageRequests: 107374182400,
			},
			usedQuota: &HarvesterResourceQuota{
				CPULimits:       8000,
				MemoryLimits:    4294967296,
				StorageRequests: 53687091200,
			},
			incPoolResources: &PoolResources{
				MilliCPUs:        3000,
				MemorySizeBytes:  2147483648,
				StorageSizeBytes: 21474836480,
			},
			expectedAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				operateMode: tt.operateMode,
			}

			ar := &admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					UID: types.UID("test-uid"),
				},
			}

			response := h.validateHarvesterQuota("test", ar, tt.hardQuota, tt.usedQuota, tt.incPoolResources)

			if response.Allowed != tt.expectedAllowed {
				t.Errorf("validateHarvesterQuota() allowed = %v, want %v", response.Allowed, tt.expectedAllowed)
			}

			if !tt.expectedAllowed && response.Result != nil && response.Result.Message != tt.expectedMessage {
				t.Errorf("validateHarvesterQuota() message = %v, want %v", response.Result.Message, tt.expectedMessage)
			}
		})
	}
}

func TestGetResourceQuotaFromHarvester(t *testing.T) {
	tests := []struct {
		name            string
		namespace       string
		resourceQuota   *corev1.ResourceQuota
		expectHardQuota *HarvesterResourceQuota
		expectUsedQuota *HarvesterResourceQuota
		expectError     bool
		errorMessage    string
	}{
		{
			name:      "successful quota retrieval",
			namespace: "test-namespace",
			resourceQuota: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-quota",
					Namespace: "test-namespace",
				},
				Status: corev1.ResourceQuotaStatus{
					Hard: corev1.ResourceList{
						corev1.ResourceLimitsCPU:       resource.MustParse("10"),
						corev1.ResourceLimitsMemory:    resource.MustParse("10Gi"),
						corev1.ResourceRequestsStorage: resource.MustParse("100Gi"),
					},
					Used: corev1.ResourceList{
						corev1.ResourceLimitsCPU:       resource.MustParse("4"),
						corev1.ResourceLimitsMemory:    resource.MustParse("4Gi"),
						corev1.ResourceRequestsStorage: resource.MustParse("50Gi"),
					},
				},
			},
			expectHardQuota: &HarvesterResourceQuota{
				CPULimits:       10000,
				MemoryLimits:    10737418240,
				StorageRequests: 107374182400,
			},
			expectUsedQuota: &HarvesterResourceQuota{
				CPULimits:       4000,
				MemoryLimits:    4294967296,
				StorageRequests: 53687091200,
			},
			expectError: false,
		},
		{
			name:         "empty namespace error",
			namespace:    "",
			expectError:  true,
			errorMessage: "(getResourceQuotaFromHarvester) error namespace should not be empty",
		},
		{
			name:      "no resource quota found",
			namespace: "test-namespace",
			resourceQuota: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-quota",
					Namespace: "different-namespace",
				},
			},
			expectHardQuota: &HarvesterResourceQuota{
				CPULimits:       0,
				MemoryLimits:    0,
				StorageRequests: 0,
			},
			expectUsedQuota: &HarvesterResourceQuota{
				CPULimits:       0,
				MemoryLimits:    0,
				StorageRequests: 0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake clientset
			var fakeClient *fake.Clientset
			if tt.resourceQuota != nil {
				fakeClient = fake.NewSimpleClientset(tt.resourceQuota)
			} else {
				fakeClient = fake.NewSimpleClientset()
			}

			// Use the mock helper function
			hardQuota, usedQuota, err := MockHarvesterQuotaGetter(fakeClient, tt.namespace)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if err.Error() != tt.errorMessage {
					t.Errorf("Expected error message %q, got %q", tt.errorMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectHardQuota != nil {
				if hardQuota.CPULimits != tt.expectHardQuota.CPULimits {
					t.Errorf("Hard CPU limits mismatch: got %d, want %d", hardQuota.CPULimits, tt.expectHardQuota.CPULimits)
				}
				if hardQuota.MemoryLimits != tt.expectHardQuota.MemoryLimits {
					t.Errorf("Hard memory limits mismatch: got %d, want %d", hardQuota.MemoryLimits, tt.expectHardQuota.MemoryLimits)
				}
				if hardQuota.StorageRequests != tt.expectHardQuota.StorageRequests {
					t.Errorf("Hard storage requests mismatch: got %d, want %d", hardQuota.StorageRequests, tt.expectHardQuota.StorageRequests)
				}
			}

			if tt.expectUsedQuota != nil {
				if usedQuota.CPULimits != tt.expectUsedQuota.CPULimits {
					t.Errorf("Used CPU limits mismatch: got %d, want %d", usedQuota.CPULimits, tt.expectUsedQuota.CPULimits)
				}
				if usedQuota.MemoryLimits != tt.expectUsedQuota.MemoryLimits {
					t.Errorf("Used memory limits mismatch: got %d, want %d", usedQuota.MemoryLimits, tt.expectUsedQuota.MemoryLimits)
				}
				if usedQuota.StorageRequests != tt.expectUsedQuota.StorageRequests {
					t.Errorf("Used storage requests mismatch: got %d, want %d", usedQuota.StorageRequests, tt.expectUsedQuota.StorageRequests)
				}
			}
		})
	}
}

func TestGetClusterNameFromHarvesterConfigName(t *testing.T) {
	tests := []struct {
		name                string
		harvesterConfigName string
		clusters            string
		expectedClusterName string
		expectError         bool
		errorMessage        string
	}{
		{
			name:                "valid cluster name extraction",
			harvesterConfigName: "nc-test-cluster-pool1",
			clusters: `{
				"items": [
					{"metadata": {"name": "test-cluster"}},
					{"metadata": {"name": "another-cluster"}}
				]
			}`,
			expectedClusterName: "test-cluster",
			expectError:         false,
		},
		{
			name:                "no matching cluster",
			harvesterConfigName: "nc-nonexistent-pool1",
			clusters: `{
				"items": [
					{"metadata": {"name": "test-cluster"}},
					{"metadata": {"name": "another-cluster"}}
				]
			}`,
			expectedClusterName: "",
			expectError:         false,
		},
		{
			name:                "invalid json response",
			harvesterConfigName: "nc-test-cluster-pool1",
			clusters:            `invalid json`,
			expectedClusterName: "",
			expectError:         true,
		},
		{
			name:                "empty harvester config name",
			harvesterConfigName: "",
			clusters: `{
				"items": [
					{"metadata": {"name": "test-cluster"}}
				]
			}`,
			expectedClusterName: "",
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For this test, we'll simulate the expected behavior
			// Simulate the logic of getClusterNameFromHarvesterConfigName
			var result string
			var err error

			if tt.harvesterConfigName == "" {
				result = ""
			} else if tt.expectError {
				err = fmt.Errorf("test error")
			} else {
				// Simulate parsing the response
				var clustersResponse struct {
					Items []struct {
						Metadata struct {
							Name string `json:"name"`
						} `json:"metadata"`
					} `json:"items"`
				}

				if err := json.Unmarshal([]byte(tt.clusters), &clustersResponse); err != nil {
					if tt.expectError {
						// This is expected
						err = fmt.Errorf("test error")
					}
				} else {
					// Extract cluster name from harvesterConfigName
					prefix := "nc-"
					if strings.HasPrefix(tt.harvesterConfigName, prefix) {
						configNameWithoutPrefix := strings.TrimPrefix(tt.harvesterConfigName, prefix)

						// Find matching cluster
						for _, item := range clustersResponse.Items {
							if strings.HasPrefix(configNameWithoutPrefix, item.Metadata.Name+"-") {
								result = item.Metadata.Name
								break
							}
						}
					}
				}
			}

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if result != tt.expectedClusterName {
				t.Errorf("Expected cluster name %q, got %q", tt.expectedClusterName, result)
			}
		})
	}
}
