package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/redhat-consulting-services/ocp-support-web/internal/acm"
	"github.com/redhat-consulting-services/ocp-support-web/internal/agent"
	"github.com/redhat-consulting-services/ocp-support-web/internal/collector"
	"github.com/redhat-consulting-services/ocp-support-web/internal/gather"
	"github.com/redhat-consulting-services/ocp-support-web/internal/config"
	"github.com/redhat-consulting-services/ocp-support-web/internal/handler"
	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"github.com/redhat-consulting-services/ocp-support-web/internal/metrics"
	"github.com/redhat-consulting-services/ocp-support-web/internal/monitoring"
	"github.com/redhat-consulting-services/ocp-support-web/internal/mustgather"
	"github.com/redhat-consulting-services/ocp-support-web/internal/status"
	"github.com/redhat-consulting-services/ocp-support-web/web"
)

var version = "dev"

// collectorAdapter bridges collector.Engine to the mustgather.Collector interface.
type collectorAdapter struct {
	engine *collector.Engine
}

func (a *collectorAdapter) Run(ctx context.Context, opts mustgather.CollectorRunOpts) error {
	return a.engine.Run(ctx, collector.RunOpts{
		GatherType:          opts.GatherType,
		Detected:            opts.Detected,
		DestDir:             opts.DestDir,
		Since:               opts.Since,
		SkipDefault:         opts.SkipDefault,
		OnStep:              opts.OnStep,
		OnLog:               opts.OnLog,
		CustomNamespaces:    opts.CustomNamespaces,
		CustomResourceTypes: opts.CustomResourceTypes,
		CustomIncludeLogs:   opts.CustomIncludeLogs,
	})
}

func (a *collectorAdapter) RunEtcdBackup(ctx context.Context, destDir string, logFn func(string), stepFn func(int, int, string)) error {
	return a.engine.RunEtcdBackup(ctx, destDir, logFn, stepFn)
}

func main() {
	// Must-gather mode: run as standalone must-gather image via oc adm must-gather
	if os.Getenv("MUST_GATHER") == "true" || (len(os.Args) > 1 && os.Args[1] == "gather") {
		gather.Run(version)
		return
	}

	// Agent mode: run as a lightweight gather agent on managed clusters
	if os.Getenv("AGENT_MODE") == "true" {
		agent.Run(version)
		return // never reached
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	k8sClient := k8s.NewClient(
		cfg.OpenShift.APIURL,
		cfg.OpenShift.Token,
		cfg.OpenShift.InsecureSkipTLS,
	)

	var coll mustgather.Collector
	if cfg.NativeGather {
		coll = &collectorAdapter{engine: collector.NewEngine(k8sClient, "ocp-support-web")}
		log.Printf("Native gather mode enabled")
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
	}, coll, cfg.NativeGather, k8sClient)
	if err != nil {
		log.Fatalf("Failed to create must-gather manager: %v", err)
	}
	if cfg.OpenShift.ClusterDomain != "" {
		mgr.SetClusterDomain(cfg.OpenShift.ClusterDomain)
	}
	// Detect cluster infrastructure name for archive naming
	if infraData, err := k8sClient.Get("/apis/config.openshift.io/v1/infrastructures/cluster"); err == nil {
		if infraName := k8s.JsonPath(infraData, "status", "infrastructureName"); infraName != "" {
			mgr.SetClusterName(infraName)
			log.Printf("Cluster infrastructure name: %s", infraName)
		}
	}
	log.Printf("Must-gather support enabled (workdir: %s)", cfg.MustGatherDir)

	stClient := status.NewClient(k8sClient)
	log.Printf("Cluster status enabled")

	// Always create ACM client — routes check availability at runtime,
	// and the frontend shows the tab based on capabilities detection.
	acmClient := acm.NewClient(k8sClient, cfg.MustGatherDir, cfg.OpenShift.ClusterDomain)
	log.Printf("ACM multi-cluster client initialized")

	var monClient *monitoring.Client
	if cfg.OpenShift.ClusterDomain != "" {
		monClient = monitoring.NewClient(
			cfg.OpenShift.ClusterDomain,
			k8sClient,
		)
		log.Printf("Monitoring (etcd health) enabled")
	}

	h, err := handler.New(mgr, stClient, monClient, acmClient, k8sClient, web.FS, version)
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
