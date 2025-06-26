package service

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// operate modes
var DENY int = 1
var LOGONLY int = 2

// for mapping cluster.provisioning.cattle.io
type ClustersStruct struct {
	Items []ClusterStruct `json:"items"`
}

// for mapping cluster.provisioning.cattle.io
type ClusterStruct struct {
	Metadata MetadataStruct `json:"metadata"`
	Spec     SpecStruct     `json:"spec"`
}

// for mapping cluster.provisioning.cattle.io
type MetadataStruct struct {
	Name string `json:"name"`
}

// for mapping cluster.provisioning.cattle.io
type SpecStruct struct {
	CloudCredentialSecretName string `json:"cloudCredentialSecretName"`
}

type HarvesterResourceQuota struct {
	CPULimits       int64 `json:"cpulimits,omitempty"`       // Milli
	MemoryLimits    int64 `json:"memorylimits,omitempty"`    // Bytes
	StorageRequests int64 `json:"storagerequests,omitempty"` // Bytes
}

type PoolResources struct {
	MilliCPUs        int64 `json:"millicpus,omitempty"`        // Milli
	MemorySizeBytes  int64 `json:"memorysizebytes,omitempty"`  // Bytes
	StorageSizeBytes int64 `json:"storagesizebytes,omitempty"` // Bytes
}

type HarvesterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// KubeConfigContent string `json:"kubeconfigcontent,omitempty"`

	VMNamespace string `json:"vmnamespace,omitempty"`
	// VMAffinity  string `json:"vmaffinity,omitempty"`
	// ClusterType string `json:"clustertype,omitempty"`
	// ClusterID   string `json:"clusterid,omitempty"`

	CPUcount   string `json:"cpucount,omitempty"`
	MemorySize string `json:"memorysize,omitempty"`
	DiskSize   string `json:"disksize,omitempty"`
	// DiskBus    string `json:"diskbus,omitempty"`

	ImageName string `json:"imagename,omitempty"`

	// DiskInfo DiskInfo `json:"diskinfo,omitempty"`
	DiskInfo string `json:"diskinfo,omitempty"`

	// KeyPairName       string `json:"keypairname,omitempty"`
	// SSHPrivateKeyPath string `json:"sshprivatekeypath,omitempty"`

	SSHUser string `json:"sshuser,omitempty"`
	// SSHPassword string `json:"sshpassword,omitempty"`
	// SSHPort     string `json:"sshport,omitempty"`

	// NetworkType  string `json:"networktype,omitempty"`
	// NetworkName  string `json:"networkname,omitempty"`
	// NetworkModel string `json:"networkmodel,omitempty"`

	NetworkInfo string `json:"networkinfo,omitempty"`

	// CloudConfig string `json:"cloudconfig,omitempty"`
	// UserData    string `json:"userdata,omitempty"`
	// NetworkData string `json:"networkdata,omitempty"`

	// EnableEFI        bool `json:"enableefi,omitempty"`
	// EnableSecureBoot bool `json:"enablesecureboot,omitempty"`
	// VGPUInfo         *VGPUInfo `json:"vgpuinfo,omitempty"`
}

type DiskInfo struct {
	Disks []Disk `json:"disks,omitempty"`
}

type Disk struct {
	ImageName string `json:"imageName,omitempty"`
	// StorageClassName string `json:"storageclassname,omitempty"`

	Size int `json:"size,omitempty"`
	// BootOrder uint `json:"bootorder,omitempty"`

	// Bus  string `json:"bus,omitempty"`
	// Type string `json:"type,omitempty"`

	// HotPlugAble bool `json:"hotplugable,omitempty"`
}

type NetworkInfo struct {
	Interfaces []NetworkInterface `json:"interfaces,omitempty"`
}

type NetworkInterface struct {
	NetworkName string `json:"networkName,omitempty"`
	MacAddress  string `json:"macAddress,omitempty"`
}

func UnmarshalDiskInfo(data []byte) (DiskInfo, error) {
	var r DiskInfo
	err := json.Unmarshal(data, &r)
	return r, err
}

func UnmarshalNetworkInfo(data []byte) (NetworkInfo, error) {
	var r NetworkInfo
	err := json.Unmarshal(data, &r)
	return r, err
}
