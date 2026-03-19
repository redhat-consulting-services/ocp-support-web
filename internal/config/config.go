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
	DefaultMustGather string
	CNVMustGather     string
	ODFMustGather     string
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
			CNVMustGather:     envOr("MUST_GATHER_IMAGE_CNV", "registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0"),
			ODFMustGather:     envOr("MUST_GATHER_IMAGE_ODF", "registry.redhat.io/odf4/ocs-must-gather-rhel9:latest"),
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
