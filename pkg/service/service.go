package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/joeyloman/guestcluster-quota-webhook/pkg/util"
	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// WebUI and Terraform cluster deployment order:
// 1) Creates all HarvesterConfig objects
// 2) Creates Cluster object
// 3) Updates Cluster object multiple times
// 4) Updates HarvesterConfig object multiple times

type Handler struct {
	ctx         context.Context
	httpServer  *http.Server
	kubeConfig  string
	kubeContext string
	clientset   *kubernetes.Clientset
	operateMode int
}

func Register(ctx context.Context, kubeConfig string, kubeContext string, operateMode int) *Handler {
	return &Handler{
		ctx:         ctx,
		kubeConfig:  kubeConfig,
		kubeContext: kubeContext,
		operateMode: operateMode,
	}
}

func (h *Handler) Init() {
	config, err := util.GetKubeConfig(h.kubeConfig, h.kubeContext)
	if err != nil {
		log.Panicf("%s", err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panicf("%s", err.Error())
	}
	h.clientset = clientset
}

func (h *Handler) validateCluster(ar *admissionv1.AdmissionReview, oldCluster *provisioningv1.Cluster, newCluster *provisioningv1.Cluster) *admissionv1.AdmissionResponse {
	logRef := fmt.Sprintf("[c=%s]", newCluster.Name)

	log.Debugf("(validateCluster) %s starting Cluster validation", logRef)

	log.Tracef("(validateCluster) %s oldCluster [%+v]", logRef, oldCluster)
	log.Tracef("(validateCluster) %s newCluster [%+v]", logRef, newCluster)

	// do some checks if the cluster is really a Harvester Guest cluster
	if newCluster.Spec.RKEConfig == nil {
		log.Debugf("(validateCluster) %s no RKEConfig detected in cluster object, cluster is not a Harvester Guest Cluster.", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}
	if newCluster.Spec.RKEConfig.MachineSelectorConfig == nil {
		log.Debugf("(validateCluster) %s no RKEConfig.MachineSelectorConfig detected in cluster object, cluster is not a Harvester Guest Cluster.", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}
	var isHarvesterGuest bool = false
	for _, cfg := range newCluster.Spec.RKEConfig.MachineSelectorConfig {
		log.Debugf("(validateCluster) %s checking cloud-provider-name: %s", logRef, cfg.Config.Data["cloud-provider-name"])

		if cfg.Config.Data["cloud-provider-name"] == "harvester" {
			isHarvesterGuest = true
			break
		}
	}
	if !isHarvesterGuest {
		log.Debugf("(validateCluster) %s cannot find the harvester cloud provider reference in the MachineSelectorConfig, cluster is not a Harvester Guest Cluster.", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	// if the oldCluster.Name is empty then we have a new cluster and need to construct empty machinepools
	var tmpCluster provisioningv1.Cluster
	if oldCluster.Name != "" {
		log.Debugf("(validateCluster) %s existing cluster detected", logRef)

		tmpCluster = *oldCluster.DeepCopy()
	} else {
		log.Debugf("(validateCluster) %s new cluster detected", logRef)

		tmpCluster = *newCluster.DeepCopy()
		tmpMachinePools := []provisioningv1.RKEMachinePool{}
		for _, pool := range newCluster.Spec.RKEConfig.MachinePools {
			tmpPool := pool.DeepCopy()
			*tmpPool.Quantity = int32(0)
			tmpMachinePools = append(tmpMachinePools, *tmpPool)
		}
		tmpCluster.Spec.RKEConfig.MachinePools = tmpMachinePools
	}

	var vmNamespace string

	incPoolResources := PoolResources{}
	incPoolResources.MilliCPUs = 0
	incPoolResources.MemorySizeBytes = 0
	incPoolResources.StorageSizeBytes = 0

	// first detect the pool increases
	for _, newMpool := range newCluster.Spec.RKEConfig.MachinePools {
		// Rancher does not filter all inputs properly and accepts negative pool numbers
		if *newMpool.Quantity < 0 {
			log.Errorf("(validateCluster) %s pool quantity %d of pool name %s cannot be a negative number", logRef, *newMpool.Quantity, newMpool.Name)
			webhookMessage := fmt.Sprintf("Pool quantity %d of pool name %s cannot be a negative number", *newMpool.Quantity, newMpool.Name)
			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: false,
				Result:  &metav1.Status{Message: webhookMessage},
			}
		}

		harvesterConfig, err := h.getHarvesterConfig(newMpool.NodeConfig.Name)
		if err != nil {
			log.Errorf("(validateCluster) %s error while getting harvester config for newMpool: %s", logRef, err.Error())

			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: true,
			}
		}

		if vmNamespace == "" {
			vmNamespace = harvesterConfig.VMNamespace
		}

		log.Debugf("(validateCluster) %s trying to get the pool sizes for pool [%s]", logRef, newMpool.NodeConfig.Name)

		poolSizes, err := h.getHarvesterConfigPoolSizes(logRef, &harvesterConfig)
		if err != nil {
			log.Errorf("(validateCluster) %s error while getting pool sizes: %s", logRef, err.Error())

			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: true,
			}
		}

		oldMpool := h.checkMachinePools(logRef, newMpool.Name, tmpCluster.Spec.RKEConfig.MachinePools)

		log.Debugf("(validateCluster) %s comparing oldCluster quantity [%d] and newCluster quantity [%d] of existing machinepool: [%s] and NodeConfig [%s]",
			logRef, *oldMpool.Quantity, *newMpool.Quantity, newMpool.Name, newMpool.NodeConfig.Name)

		incPoolResources.MilliCPUs += poolSizes.MilliCPUs * int64(*newMpool.Quantity-*oldMpool.Quantity)
		incPoolResources.MemorySizeBytes += poolSizes.MemorySizeBytes * int64(*newMpool.Quantity-*oldMpool.Quantity)
		incPoolResources.StorageSizeBytes += poolSizes.StorageSizeBytes * int64(*newMpool.Quantity-*oldMpool.Quantity)
	}

	// then detect the pool removals
	for _, oldMpool := range tmpCluster.Spec.RKEConfig.MachinePools {
		harvesterConfig, err := h.getHarvesterConfig(oldMpool.NodeConfig.Name)
		if err != nil {
			log.Errorf("(validateCluster) %s error while getting harvester config for oldMpool: %s", logRef, err.Error())

			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: true,
			}
		}

		if vmNamespace == "" {
			vmNamespace = harvesterConfig.VMNamespace
		}

		poolSizes, err := h.getHarvesterConfigPoolSizes(logRef, &harvesterConfig)
		if err != nil {
			log.Errorf("(validateCluster) %s error while getting pool sizes: %s", logRef, err.Error())

			return &admissionv1.AdmissionResponse{
				UID:     ar.Request.UID,
				Allowed: true,
			}
		}

		newMpool := h.checkMachinePools(logRef, oldMpool.Name, newCluster.Spec.RKEConfig.MachinePools)

		log.Debugf("(validateCluster) %s comparing oldCluster quantity [%d] and newCluster quantity [%d] of existing machinepool: [%s] and NodeConfig [%s]",
			logRef, *oldMpool.Quantity, *newMpool.Quantity, oldMpool.Name, oldMpool.NodeConfig.Name)

		if newMpool.Name == "" {
			log.Debugf("(validateCluster) %s new pool [%s] not found anymore, so it's deleted", logRef, oldMpool.Name)

			incPoolResources.MilliCPUs -= poolSizes.MilliCPUs * int64(*oldMpool.Quantity)
			incPoolResources.MemorySizeBytes -= poolSizes.MemorySizeBytes * int64(*oldMpool.Quantity)
			incPoolResources.StorageSizeBytes -= poolSizes.StorageSizeBytes * int64(*oldMpool.Quantity)
		}
	}

	// TODO: can't we fix this?
	if vmNamespace == "" {
		log.Warnf("(validateCluster) %s vmNamespace is empty, so no checks can be performed", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}
	log.Debugf("(validateCluster) %s HarvesterConfig vmnamespace: %s", logRef, vmNamespace)

	log.Debugf("(validateCluster) %s total resource increase or decrease request -> totalMilliCPU=%d / totalMemorySizeBytes=%d / totalStorageSizeBytes=%d",
		logRef, incPoolResources.MilliCPUs, incPoolResources.MemorySizeBytes, incPoolResources.StorageSizeBytes)

	// check if the cloudCredentialSecretName exists
	if newCluster.Spec.CloudCredentialSecretName == "" {
		log.Errorf("(validateCluster) %s error cluster cloud credential is empty", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	hardQuota, usedQuota, err := h.getHarvesterResourceQuota(logRef, vmNamespace, newCluster.Spec.CloudCredentialSecretName)
	if err != nil {
		log.Errorf("(validateCluster) %s error while gathering Harvester resource quota: %s", logRef, err.Error())

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	return h.validateHarvesterQuota(logRef, ar, &hardQuota, &usedQuota, &incPoolResources)
}

func (h *Handler) validateClusterAdmission(w http.ResponseWriter, r *http.Request) {
	ar := &admissionv1.AdmissionReview{}
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		log.Errorf("(validateClusterAdmission) cannot decode AdmissionReview to json: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "(validateClusterAdmission) cannot decode AdmissionReview to json: %s", err)
	}

	oldCluster := &provisioningv1.Cluster{}
	if len(ar.Request.OldObject.Raw) > 0 {
		if err := json.Unmarshal(ar.Request.OldObject.Raw, &oldCluster); err != nil {
			log.Errorf("(validateClusterAdmission) cannot unmarshal json to OLD cluster object: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "(validateClusterAdmission) cannot unmarshal json to OLD cluster object: %s", err)
		}
	}

	newCluster := &provisioningv1.Cluster{}
	if len(ar.Request.Object.Raw) > 0 {
		if err := json.Unmarshal(ar.Request.Object.Raw, &newCluster); err != nil {
			log.Errorf("(validateClusterAdmission) cannot unmarshal json to NEW cluster object: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "(validateClusterAdmission) cannot unmarshal json to NEW cluster object: %s", err)
		}
	}

	ar.Response = h.validateCluster(ar, oldCluster, newCluster)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&ar)
}

// Missing use case(s):
//  1. When poolA is upped with some resources which should fit the quota and poolB is upped which exceeds the quota then:
//     a. The poolA HarvesterConfig is accepted and a new VM is created (with poweroff state)
//     b. poolB gets denied by the webhook and stops the cluster updates
//     c. Then if the cluster config is within the quota and is saved again, the VM will be terminated but will stuck, this because there is no volume created. fix is to remove the finalizer in the vm
//  2. When a NEW cluster is created through the WebUI or Terraform, the HarvesterConfigs will be created (because there is no clustername yet).
//     Then the Cluster will be created but it's declined because it doesn't fit the quota. Result is that we have HarvesterConfig orphans.
func (h *Handler) validateHarvesterConfig(ar *admissionv1.AdmissionReview, oldHarvesterConfig *HarvesterConfig, newHarvesterConfig *HarvesterConfig) *admissionv1.AdmissionResponse {
	var clusterName string

	logRef := fmt.Sprintf("[hc=%s]", newHarvesterConfig.Name)

	log.Debugf("(validateHarvesterConfig) %s starting HarvesterConfig validation", logRef)

	log.Tracef("(validateHarvesterConfig) %s oldHarvesterConfig [%+v]", logRef, oldHarvesterConfig)
	log.Tracef("(validateHarvesterConfig) %s newHarvesterConfig [%+v]", logRef, newHarvesterConfig)

	incPoolResources := PoolResources{}
	incPoolResources.MilliCPUs = 0
	incPoolResources.MemorySizeBytes = 0
	incPoolResources.StorageSizeBytes = 0

	newPoolSizes, err := h.getHarvesterConfigPoolSizes(logRef, newHarvesterConfig)
	if err != nil {
		log.Errorf("(validateHarvesterConfig) %s error while getting pool sizes: %s", logRef, err.Error())
		webhookMessage := fmt.Sprintf("Error: %s", err.Error())
		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: false,
			Result:  &metav1.Status{Message: webhookMessage},
		}
	}

	if err := h.validatePoolSizes(&newPoolSizes); err != nil {
		log.Errorf("(validateHarvesterConfig) %s error %s", logRef, err.Error())
		webhookMessage := fmt.Sprintf("Error: %s", err.Error())
		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: false,
			Result:  &metav1.Status{Message: webhookMessage},
		}
	}

	// if the oldHarvesterConfig.Name is not empty we need to do some cpu, mem and storage comparisons with the new object
	if oldHarvesterConfig.Name != "" {
		// changing the Image and SSH User also triggeres a new VM deployment
		var fullQuotaCheckNeeded bool = false
		oldHarvesterConfigDiskInfo, err := UnmarshalDiskInfo([]byte(oldHarvesterConfig.DiskInfo))
		if err != nil {
			log.Errorf("(validateHarvesterConfig) %s error while decoding the disk info of oldHarvesterConfig: %s", logRef, err.Error())
		} else {
			newHarvesterConfigDiskInfo, err := UnmarshalDiskInfo([]byte(newHarvesterConfig.DiskInfo))
			if err != nil {
				log.Errorf("(validateHarvesterConfig) %s error while decoding the disk info of newHarvesterConfig: %s", logRef, err.Error())
			} else {
				for oldId, oldDisk := range oldHarvesterConfigDiskInfo.Disks {
					for newId, newDisk := range newHarvesterConfigDiskInfo.Disks {
						if oldId == newId && oldDisk.ImageName != newDisk.ImageName {
							log.Debugf("(validateHarvesterConfig) %s ImageName changed, quota checks needed", logRef)
							fullQuotaCheckNeeded = true
						}
					}
				}
			}
		}

		if oldHarvesterConfig.SSHUser != newHarvesterConfig.SSHUser {
			log.Debugf("(validateHarvesterConfig) %s SSHUser changed, quota checks needed", logRef)
			fullQuotaCheckNeeded = true
		}

		if fullQuotaCheckNeeded {
			incPoolResources.MilliCPUs += newPoolSizes.MilliCPUs
			incPoolResources.MemorySizeBytes += newPoolSizes.MemorySizeBytes
			incPoolResources.StorageSizeBytes += newPoolSizes.StorageSizeBytes
		} else {
			oldPoolSizes, err := h.getHarvesterConfigPoolSizes(logRef, oldHarvesterConfig)
			if err != nil {
				log.Errorf("(validateHarvesterConfig) %s error while getting old pool sizes: %s", logRef, err.Error())

				return &admissionv1.AdmissionResponse{
					UID:     ar.Request.UID,
					Allowed: true,
				}
			}

			// note: this is also hit when a new pool is created because there is also an oldHarvesterConfig provided
			if oldPoolSizes.MilliCPUs == newPoolSizes.MilliCPUs &&
				oldPoolSizes.MemorySizeBytes == newPoolSizes.MemorySizeBytes &&
				oldPoolSizes.StorageSizeBytes == newPoolSizes.StorageSizeBytes {
				log.Debugf("(validateHarvesterConfig) %s old pool sizes are equal to the new pool sizes, so no need for quota checks", logRef)

				return &admissionv1.AdmissionResponse{
					UID:     ar.Request.UID,
					Allowed: true,
				}
			}

			// if the old pool size is larger then the new one we need less space we keep the incPoolResources objects on 0
			if newPoolSizes.MilliCPUs > oldPoolSizes.MilliCPUs {
				incPoolResources.MilliCPUs += newPoolSizes.MilliCPUs - oldPoolSizes.MilliCPUs
			}
			if newPoolSizes.MemorySizeBytes > oldPoolSizes.MemorySizeBytes {
				incPoolResources.MemorySizeBytes += newPoolSizes.MemorySizeBytes - oldPoolSizes.MemorySizeBytes
			}
			if newPoolSizes.StorageSizeBytes > oldPoolSizes.StorageSizeBytes {
				incPoolResources.StorageSizeBytes += newPoolSizes.StorageSizeBytes - oldPoolSizes.StorageSizeBytes
			}
		}
	} else {
		incPoolResources.MilliCPUs += newPoolSizes.MilliCPUs
		incPoolResources.MemorySizeBytes += newPoolSizes.MemorySizeBytes
		incPoolResources.StorageSizeBytes += newPoolSizes.StorageSizeBytes
	}

	log.Debugf("(validateHarvesterConfig) %s total resource increase request -> totalMilliCPU=%d / totalMemorySizeBytes=%d / totalStorageSizeBytes=%d",
		logRef, incPoolResources.MilliCPUs, incPoolResources.MemorySizeBytes, incPoolResources.StorageSizeBytes)

	if err := h.checkPoolSizes(&incPoolResources); err != nil {
		log.Errorf("(validateHarvesterConfig) %s error while checking the pool size: %s", logRef, err.Error())

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	if newHarvesterConfig.VMNamespace == "" {
		log.Warnf("(validateHarvesterConfig) %s error harvesterConfig.VMNamespace is empty, so no checks can be performed", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}
	log.Debugf("(validateHarvesterConfig) %s harvesterConfig.VMNamespace: %s", logRef, newHarvesterConfig.VMNamespace)

	// try to get the clusterName from the newHarvesterConfig OwnerReferences
	for _, ownerObj := range newHarvesterConfig.ObjectMeta.OwnerReferences {
		if ownerObj.Name != "" {
			clusterName = ownerObj.Name
			break
		}
	}
	// if the clusterName cannot be gathered from the newHarvesterConfig, try to fetch it from the oldHarvesterConfig
	if clusterName == "" && oldHarvesterConfig.Name != "" {
		for _, ownerObj := range oldHarvesterConfig.ObjectMeta.OwnerReferences {
			if ownerObj.Name != "" {
				clusterName = ownerObj.Name
				break
			}
		}
	}
	// if the clusterName cannot be gathered from the OwnerReferenced we need to extract it from the HarvesterConfig.Name
	if clusterName == "" {
		clusterName, err = h.getClusterNameFromHarvesterConfigName(logRef, newHarvesterConfig.Name)
		if err != nil {
			log.Errorf("(validateHarvesterConfig) %s error while gathering Harvester cluster name: %s", logRef, err.Error())
		}
	}
	// if the clusterName is still empty here we cannot perform any checks
	if clusterName == "" {
		log.Debugf("(validateHarvesterConfig) %s error no clusterName found in owner reference, so no checks can be performed", logRef)

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}
	log.Debugf("(validateHarvesterConfig) %s clusterName found: %s", logRef, clusterName)

	cloudCredentialSecretName, err := h.getHarvesterClusterCloudCredential(clusterName)
	if err != nil {
		log.Errorf("(validateHarvesterConfig) %s error while gathering Harvester cloud credential: %s", logRef, err.Error())

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	hardQuota, usedQuota, err := h.getHarvesterResourceQuota(logRef, newHarvesterConfig.VMNamespace, cloudCredentialSecretName)
	if err != nil {
		log.Errorf("(validateHarvesterConfig) %s error while gathering Harvester resource quota: %s", logRef, err.Error())

		return &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
		}
	}

	log.Debugf("(validateHarvesterConfig) %s hardQuota.CPULimits=%d / usedQuota.CPULimits=%d / incPoolResources.MilliCPUs=%d",
		logRef, hardQuota.CPULimits, usedQuota.CPULimits, incPoolResources.MilliCPUs)
	log.Debugf("(validateHarvesterConfig) %s hardQuota.MemoryLimits=%d / usedQuota.MemoryLimits=%d / incPoolResources.MemorySizeBytes=%d",
		logRef, hardQuota.MemoryLimits, usedQuota.MemoryLimits, incPoolResources.MemorySizeBytes)
	log.Debugf("(validateHarvesterConfig) %s hardQuota.StorageRequests=%d / usedQuota.StorageRequests=%d / incPoolResources.StorageSizeBytes=%d",
		logRef, hardQuota.StorageRequests, usedQuota.StorageRequests, incPoolResources.StorageSizeBytes)

	return h.validateHarvesterQuota(logRef, ar, &hardQuota, &usedQuota, &incPoolResources)
}

func (h *Handler) validateHarvesterConfigAdmission(w http.ResponseWriter, r *http.Request) {
	ar := &admissionv1.AdmissionReview{}
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		log.Errorf("(validateHarvesterConfigAdmission) cannot decode AdmissionReview to json: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "(validateHarvesterConfigAdmission) cannot decode AdmissionReview to json: %s", err)
	}

	oldHarvesterConfig := &HarvesterConfig{}
	if len(ar.Request.OldObject.Raw) > 0 {
		if err := json.Unmarshal(ar.Request.OldObject.Raw, &oldHarvesterConfig); err != nil {
			log.Errorf("(validateHarvesterConfigAdmission) cannot unmarshal json to OLD harvesterconfig object: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "(validateHarvesterConfigAdmission) cannot unmarshal json to OLD harvesterconfig object: %s", err)
		}
	}

	newHarvesterConfig := &HarvesterConfig{}
	if len(ar.Request.Object.Raw) > 0 {
		if err := json.Unmarshal(ar.Request.Object.Raw, &newHarvesterConfig); err != nil {
			log.Errorf("(validateHarvesterConfigAdmission) cannot unmarshal json to NEW harvesterconfig object: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "(validateHarvesterConfigAdmission) cannot unmarshal json to NEW harvesterconfig object: %s", err)
		}
	}

	ar.Response = h.validateHarvesterConfig(ar, oldHarvesterConfig, newHarvesterConfig)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&ar)
}

func (h *Handler) Run() {
	homedir := os.Getenv("HOME")
	keyPath := fmt.Sprintf("%s/tls.key", homedir)
	certPath := fmt.Sprintf("%s/tls.crt", homedir)

	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/validate-cluster", h.validateClusterAdmission)
	mux.HandleFunc("/validate-harvesterconfig", h.validateHarvesterConfigAdmission)

	h.httpServer = &http.Server{
		Addr:           ":8443",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1048576
	}

	log.Error(h.httpServer.ListenAndServeTLS(certPath, keyPath))
}

func (h *Handler) Stop() error {
	return h.httpServer.Shutdown(h.ctx)
}
