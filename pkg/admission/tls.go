package admission

import "fmt"

func getCaBundleFromConfigMapData(data map[string]string) (string, error) {
	cert, exists := data["ca.crt"]
	if !exists {
		return cert, fmt.Errorf("ca.crt not found in configmap")
	}
	return cert, nil
}

func (h *Handler) getCaBundleFromCABundleConfigMap() (cert string, err error) {
	c := h.getCABundleConfigMap()
	return getCaBundleFromConfigMapData(c.Data)
}
