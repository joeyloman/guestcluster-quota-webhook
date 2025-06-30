package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *Handler) getKubeVirtIpHelperConfig() (map[string]map[string]int64, error) {
	var err error

	kihIpReservationsClusterMap := make(map[string]map[string]int64)

	kihIpReservationsConfigMap, err := h.clientset.CoreV1().ConfigMaps(h.webhookNamespace).Get(context.TODO(), "kubevirt-ip-helper-ip-reservations", metav1.GetOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// return an empty map
			var empty error
			return kihIpReservationsClusterMap, empty
		} else {
			return nil, err
		}
	}

	for k, v := range kihIpReservationsConfigMap.Data {
		// <harvester_cluster_name>_<harvester_network_namespace>_<harvester_network_name>: "<ip reservation count>"
		clusterNetworkStr := strings.Split(k, "_")
		if len(clusterNetworkStr) < 1 {
			return nil, fmt.Errorf("error IP reservation cluster/network name string format is not correct")
		}

		IPcount, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("error IP reservation count is invalid")
		}

		if _, exists := kihIpReservationsClusterMap[clusterNetworkStr[0]]; !exists {
			kihIpReservationsClusterMap[clusterNetworkStr[0]] = make(map[string]int64)
		}
		kihIpReservationsClusterMap[clusterNetworkStr[0]][fmt.Sprintf("%s/%s", clusterNetworkStr[1], clusterNetworkStr[2])] = int64(IPcount)
	}

	return kihIpReservationsClusterMap, err
}
