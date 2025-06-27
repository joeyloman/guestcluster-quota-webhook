package service

import (
	"fmt"
	"strconv"
	"strings"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	kihclientset "github.com/joeyloman/kubevirt-ip-helper/pkg/generated/clientset/versioned"
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
		err = fmt.Errorf("error namespace should not be empty")
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

	rqList, err := clientset.CoreV1().ResourceQuotas(namespace).List(h.ctx, metav1.ListOptions{})
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

func (h *Handler) getHarvesterClusterId(logRef string, cloudCredentialSecretName string) (clusterId string, err error) {
	// get the cloud credential secret name by splitting the secret object <namespace>:<secret>
	cloudCredentialSecretNameSplitted := strings.Split(cloudCredentialSecretName, ":")
	if len(cloudCredentialSecretNameSplitted) < 1 {
		return "", fmt.Errorf("error cloudCredentialSecretName format is not correct")
	}
	cloudCredentialNamespace := cloudCredentialSecretNameSplitted[0]
	cloudCredentialName := cloudCredentialSecretNameSplitted[1]
	log.Debugf("(getHarvesterClusterId) %s cloudCredential [%s/%s]", logRef, cloudCredentialNamespace, cloudCredentialName)

	cloudCredentialSecret, err := h.clientset.CoreV1().Secrets(cloudCredentialNamespace).Get(h.ctx, cloudCredentialName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error while fetching the cloud credential secret contents: %s", err.Error())
	}

	clusterId = string(cloudCredentialSecret.Data["harvestercredentialConfig-clusterId"])
	log.Debugf("(getHarvesterClusterId) %s Harvester clusterId found: %s", logRef, clusterId)

	return
}

func (h *Handler) getHarvesterClusterName(logRef string, cloudCredentialSecretName string) (clusterName string, err error) {
	clusterId, err := h.getHarvesterClusterId(logRef, cloudCredentialSecretName)
	if err != nil {
		return
	}

	cluster, err := h.GetManagementClusters(clusterId)
	if err != nil {
		return
	}
	log.Debugf("(getHarvesterClusterName) %s Harvester DisplayName found: %s", logRef, cluster.Spec.DisplayName)

	// check if the Harvester Cluster Name exists
	if cluster.Spec.DisplayName == "" {
		return "", fmt.Errorf("error cluster object has no harvester clustername in the spec")
	}

	return cluster.Spec.DisplayName, err
}

func (h *Handler) getHarvesterKubeConfig(logRef string, cloudCredentialSecretName string) (kubeConfig []byte, err error) {
	clusterId, err := h.getHarvesterClusterId(logRef, cloudCredentialSecretName)
	if err != nil {
		return
	}

	harvesterKubeConfigSecret, err := h.clientset.CoreV1().Secrets("fleet-default").Get(h.ctx, fmt.Sprintf("%s-kubeconfig", clusterId), metav1.GetOptions{})
	if err != nil {
		return kubeConfig, fmt.Errorf("error while fetching the Harvester kubeconfig secret contents: %s", err.Error())
	}

	kubeConfig = harvesterKubeConfigSecret.Data["value"]

	return
}

func (h *Handler) getHarvesterClusterCloudCredential(clusterName string) (cloudCredentialSecretName string, err error) {
	cluster, err := h.GetProvisioningClusters("fleet-default", clusterName)
	if err != nil {
		return
	}

	// check if the cloudCredentialSecretName exists
	if cluster.Spec.CloudCredentialSecretName == "" {
		return cloudCredentialSecretName, fmt.Errorf("error cluster object has no cloudCredentialSecretName in the spec")
	}

	return cluster.Spec.CloudCredentialSecretName, err
}

func (h *Handler) getClusterNameFromHarvesterConfigName(logRef string, harvesterConfigName string) (clusterName string, err error) {
	list, err := h.ListProvisioningClusters()
	if err != nil {
		return
	}

	firstIndex := strings.Index(harvesterConfigName, "-")
	lastIndex := strings.LastIndex(harvesterConfigName, "-")

	for _, item := range list.Items {
		log.Debugf("(getClusterNameFromHarvesterConfigName) %s checking clustername: [%s]", logRef, item.Name)

		for i := 0; i < lastIndex; i++ {
			log.Tracef("(getClusterNameFromHarvesterConfigName) %s [i=%d] checking harvesterConfigName: [%s]", logRef, i, harvesterConfigName[firstIndex+1:lastIndex-i])

			if harvesterConfigName[firstIndex+1:lastIndex-i] == item.Name {
				// we found a match
				return item.Name, err
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
	obj, err := h.GetHarvesterConfigs("fleet-default", harvesterConfigName)
	if err != nil {
		return
	}
	harvesterConfig = *obj

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
	// If the cpu, memory or storage values are too high, they will be converted to a negative numer.
	// Or if Rancher accepts negative numbers it should prevent updates as well
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
	kubeConfig, err := h.getHarvesterKubeConfig(logRef, cloudCredentialSecretName)
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

func (h *Handler) getHarvesterNetworks(logRef string, config *HarvesterConfig) (networkInfo NetworkInfo, err error) {
	if config.NetworkInfo != "" {
		networkInfo, err = UnmarshalNetworkInfo([]byte(config.NetworkInfo))
		if err != nil {
			return networkInfo, fmt.Errorf("cannnot decode networkInfo")
		}

		for _, network := range networkInfo.Interfaces {
			log.Debugf("(getHarvesterNetworks) %s gathered HarvesterConfig networkName [%s]", logRef, network.NetworkName)
		}
	} else {
		return networkInfo, fmt.Errorf("config.NetworkInfo is empty")
	}

	return
}

func (h *Handler) getIPPoolFromHarvester(logRef string, networkInfoName string, cloudCredentialSecretName string) (available int64, err error) {
	if networkInfoName == "" {
		err = fmt.Errorf("error networkName should not be empty")
		return -1, err
	}

	// split the network name
	networkNameSplitted := strings.Split(networkInfoName, "/")

	// get the network name by splitting the object <namespace>/<name>
	if len(networkNameSplitted) < 1 {
		return -1, fmt.Errorf("error networkName format is not correct")
	}

	networkNamespace := networkNameSplitted[0]
	networkName := networkNameSplitted[1]

	log.Debugf("(getIPPoolFromHarvester) %s networkInfoName [%s/%s]", logRef, networkNamespace, networkName)

	kubeConfig, err := h.getHarvesterKubeConfig(logRef, cloudCredentialSecretName)
	if err != nil {
		return -1, fmt.Errorf("error while gathering Harvester kubeconfig: %s", err.Error())
	}

	config, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return -1, err
	}

	kihClientset, err := kihclientset.NewForConfig(config)
	if err != nil {
		return -1, err
	}

	// note: IPPool names should match the networkName otherwise this doesn't work
	ippool, err := kihClientset.KubevirtiphelperV1().IPPools().Get(h.ctx, networkName, metav1.GetOptions{})
	if err != nil {
		return -1, fmt.Errorf("cannot get IPPool %s: %s", networkName, err.Error())
	}

	// check if the network name matches the spec.networkname of the IPPool
	if networkInfoName != ippool.Spec.NetworkName {
		return -1, fmt.Errorf("networkInfoName does not match the IPPool networkname: %s", networkInfoName)
	}

	log.Debugf("(getIPPoolFromHarvester) %s found IPPool [%s] with available IPS: %d", logRef, ippool.Spec.NetworkName, ippool.Status.IPv4.Available)

	return int64(ippool.Status.IPv4.Available), err
}
