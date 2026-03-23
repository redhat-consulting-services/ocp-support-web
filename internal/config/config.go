package config

import (
	"fmt"
	"os"
	"strings"
)

type AppConfig struct {
	ListenAddr    string
	MustGatherDir string
	OpenShift     OpenShiftConfig
	Images        ImageConfig
}

type OpenShiftConfig struct {
	APIURL          string
	Token           string
	ClusterDomain   string
	InsecureSkipTLS bool
}

type ImageConfig struct {
	DefaultMustGather         string
	CNVMustGather             string
	ODFMustGather             string
	ACMMustGather             string
	LoggingMustGather         string
	ServiceMeshMustGather     string
	ComplianceMustGather      string
	MTCMustGather             string
	GitOpsMustGather          string
	ServerlessMustGather      string
	MCEMustGather             string
	NetObservMustGather       string
	LocalStorageMustGather    string
	SandboxedMustGather       string
	NHCMustGather             string
	NUMAMustGather            string
	PTPMustGather             string
	SecretsStoreMustGather    string
	LVMSMustGather            string
}

const saTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

func Load() (*AppConfig, error) {
	cfg := &AppConfig{
		ListenAddr:    envOr("LISTEN_ADDR", ":8080"),
		MustGatherDir: envOr("MUST_GATHER_DIR", "/tmp/ocp-support-web/gather"),
		OpenShift: OpenShiftConfig{
			APIURL:          os.Getenv("OPENSHIFT_API_URL"),
			Token:           os.Getenv("OPENSHIFT_TOKEN"),
			ClusterDomain:   os.Getenv("CLUSTER_DOMAIN"),
			InsecureSkipTLS: envOr("INSECURE_SKIP_TLS", "true") == "true",
		},
		Images: ImageConfig{
			DefaultMustGather: os.Getenv("MUST_GATHER_IMAGE_DEFAULT"),
			CNVMustGather:         os.Getenv("MUST_GATHER_IMAGE_CNV"),
			ODFMustGather:         os.Getenv("MUST_GATHER_IMAGE_ODF"),
			ACMMustGather:         os.Getenv("MUST_GATHER_IMAGE_ACM"),
			LoggingMustGather:     os.Getenv("MUST_GATHER_IMAGE_LOGGING"),
			ServiceMeshMustGather: os.Getenv("MUST_GATHER_IMAGE_SERVICE_MESH"),
			ComplianceMustGather:  envOr("MUST_GATHER_IMAGE_COMPLIANCE", "registry.redhat.io/compliance/openshift-compliance-must-gather-rhel8:latest"),
			MTCMustGather:         os.Getenv("MUST_GATHER_IMAGE_MTC"),
			GitOpsMustGather:      os.Getenv("MUST_GATHER_IMAGE_GITOPS"),
			ServerlessMustGather:  os.Getenv("MUST_GATHER_IMAGE_SERVERLESS"),
			MCEMustGather:             envOr("MUST_GATHER_IMAGE_MCE", "registry.redhat.io/multicluster-engine/must-gather-rhel8"),
			NetObservMustGather:       envOr("MUST_GATHER_IMAGE_NETOBSERV", "quay.io/netobserv/must-gather"),
			LocalStorageMustGather:    os.Getenv("MUST_GATHER_IMAGE_LOCAL_STORAGE"),
			SandboxedMustGather:       os.Getenv("MUST_GATHER_IMAGE_SANDBOXED"),
			NHCMustGather:             os.Getenv("MUST_GATHER_IMAGE_NHC"),
			NUMAMustGather:            os.Getenv("MUST_GATHER_IMAGE_NUMA"),
			PTPMustGather:             os.Getenv("MUST_GATHER_IMAGE_PTP"),
			SecretsStoreMustGather:    os.Getenv("MUST_GATHER_IMAGE_SECRETS_STORE"),
			LVMSMustGather:            os.Getenv("MUST_GATHER_IMAGE_LVMS"),
		},
	}

	if cfg.OpenShift.APIURL == "" {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT")
		if host != "" && port != "" {
			cfg.OpenShift.APIURL = fmt.Sprintf("https://%s:%s", host, port)
		}
	}

	if cfg.OpenShift.Token == "" {
		if tokenBytes, err := os.ReadFile(saTokenPath); err == nil {
			cfg.OpenShift.Token = strings.TrimSpace(string(tokenBytes))
		}
	}

	if cfg.OpenShift.APIURL == "" || cfg.OpenShift.Token == "" {
		return nil, fmt.Errorf("OPENSHIFT_API_URL and OPENSHIFT_TOKEN are required (or run in-cluster)")
	}

	cfg.OpenShift.APIURL = strings.TrimRight(cfg.OpenShift.APIURL, "/")

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
