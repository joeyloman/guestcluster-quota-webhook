package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joeyloman/guestcluster-quota-webhook/pkg/admission"
	"github.com/joeyloman/guestcluster-quota-webhook/pkg/config"
	"github.com/joeyloman/guestcluster-quota-webhook/pkg/metrics"
	"github.com/joeyloman/guestcluster-quota-webhook/pkg/scheduler"
	"github.com/joeyloman/guestcluster-quota-webhook/pkg/service"
	log "github.com/sirupsen/logrus"
)

var progname string = "guestcluster-quota-webhook"

// operate modes
var DENY int = 1
var LOGONLY int = 2

const (
	DefaultCertRenewalPeriod = 30 * 24 * 60 // 30 days in minutes
	MinCertRenewalPeriod     = 1 * 24 * 60  // 1 day in minutes
	MaxCertRenewalPeriod     = 90 * 24 * 60 // 90 days in minutes
)

var certRenewalPeriod int64

func init() {
	// Log as JSON instead of the default ASCII formatter.
	formatter := &log.TextFormatter{
		FullTimestamp: true,
	}
	log.SetFormatter(formatter)
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

func main() {
	var kubeconfig_file string
	var kubenamespace string

	level, err := log.ParseLevel(os.Getenv("LOGLEVEL"))
	if err == nil {
		log.SetLevel(level)
	}

	var operateMode int = DENY
	if os.Getenv("OPERATEMODE") == "LOGONLY" {
		operateMode = LOGONLY
		log.Infof("starting webhook in operate mode: LOGONLY")
	}

	certRenewalPeriod, err = strconv.ParseInt(os.Getenv("CERTRENEWALPERIOD"), 10, 64)
	if err != nil || certRenewalPeriod == 0 {
		certRenewalPeriod = DefaultCertRenewalPeriod
		log.Infof("Using default cert renewal period: %d minutes", certRenewalPeriod)
	} else if certRenewalPeriod < MinCertRenewalPeriod || certRenewalPeriod > MaxCertRenewalPeriod {
		log.Warnf("Cert renewal period %d is outside recommended range [%d-%d], using default", 
			certRenewalPeriod, MinCertRenewalPeriod, MaxCertRenewalPeriod)
		certRenewalPeriod = DefaultCertRenewalPeriod
	}

	kubeconfig_file = os.Getenv("KUBECONFIG")
	if kubeconfig_file == "" {
		homedir := os.Getenv("HOME")
		kubeconfig_file = filepath.Join(homedir, ".kube", "config")
	}

	kubeconfig_context := os.Getenv("KUBECONTEXT")

	kubenamespace = os.Getenv("KUBENAMESPACE")
	if kubenamespace == "" {
		kubenamespace = "kube-system"
	}

	ctx, cancel := context.WithCancel(context.Background())

	metricsHandler := metrics.Register()
	go metricsHandler.Run()

	configHandler := config.Register(
		ctx,
		kubeconfig_file,
		kubeconfig_context,
		"guestcluster-quota-webhook",
		kubenamespace,
		metricsHandler,
	)

	admissionHandler := admission.Register(
		ctx,
		kubeconfig_file,
		kubeconfig_context,
		"guestcluster-quota-webhook",
		kubenamespace,
		"guestcluster-quota-validator",
	)

	serviceHandler := service.Register(
		ctx,
		kubeconfig_file,
		kubeconfig_context,
		operateMode,
		metricsHandler,
	)

	configHandler.Init()
	configHandler.Run(certRenewalPeriod)
	admissionHandler.Init()
	scheduler.StartCertRenewalScheduler(configHandler, serviceHandler, certRenewalPeriod)
	serviceHandler.Init()
	go serviceHandler.Run()
	go Run()

	log.Infof("%s is running", progname)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	os.Exit(1)
}

func Run() {
	for {
		time.Sleep(time.Second)
	}
}
