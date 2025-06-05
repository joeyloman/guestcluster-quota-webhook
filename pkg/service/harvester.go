package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func (h *Handler) checkMachinePools(logRef string, machinePoolname string, machinePools2Check []provisioningv1.RKEMachinePool) provisioningv1.RKEMachinePool {
	for _, mpool := range machinePools2Check {
		if mpool.Name == machinePoolname {
			return mpool
		}
	}

	// pool not found, set quantity to 0 so it can be used in the calculations
	emptyPool := provisioningv1.RKEMachinePool{}
	quantity := int32(0)
	emptyPool.Quantity = &quantity
	return emptyPool
}

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rqList, err := clientset.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	harvesterKubeConfigSecret, err := h.clientset.CoreV1().Secrets("fleet-default").Get(ctx, fmt.Sprintf("%s-kubeconfig", clusterId), metav1.GetOptions{})
	if err != nil {
		return kubeConfig, fmt.Errorf("error while fetching the Harvester kubeconfig secret contents: %s", err.Error())
	}

	kubeConfig = harvesterKubeConfigSecret.Data["value"]

	return
}

func (h *Handler) getHarvesterClusterId(secretNamespace string, secretName string) (clusterId string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cloudCredentialSecret, err := h.clientset.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return clusterId, fmt.Errorf("error while fetching the cloud credential secret contents: %s", err.Error())
	}

	clusterId = string(cloudCredentialSecret.Data["harvestercredentialConfig-clusterId"])

	return
}

func (h *Handler) getHarvesterClusterCloudCredential(clusterName string) (cloudCredentialSecretName string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use dynamic client to get the cluster custom resource
	// dynClient := h.dynamicClient // You must add dynamicClient to Handler struct and initialize it elsewhere
	gvr := schema.GroupVersionResource{
		Group:    "provisioning.cattle.io",
		Version:  "v1",
		Resource: "clusters",
	}
	unstructuredObj, err := h.dynamicClient.Resource(gvr).Namespace("fleet-default").Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return cloudCredentialSecretName, fmt.Errorf("error while fetching cluster objects: %s", err.Error())
	}

	// Marshal and unmarshal to your struct
	clusterRaw, err := json.Marshal(unstructuredObj.Object)
	if err != nil {
		return cloudCredentialSecretName, fmt.Errorf("error while marshalling cluster object: %s", err.Error())
	}

	c := ClusterStruct{}
	if err = json.Unmarshal(clusterRaw, &c); err != nil {
		return cloudCredentialSecretName, fmt.Errorf("error while unmarshall json: %s", err.Error())
	}

	// check if the cloudCredentialSecretName exists
	if c.Spec.CloudCredentialSecretName == "" {
		return cloudCredentialSecretName, fmt.Errorf("error cluster object has no cloudCredentialSecretName in the spec")
	}

	return c.Spec.CloudCredentialSecretName, err
}

func (h *Handler) getClusterNameFromHarvesterConfigName(logRef string, harvesterConfigName string) (clusterName string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    "provisioning.cattle.io",
		Version:  "v1",
		Resource: "clusters",
	}
	unstructuredList, err := h.dynamicClient.Resource(gvr).Namespace("fleet-default").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	clustersRaw, err := json.Marshal(unstructuredList.Items)
	if err != nil {
		return "", fmt.Errorf("error marshalling cluster list: %s", err.Error())
	}

	c := ClustersStruct{}
	if err = json.Unmarshal(clustersRaw, &c); err != nil {
		return "", fmt.Errorf("error unmarshall json: %s", err.Error())
	}

	// Use regex to extract cluster name from harvester config name
	// Pattern: nc-<clustername>-<poolname>
	re := regexp.MustCompile(`^nc-(.+)-[^-]+$`)
	matches := re.FindStringSubmatch(harvesterConfigName)
	if len(matches) < 2 {
		return "", nil
	}

	extractedName := matches[1]
	log.Debugf("(getClusterNameFromHarvesterConfigName) %s extracted cluster name candidate: [%s]", logRef, extractedName)

	// Verify the extracted name exists in the cluster list
	for _, item := range c.Items {
		if item.Metadata.Name == extractedName {
			log.Debugf("(getClusterNameFromHarvesterConfigName) %s found matching cluster: [%s]", logRef, item.Metadata.Name)
			return item.Metadata.Name, nil
		}
	}

	return "", nil
}

func (h *Handler) getHarvesterConfig(harvesterConfigName string) (harvesterConfig HarvesterConfig, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    "rke-machine-config.cattle.io",
		Version:  "v1",
		Resource: "harvesterconfigs",
	}
	unstructuredObj, err := h.dynamicClient.Resource(gvr).Namespace("fleet-default").Get(ctx, harvesterConfigName, metav1.GetOptions{})
	if err != nil {
		return harvesterConfig, fmt.Errorf("error while getting Harvester Config object: %s", err.Error())
	}

	harvesterConfigRaw, err := json.Marshal(unstructuredObj.Object)
	if err != nil {
		return harvesterConfig, fmt.Errorf("error while marshalling Harvester Config object: %s", err.Error())
	}

	if err = json.Unmarshal(harvesterConfigRaw, &harvesterConfig); err != nil {
		return harvesterConfig, fmt.Errorf("error while unmarshall json: %s", err.Error())
	}

	return
}

func (h *Handler) getHarvesterConfigPoolSizes(logRef string, config *HarvesterConfig) (poolSizes PoolResources, err error) {
	CPUcount, err := strconv.Atoi(config.CPUcount)
	if err != nil {
		return poolSizes, fmt.Errorf("CPUcount is invalid")
	}
	poolSizes.MilliCPUs = int64(CPUcount) * 1000

	log.Debugf("(getHarvesterConfigPoolSizes) %s HarvesterConfig CPUs: %d", logRef, poolSizes.MilliCPUs)

	memorySize, err := strconv.Atoi(config.MemorySize)
	if err != nil {
		return poolSizes, fmt.Errorf("MemorySize is invalid")
	}
	poolSizes.MemorySizeBytes = int64(memorySize) * 1024 * 1024 * 1024
	log.Debugf("(getHarvesterConfigPoolSizes) %s HarvesterConfig Memory: %d", logRef, poolSizes.MemorySizeBytes)

	var poolStorageSizeGB int = 0
	if config.DiskInfo != "" {
		diskInfo, err := UnmarshalDiskInfo([]byte(config.DiskInfo))
		if err != nil {
			return poolSizes, fmt.Errorf("cannnot decode diskInfo")
		}

		for id, disk := range diskInfo.Disks {
			poolStorageSizeGB = poolStorageSizeGB + disk.Size
			log.Debugf("(getHarvesterConfigPoolSizes) %s HarvesterConfig Disk %d: %d GB", logRef, id, disk.Size)
		}
	} else {
		if config.DiskSize != "" {
			poolStorageSizeGB, err = strconv.Atoi(config.DiskSize)
			if err != nil {
				return poolSizes, fmt.Errorf("DiskSize is invalid")
			}
		}
	}
	poolSizes.StorageSizeBytes = int64(poolStorageSizeGB * 1024 * 1024 * 1024)
	log.Debugf("(getHarvesterConfigPoolSizes) %s poolStorageSizeBytes: %d", logRef, poolSizes.StorageSizeBytes)

	return
}

func (h *Handler) validatePoolSizes(poolResources *PoolResources) (err error) {
	// Rancher accepts negative numbers it should prevents updates as well
	if poolResources.MilliCPUs < 900 {
		return fmt.Errorf("incorrect amount [%d] of CPUs configured", poolResources.MilliCPUs/1000)
	}

	if poolResources.MemorySizeBytes < 1000000 {
		return fmt.Errorf("incorrect amount [%d MiB] of Memory configured", poolResources.MemorySizeBytes/1024/1024)
	}

	if poolResources.StorageSizeBytes < 1000000 {
		return fmt.Errorf("incorrect amount [%d MiB] of Storage configured", poolResources.StorageSizeBytes/1024/1024)
	}

	return
}

func (h *Handler) checkPoolSizes(poolResources *PoolResources) (err error) {
	// This check is for the following behavior:
	// If the cpu, memory or storage values are too high, they will be a converted to a negative numer.
	// Or if Rancher accepts negatove numbers it should prevents updates as well
	if poolResources.MilliCPUs < 0 {
		return fmt.Errorf("incorrect amount [%d] of CPUs configured", poolResources.MilliCPUs)
	}

	if poolResources.MemorySizeBytes < 0 {
		return fmt.Errorf("incorrect amount [%d] of Memory configured", poolResources.MemorySizeBytes)
	}

	if poolResources.StorageSizeBytes < 0 {
		return fmt.Errorf("incorrect amount [%d] of Storage configured", poolResources.StorageSizeBytes)
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
