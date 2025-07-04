package config

import (
	"context"
	"fmt"

	"github.com/joeyloman/guestcluster-quota-webhook/pkg/metrics"
	"github.com/joeyloman/guestcluster-quota-webhook/pkg/util"
	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"
)

type Handler struct {
	ctx               context.Context
	kubeConfig        string
	kubeContext       string
	clientset         kubernetes.Interface
	webhookNamespace  string
	webhookName       string
	webhookSecretName string
	csrName           string
	metrics           *metrics.MetricsAllocator
}

func Register(ctx context.Context, kubeConfig string, kubeContext string, webhookName string, webhookNamespace string, metrics *metrics.MetricsAllocator) *Handler {
	return &Handler{
		ctx:              ctx,
		kubeConfig:       kubeConfig,
		kubeContext:      kubeContext,
		webhookName:      webhookName,
		webhookNamespace: webhookNamespace,
		metrics:          metrics,
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

	h.webhookSecretName = fmt.Sprintf("%s-tls", h.webhookName)
	h.csrName = fmt.Sprintf("%s.%s.svc", h.webhookName, h.webhookNamespace)
}

func (h *Handler) Run(certRenewalPeriod int64) {
	if h.checkSecret() {
		if h.checkCertExpireDate(certRenewalPeriod) {
			if err := h.renewTLSPair(); err != nil {
				log.Errorf("%s", err.Error())
				h.metrics.UpdateLogStatus("error")
			}
		}
	} else {
		if h.checkCSR() {
			if err := h.deleteCSR(); err != nil {
				log.Errorf("%s", err.Error())
				h.metrics.UpdateLogStatus("error")
			}
		}

		tlsPair, err := h.generateTLSKeyAndCert()
		if err != nil {
			log.Errorf("%s", err.Error())
			h.metrics.UpdateLogStatus("error")
		}

		if err := h.createSecret(tlsPair); err != nil {
			log.Errorf("%s", err.Error())
			h.metrics.UpdateLogStatus("error")
		}
	}

	if err := h.writeTLSDataFromSecret(); err != nil {
		log.Errorf("%s", err.Error())
		h.metrics.UpdateLogStatus("error")
	}
}
