package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	LabelLogLevel = "loglevel"
	LabelResource = "resource"
	LabelName     = "name"
	LabelAction   = "action"
)

type MetricsAllocator struct {
	httpServer                         http.Server
	guestclusterquotawebhookAppLogs    *prometheus.GaugeVec
	guestclusterquotawebhookAppActions *prometheus.GaugeVec
	registry                           *prometheus.Registry
}

func Register() *MetricsAllocator {
	return NewMetricsAllocator()
}

func NewMetricsAllocator() *MetricsAllocator {
	m := &MetricsAllocator{
		guestclusterquotawebhookAppLogs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "guestclusterquotawebhook_app_logs",
				Help: "Important log entries of the application",
			},
			[]string{
				LabelLogLevel,
			},
		),
		guestclusterquotawebhookAppActions: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "guestclusterquotawebhook_app_actions",
				Help: "Amount of accepted or blocked actions per resource type",
			},
			[]string{
				LabelResource,
				LabelName,
				LabelAction,
			},
		),
	}

	m.registry = prometheus.NewRegistry()
	m.registry.MustRegister(m.guestclusterquotawebhookAppLogs)
	m.registry.MustRegister(m.guestclusterquotawebhookAppActions)

	return m
}

func (m *MetricsAllocator) UpdateLogStatus(loglevel string) {
	m.guestclusterquotawebhookAppLogs.With(prometheus.Labels{
		LabelLogLevel: loglevel,
	}).Inc()
}

func (m *MetricsAllocator) UpdateAppActions(resource string, name string, action string) {
	m.guestclusterquotawebhookAppActions.With(prometheus.Labels{
		LabelResource: resource,
		LabelName:     name,
		LabelAction:   action,
	}).Inc()
}

func (m *MetricsAllocator) Run() {
	log.Debugf("(metrics.Run) starting the Metrics service")

	var metricsPort int

	metricsPort, err := strconv.Atoi(os.Getenv("METRICS_PORT"))
	if err != nil {
		metricsPort = 8080
	}
	listenAddress := fmt.Sprintf(":%d", metricsPort)

	m.httpServer = http.Server{
		Addr:    listenAddress,
		Handler: promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry}),
	}

	log.Infof("(metrics.Run) %s", m.httpServer.ListenAndServe())
}

func (m *MetricsAllocator) Stop() {
	log.Infof("(metrics.Stop) stopping the Metrics service")
	if err := m.httpServer.Shutdown(context.Background()); err != nil {
		log.Errorf("(metrics.Stop) error while stopping the Metrics service: %s", err.Error())
	}
}
