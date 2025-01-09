package admission

import (
	"context"
	"fmt"

	"github.com/joeyloman/guestcluster-quota-webhook/pkg/util"
	log "github.com/sirupsen/logrus"
	admregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Handler struct {
	ctx                         context.Context
	kubeConfig                  string
	kubeContext                 string
	clientset                   *kubernetes.Clientset
	webhookNamespace            string
	webhookName                 string
	validatingWebhookConfigName string
}

func Register(ctx context.Context, kubeConfig string, kubeContext string, webhookName string, webhookNamespace string, validatingWebhookConfigName string) *Handler {
	return &Handler{
		ctx:                         ctx,
		kubeConfig:                  kubeConfig,
		kubeContext:                 kubeContext,
		webhookName:                 webhookName,
		webhookNamespace:            webhookNamespace,
		validatingWebhookConfigName: validatingWebhookConfigName,
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

	if err := h.AddValidatingWebhookConfiguration(); err != nil {
		log.Panicf("%s", err.Error())
	}
}

func (h *Handler) checkValidatingWebhookConfiguration() bool {
	_, err := h.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), h.validatingWebhookConfigName, metav1.GetOptions{})

	return err == nil
}

func (h *Handler) getClusterWebhook() (webhook admregv1.ValidatingWebhook, err error) {
	cert, err := h.getCaBundleFromCABundleConfigMap()
	if err != nil {
		return
	}

	webhook.Name = fmt.Sprintf("clusters.%s.%s.svc", h.webhookName, h.webhookNamespace)

	matchLabels := make(map[string]string)
	matchLabels["admission-webhook"] = "enabled"
	nameSpaceSelector := metav1.LabelSelector{}
	webhook.NamespaceSelector = &nameSpaceSelector

	var rules []admregv1.RuleWithOperations

	rule := admregv1.RuleWithOperations{}
	rule.APIGroups = []string{"provisioning.cattle.io"}
	rule.APIVersions = []string{"v1"}
	rule.Operations = []admregv1.OperationType{"CREATE", "UPDATE"}
	rule.Resources = []string{"clusters"}
	scope := admregv1.NamespacedScope
	rule.Scope = &scope
	rules = append(rules, rule)
	webhook.Rules = rules

	sideeffects := admregv1.SideEffectClassNone
	webhook.SideEffects = &sideeffects

	clientconfig := admregv1.WebhookClientConfig{}
	serviceref := admregv1.ServiceReference{}
	serviceref.Namespace = h.webhookNamespace
	serviceref.Name = h.webhookName
	path := "/validate-cluster"
	serviceref.Path = &path
	port := int32(8443)
	serviceref.Port = &port
	clientconfig.Service = &serviceref
	clientconfig.CABundle = []byte(cert)
	webhook.ClientConfig = clientconfig

	webhook.AdmissionReviewVersions = []string{"v1"}

	return
}

func (h *Handler) getHarvesterConfigWebhook() (webhook admregv1.ValidatingWebhook, err error) {
	cert, err := h.getCaBundleFromCABundleConfigMap()
	if err != nil {
		return
	}

	webhook.Name = fmt.Sprintf("%s.%s.svc", h.webhookName, h.webhookNamespace)

	matchLabels := make(map[string]string)
	matchLabels["admission-webhook"] = "enabled"
	nameSpaceSelector := metav1.LabelSelector{}
	webhook.NamespaceSelector = &nameSpaceSelector

	var rules []admregv1.RuleWithOperations

	rule := admregv1.RuleWithOperations{}
	rule.APIGroups = []string{"rke-machine-config.cattle.io"}
	rule.APIVersions = []string{"v1"}
	rule.Operations = []admregv1.OperationType{"CREATE", "UPDATE"}
	rule.Resources = []string{"harvesterconfigs"}
	scope := admregv1.NamespacedScope
	rule.Scope = &scope
	rules = append(rules, rule)
	webhook.Rules = rules

	sideeffects := admregv1.SideEffectClassNone
	webhook.SideEffects = &sideeffects

	clientconfig := admregv1.WebhookClientConfig{}
	serviceref := admregv1.ServiceReference{}
	serviceref.Namespace = h.webhookNamespace
	serviceref.Name = h.webhookName
	path := "/validate-harvesterconfig"
	serviceref.Path = &path
	port := int32(8443)
	serviceref.Port = &port
	clientconfig.Service = &serviceref
	clientconfig.CABundle = []byte(cert)
	webhook.ClientConfig = clientconfig

	webhook.AdmissionReviewVersions = []string{"v1"}

	return
}

func (h *Handler) AddValidatingWebhookConfiguration() (err error) {
	if h.checkValidatingWebhookConfiguration() {
		return
	}

	vwc := admregv1.ValidatingWebhookConfiguration{}
	vwc.ObjectMeta.Name = h.validatingWebhookConfigName

	clusterWebhook, err := h.getClusterWebhook()
	if err != nil {
		return
	}
	vwc.Webhooks = append(vwc.Webhooks, clusterWebhook)

	harvesterConfigWebhook, err := h.getHarvesterConfigWebhook()
	if err != nil {
		return
	}
	vwc.Webhooks = append(vwc.Webhooks, harvesterConfigWebhook)

	_, err = h.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), &vwc, metav1.CreateOptions{})

	return
}
