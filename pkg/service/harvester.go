package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func (h *Handler) getResourceQuotaFromHarvester(kubeConfig []byte, namespace string) (hardQuota HarvesterResourceQuota, usedQuota HarvesterResourceQuota, err error) {
	if namespace == "" {
		err = fmt.Errorf("(getResourceQuotaFromHarvester) error namespace should not be empty")
		return
	}

	config, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return
	}

	rqList, err := clientset.CoreV1().ResourceQuotas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, rq := range rqList.Items {
		if strings.Contains(rq.Name, "default-") {
			hardCPULimit := rq.Status.Hard[corev1.ResourceLimitsCPU]
			hardQuota.CPULimits = hardCPULimit.MilliValue()

			hardMemoryLimit := rq.Status.Hard[corev1.ResourceLimitsMemory]
			hardQuota.MemoryLimits = hardMemoryLimit.Value()

			hardStorageLimit := rq.Status.Hard[corev1.ResourceRequestsStorage]
			hardQuota.StorageRequests = hardStorageLimit.Value()

			usedCPULimit := rq.Status.Used[corev1.ResourceLimitsCPU]
			usedQuota.CPULimits = usedCPULimit.MilliValue()

			usedMemoryLimit := rq.Status.Used[corev1.ResourceLimitsMemory]
			usedQuota.MemoryLimits = usedMemoryLimit.Value()

			usedStorageLimit := rq.Status.Used[corev1.ResourceRequestsStorage]
			usedQuota.StorageRequests = usedStorageLimit.Value()
		}
	}

	return
}

func (h *Handler) getHarvesterKubeConfig(clusterId string) (kubeConfig []byte, err error) {
	harvesterKubeConfigSecret, err := h.clientset.CoreV1().Secrets("fleet-default").Get(context.TODO(), fmt.Sprintf("%s-kubeconfig", clusterId), metav1.GetOptions{})
	if err != nil {
		return kubeConfig, fmt.Errorf("error while fetching the Harvester kubeconfig secret contents: %s", err.Error())
	}

	kubeConfig = harvesterKubeConfigSecret.Data["value"]

	return
}

func (h *Handler) getHarvesterClusterId(secretNamespace string, secretName string) (clusterId string, err error) {
	cloudCredentialSecret, err := h.clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return clusterId, fmt.Errorf("error while fetching the cloud credential secret contents: %s", err.Error())
	}

	clusterId = string(cloudCredentialSecret.Data["harvestercredentialConfig-clusterId"])

	return
}

func (h *Handler) getHarvesterClusterCloudCredential(clusterName string) (cloudCredentialSecretName string, err error) {
	cluster, err := h.clientset.RESTClient().Get().AbsPath("/apis/provisioning.cattle.io/v1").Namespace("fleet-default").Resource("clusters").Name(clusterName).DoRaw(context.TODO())
	if err != nil {
		return cloudCredentialSecretName, fmt.Errorf("error while fetching cluster objects: %s", err.Error())
	}

	c := ClusterStruct{}
	if err = json.Unmarshal(cluster, &c); err != nil {
		return cloudCredentialSecretName, fmt.Errorf("error while unmarshall json: %s", err.Error())
	}

	// check if the cloudCredentialSecretName exists
	if c.Spec.CloudCredentialSecretName == "" {
		return cloudCredentialSecretName, fmt.Errorf("error cluster object has no cloudCredentialSecretName in the spec")
	}

	return c.Spec.CloudCredentialSecretName, err
}

func (h *Handler) getClusterNameFromHarvesterConfigName(logRef string, harvesterConfigName string) (clusterName string, err error) {
	clusters, err := h.clientset.RESTClient().Get().AbsPath("/apis/provisioning.cattle.io/v1").Namespace("fleet-default").Resource("clusters").DoRaw(context.TODO())
	if err != nil {
		return
	}

	c := ClustersStruct{}
	if err = json.Unmarshal(clusters, &c); err != nil {
		return "", fmt.Errorf("error unmarshall json: %s", err.Error())
	}

	firstIndex := strings.Index(harvesterConfigName, "-")
	lastIndex := strings.LastIndex(harvesterConfigName, "-")

	for _, item := range c.Items {
		log.Debugf("(getClusterNameFromHarvesterConfigName) %s checking clustername: [%s]", logRef, item.Metadata.Name)

		for i := 0; i < lastIndex; i++ {
			log.Tracef("(getClusterNameFromHarvesterConfigName) %s [i=%d] checking harvesterConfigName: [%s]", logRef, i, harvesterConfigName[firstIndex+1:lastIndex-i])

			if harvesterConfigName[firstIndex+1:lastIndex-i] == item.Metadata.Name {
				// we found a match
				return item.Metadata.Name, err
			}

			if firstIndex+1 == lastIndex-i {
				// end of string reached
				break
			}
		}
	}

	return
}

func (h *Handler) getHarvesterConfig(harvesterConfigName string) (harvesterConfig HarvesterConfig, err error) {
	harvesterConfigRaw, err := h.clientset.RESTClient().Get().AbsPath("/apis/rke-machine-config.cattle.io/v1").Namespace("fleet-default").Resource("harvesterconfigs").Name(harvesterConfigName).DoRaw(context.TODO())
	if err != nil {
		return harvesterConfig, fmt.Errorf("error while getting Harvester Config object: %s", err.Error())
	}

	if err = json.Unmarshal(harvesterConfigRaw, &harvesterConfig); err != nil {
		return harvesterConfig, fmt.Errorf("error while unmarshall json: %s", err.Error())
	}

	return
}

func (h *Handler) getHarvesterConfigPoolSizes(logRef string, config *HarvesterConfig) (poolSizes PoolResources, err error) {
	CPUcount, err := strconv.Atoi(config.CPUcount)
	if err != nil {
		return poolSizes, fmt.Errorf("error cannnot convert CPUcount to int")
	}
	poolSizes.MilliCPUs = int64(CPUcount) * 1000

	log.Debugf("(getHarvesterConfigPoolSizes) %s HarvesterConfig CPUs: %d", logRef, poolSizes.MilliCPUs)

	memorySize, err := strconv.Atoi(config.MemorySize)
	if err != nil {
		return poolSizes, fmt.Errorf("error cannnot convert MemorySize to int")
	}
	poolSizes.MemorySizeBytes = int64(memorySize) * 1024 * 1024 * 1024
	log.Debugf("(getHarvesterConfigPoolSizes) %s HarvesterConfig Memory: %d", logRef, poolSizes.MemorySizeBytes)

	diskInfo, err := UnmarshalDiskInfo([]byte(config.DiskInfo))
	if err != nil {
		return poolSizes, fmt.Errorf("error cannnot decode diskInfo")
	}

	var poolStorageSizeGB int = 0

	for id, disk := range diskInfo.Disks {
		poolStorageSizeGB = poolStorageSizeGB + disk.Size
		log.Debugf("(getHarvesterConfigPoolSizes) %s HarvesterConfig Disk %d: %d GB", logRef, id, disk.Size)
	}

	poolSizes.StorageSizeBytes = int64(poolStorageSizeGB * 1024 * 1024 * 1024)
	log.Debugf("(getHarvesterConfigPoolSizes) %s poolStorageSizeBytes: %d", logRef, poolSizes.StorageSizeBytes)

	return
}

func (h *Handler) checkPoolSizes(incPoolResources *PoolResources) (err error) {
	// This check is for the following behavior:
	// If the cpu, memory or storage values are too high, they will be a converted to a negative numer.
	if incPoolResources.MilliCPUs < 0 {
		return fmt.Errorf("too much amount of CPUs configured")
	}

	if incPoolResources.MemorySizeBytes < 0 {
		return fmt.Errorf("too much amount of Memory configured")
	}

	if incPoolResources.StorageSizeBytes < 0 {
		return fmt.Errorf("too much amount of Storage configured")
	}

	return
}

func (h *Handler) getHarvesterResourceQuota(logRef string, vmNamespace string, cloudCredentialSecretName string) (hardQuota HarvesterResourceQuota, usedQuota HarvesterResourceQuota, err error) {
	cloudCredentialSecretNameSplitted := strings.Split(cloudCredentialSecretName, ":")

	// get the cloud credential secret name by splitting the secret object <namespace>:<secret>
	if len(cloudCredentialSecretNameSplitted) < 1 {
		return hardQuota, usedQuota, fmt.Errorf("error cloudCredentialSecretName format is not correct")
	}

	cloudCredentialNamespace := cloudCredentialSecretNameSplitted[0]
	cloudCredentialName := cloudCredentialSecretNameSplitted[1]

	log.Debugf("(getHarvesterResourceQuota) %s cloudCredential [%s/%s]", logRef, cloudCredentialNamespace, cloudCredentialName)

	clusterId, err := h.getHarvesterClusterId(cloudCredentialNamespace, cloudCredentialName)
	if err != nil {
		return hardQuota, usedQuota, fmt.Errorf("error while gathering Harvester cluster id: %s", err.Error())
	}

	log.Debugf("(getHarvesterResourceQuota) %s Harvester clusterId found: %s", logRef, clusterId)

	kubeConfig, err := h.getHarvesterKubeConfig(clusterId)
	if err != nil {
		return hardQuota, usedQuota, fmt.Errorf("error while gathering Harvester kubeconfig: %s", err.Error())
	}

	return h.getResourceQuotaFromHarvester(kubeConfig, vmNamespace)
}

func (h *Handler) validateHarvesterQuota(logRef string, ar *admissionv1.AdmissionReview, hardQuota *HarvesterResourceQuota, usedQuota *HarvesterResourceQuota, incPoolResources *PoolResources) *admissionv1.AdmissionResponse {
	// calculate the new used resources
	newUsedCPUcount := usedQuota.CPULimits + incPoolResources.MilliCPUs
	log.Debugf("(validateHarvesterQuota) %s comparing newUsedCPUcount [%d] with hardQuota.CPULimits [%d]", logRef, newUsedCPUcount, hardQuota.CPULimits)
	if hardQuota.CPULimits > 0 && newUsedCPUcount > hardQuota.CPULimits {
		log.Warnf("(validateHarvesterQuota) %s amount of cluster CPUs [%d] exceeded the Hard Quota Limits [%d]", logRef, newUsedCPUcount, hardQuota.CPULimits)

		if h.operateMode == DENY {
			webhookMessage := fmt.Sprintf("Amount of cluster CPUs [%d] exceeded the Hard Quota Limits [%d]", newUsedCPUcount/1000, hardQuota.CPULimits/1000)
			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: false,
				Result:  &metav1.Status{Message: webhookMessage},
			}
		}
	}

	newUsedMemorySize := usedQuota.MemoryLimits + incPoolResources.MemorySizeBytes
	log.Debugf("(validateHarvesterQuota) %s comparing newUsedMemorySize [%d] with hardQuota.MemoryLimits [%d]", logRef, newUsedMemorySize, hardQuota.MemoryLimits)
	if hardQuota.MemoryLimits > 0 && newUsedMemorySize > hardQuota.MemoryLimits {
		log.Warnf("(validateHarvesterQuota) %s amount of cluster Memory [%d] exceeded the Hard Quota Limits [%d]", logRef, newUsedMemorySize, hardQuota.MemoryLimits)

		if h.operateMode == DENY {
			webhookMessage := fmt.Sprintf("Amount of cluster Memory [%d MiB] exceeded the Hard Quota Limits [%d MiB]", newUsedMemorySize/1024/1024, hardQuota.MemoryLimits/1024/1024)
			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: false,
				Result:  &metav1.Status{Message: webhookMessage},
			}
		}
	}

	newUsedStorageSize := usedQuota.StorageRequests + incPoolResources.StorageSizeBytes
	log.Debugf("(validateHarvesterQuota) %s comparing newUsedStorageSize [%d] with hardQuota.StorageRequests [%d]", logRef, newUsedStorageSize, hardQuota.StorageRequests)
	if hardQuota.StorageRequests > 0 && newUsedStorageSize > hardQuota.StorageRequests {
		log.Warnf("(validateHarvesterQuota) %s amount of cluster Storage size [%d] exceeded the Hard Quota Limits [%d]", logRef, newUsedStorageSize, hardQuota.StorageRequests)

		if h.operateMode == DENY {
			webhookMessage := fmt.Sprintf("Amount of cluster Storage size [%d MiB] exceeded the Hard Quota Limits [%d MiB]", newUsedStorageSize/1024/1024, hardQuota.StorageRequests/1024/1024)
			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: false,
				Result:  &metav1.Status{Message: webhookMessage},
			}
		}
	}

	log.Debugf("(validateHarvesterQuota) %s validation ended, all ok", logRef)

	return &admissionv1.AdmissionResponse{
		UID:     ar.Request.UID,
		Allowed: true,
	}
}
