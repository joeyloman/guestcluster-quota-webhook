package service

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// MockHarvesterQuotaGetter is a test helper that simulates getResourceQuotaFromHarvester
func MockHarvesterQuotaGetter(clientset kubernetes.Interface, namespace string) (*HarvesterResourceQuota, *HarvesterResourceQuota, error) {
	if namespace == "" {
		return nil, nil, fmt.Errorf("(getResourceQuotaFromHarvester) error namespace should not be empty")
	}

	quotas, err := clientset.CoreV1().ResourceQuotas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}

	hardQuota := &HarvesterResourceQuota{}
	usedQuota := &HarvesterResourceQuota{}

	if len(quotas.Items) > 0 {
		quota := quotas.Items[0]
		if cpuLimit, ok := quota.Status.Hard[corev1.ResourceLimitsCPU]; ok {
			hardQuota.CPULimits = cpuLimit.MilliValue()
		}
		if memLimit, ok := quota.Status.Hard[corev1.ResourceLimitsMemory]; ok {
			hardQuota.MemoryLimits = memLimit.Value()
		}
		if storageReq, ok := quota.Status.Hard[corev1.ResourceRequestsStorage]; ok {
			hardQuota.StorageRequests = storageReq.Value()
		}

		if cpuUsed, ok := quota.Status.Used[corev1.ResourceLimitsCPU]; ok {
			usedQuota.CPULimits = cpuUsed.MilliValue()
		}
		if memUsed, ok := quota.Status.Used[corev1.ResourceLimitsMemory]; ok {
			usedQuota.MemoryLimits = memUsed.Value()
		}
		if storageUsed, ok := quota.Status.Used[corev1.ResourceRequestsStorage]; ok {
			usedQuota.StorageRequests = storageUsed.Value()
		}
	}

	return hardQuota, usedQuota, nil
}
