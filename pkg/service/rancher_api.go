package service

import (
	"encoding/json"
	"fmt"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (h *Handler) ListProvisioningClusters() (*provisioningv1.ClusterList, error) {
	gvr := schema.GroupVersionResource{
		Group:    "provisioning.cattle.io",
		Version:  "v1",
		Resource: "clusters",
	}

	list, err := h.dynamicClient.Resource(gvr).Namespace("fleet-default").List(h.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error gathering cluster list: %s", err.Error())
	}

	var items provisioningv1.ClusterList
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(list.UnstructuredContent(), &items); err != nil {
		return nil, fmt.Errorf("error parsing cluster list: %s", err.Error())
	}

	return &items, nil
}

func (h *Handler) GetProvisioningClusters(namespace string, name string) (*provisioningv1.Cluster, error) {
	gvr := schema.GroupVersionResource{
		Group:    "provisioning.cattle.io",
		Version:  "v1",
		Resource: "clusters",
	}
	obj, err := h.dynamicClient.Resource(gvr).Namespace(namespace).Get(h.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while fetching cluster objects: %s", err.Error())
	}

	var item provisioningv1.Cluster
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &item); err != nil {
		return nil, fmt.Errorf("error parsing cluster data: %s", err.Error())
	}

	return &item, nil
}

func (h *Handler) GetHarvesterConfigs(namespace string, name string) (*HarvesterConfig, error) {
	gvr := schema.GroupVersionResource{
		Group:    "rke-machine-config.cattle.io",
		Version:  "v1",
		Resource: "harvesterconfigs",
	}
	obj, err := h.dynamicClient.Resource(gvr).Namespace(namespace).Get(h.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while getting Harvester Config object: %s", err.Error())
	}

	var item HarvesterConfig
	// this doesn't work at the moment <-- NEED TO BE TESTED
	// if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &item); err != nil {
	// 	return nil, fmt.Errorf("error parsing harvester config data: %s", err.Error())
	// }
	// log.Infof("(GetHarvesterConfigs) DEBUG ITEM OBJ = [%+v]", item)

	// this works for now
	harvesterConfigRaw, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("error while marshalling Harvester Config object: %s", err.Error())
	}
	if err = json.Unmarshal(harvesterConfigRaw, &item); err != nil {
		return nil, fmt.Errorf("error while unmarshall json: %s", err.Error())
	}

	return &item, nil
}
