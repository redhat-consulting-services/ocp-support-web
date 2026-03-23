package main

import (
	"log"
	"net/http"

	"github.com/redhat-consulting-services/ocp-support-web/internal/config"
	"github.com/redhat-consulting-services/ocp-support-web/internal/handler"
	"github.com/redhat-consulting-services/ocp-support-web/internal/metrics"
	"github.com/redhat-consulting-services/ocp-support-web/internal/monitoring"
	"github.com/redhat-consulting-services/ocp-support-web/internal/mustgather"
	"github.com/redhat-consulting-services/ocp-support-web/internal/status"
	"github.com/redhat-consulting-services/ocp-support-web/web"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mgr, err := mustgather.NewManager(cfg.MustGatherDir, mustgather.ImageConfig{
		DefaultMustGather:      cfg.Images.DefaultMustGather,
		CNVMustGather:          cfg.Images.CNVMustGather,
		ODFMustGather:          cfg.Images.ODFMustGather,
		ACMMustGather:          cfg.Images.ACMMustGather,
		LoggingMustGather:      cfg.Images.LoggingMustGather,
		ServiceMeshMustGather:  cfg.Images.ServiceMeshMustGather,
		ComplianceMustGather:   cfg.Images.ComplianceMustGather,
		MTCMustGather:          cfg.Images.MTCMustGather,
		GitOpsMustGather:       cfg.Images.GitOpsMustGather,
		ServerlessMustGather:   cfg.Images.ServerlessMustGather,
		MCEMustGather:          cfg.Images.MCEMustGather,
		NetObservMustGather:    cfg.Images.NetObservMustGather,
		LocalStorageMustGather: cfg.Images.LocalStorageMustGather,
		SandboxedMustGather:    cfg.Images.SandboxedMustGather,
		NHCMustGather:          cfg.Images.NHCMustGather,
		NUMAMustGather:         cfg.Images.NUMAMustGather,
		PTPMustGather:          cfg.Images.PTPMustGather,
		SecretsStoreMustGather: cfg.Images.SecretsStoreMustGather,
		LVMSMustGather:         cfg.Images.LVMSMustGather,
	})
	if err != nil {
		log.Fatalf("Failed to create must-gather manager: %v", err)
	}
	log.Printf("Must-gather support enabled (workdir: %s)", cfg.MustGatherDir)

	stClient := status.NewClient(
		cfg.OpenShift.APIURL,
		cfg.OpenShift.Token,
		cfg.OpenShift.InsecureSkipTLS,
	)
	log.Printf("Cluster status enabled")

	var monClient *monitoring.Client
	if cfg.OpenShift.ClusterDomain != "" {
		monClient = monitoring.NewClient(
			cfg.OpenShift.ClusterDomain,
			cfg.OpenShift.Token,
			cfg.OpenShift.InsecureSkipTLS,
		)
		log.Printf("Monitoring (etcd health) enabled")
	}

	h, err := handler.New(mgr, stClient, monClient, web.FS, version)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metrics.Handler())
		log.Printf("Metrics server listening on :8081")
		if err := http.ListenAndServe(":8081", metricsMux); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	log.Printf("OCP Support Web listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, metrics.Middleware(mux)); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
