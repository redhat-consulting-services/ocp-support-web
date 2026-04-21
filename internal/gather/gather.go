// Package gather implements standalone must-gather mode.
// When the image is used with `oc adm must-gather --image=...`,
// it auto-detects all installed operators and runs every applicable
// native gather, writing output to /must-gather.
package gather

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/collector"
	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

// outputDir is where oc adm must-gather expects output.
const outputDir = "/must-gather"

// operatorProbe defines a CRD to check for operator presence.
type operatorProbe struct {
	apiPath    string
	gatherType string
	label      string
	csvNS      string
	csvPrefix  string
}

var probes = []operatorProbe{
	{"/apis/hco.kubevirt.io/v1beta1/hyperconvergeds", "virtualization", "Virtualization", "openshift-cnv", "kubevirt-hyperconverged-operator.v"},
	{"/apis/ocs.openshift.io/v1/storageclusters", "odf", "ODF (Storage)", "openshift-storage", "ocs-operator.v"},
	{"/apis/operator.open-cluster-management.io/v1/multiclusterhubs", "acm", "ACM", "", ""},
	{"/apis/logging.openshift.io/v1/clusterloggings", "logging", "Logging", "openshift-logging", "cluster-logging.v"},
	{"/apis/maistra.io/v2/servicemeshcontrolplanes", "service-mesh", "Service Mesh", "openshift-operators", "servicemeshoperator.v"},
	{"/apis/compliance.openshift.io/v1alpha1/compliancescans", "compliance", "Compliance", "", ""},
	{"/apis/migration.openshift.io/v1alpha1/migrationcontrollers", "mtc", "MTC", "openshift-migration", "mtc-operator.v"},
	{"/apis/argoproj.io/v1beta1/argocds", "gitops", "GitOps", "openshift-gitops-operator", "openshift-gitops-operator.v"},
	{"/apis/operator.knative.dev/v1beta1/knativeservings", "serverless", "Serverless", "openshift-serverless", "serverless-operator.v"},
	{"/apis/multicluster.openshift.io/v1/multiclusterengines", "mce", "MCE", "multicluster-engine", "multicluster-engine.v"},
	{"/apis/flows.netobserv.io/v1beta2/flowcollectors", "netobserv", "Network Observability", "", ""},
	{"/apis/local.storage.openshift.io/v1/localvolumes", "local-storage", "Local Storage", "openshift-local-storage", "local-storage-operator.v"},
	{"/apis/kataconfiguration.openshift.io/v1/kataconfigs", "sandboxed", "Sandboxed Containers", "openshift-sandboxed-containers-operator", "sandboxed-containers-operator.v"},
	{"/apis/remediation.medik8s.io/v1alpha1/nodehealthchecks", "nhc", "Node Health Check", "openshift-workload-availability", "node-healthcheck-operator.v"},
	{"/apis/nodetopology.openshift.io/v2/numaresourcesschedulers", "numa", "NUMA Resources", "openshift-numaresources", "numaresources-operator.v"},
	{"/apis/ptp.openshift.io/v1/ptpconfigs", "ptp", "PTP", "openshift-ptp", "ptp-operator.v"},
	{"/apis/secrets-store.csi.x-k8s.io/v1/secretproviderclasses", "secrets-store", "Secrets Store", "openshift-cluster-csi-drivers", "secrets-store-csi-driver-operator.v"},
	{"/apis/lvm.topolvm.io/v1alpha1/lvmclusters", "lvms", "LVMS", "openshift-lvm-storage", "lvms-operator.v"},
}

// Run executes the standalone must-gather.
// It auto-detects installed operators, runs all applicable gathers,
// and writes output to /must-gather.
func Run(version string) {
	log.Printf("ocp-support-web must-gather mode (version %s)", version)
	start := time.Now()

	client := inClusterClient()

	// Detect cluster name for logging
	clusterName := "cluster"
	if infraData, err := client.Get("/apis/config.openshift.io/v1/infrastructures/cluster"); err == nil {
		if name := k8s.JsonPath(infraData, "status", "infrastructureName"); name != "" {
			clusterName = name
			log.Printf("Cluster: %s", clusterName)
		}
	}

	// Use our own namespace for debug pods
	namespace := "openshift-must-gather"
	if nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		namespace = strings.TrimSpace(string(nsBytes))
	}

	engine := collector.NewEngine(client, namespace)

	// Detect operators
	detected := detectOperators(client)
	log.Printf("Detected %d operators", len(detected))

	// Build list of gather types to run
	gatherTypes := []string{"default"}
	for gt := range detected {
		gatherTypes = append(gatherTypes, gt)
	}
	log.Printf("Will gather: %s", strings.Join(gatherTypes, ", "))

	// Ensure output directory exists
	os.MkdirAll(outputDir, 0755)

	logFn := func(msg string) {
		log.Printf("[gather] %s", msg)
	}
	stepFn := func(step, total int, label string) {
		log.Printf("[gather] Step %d/%d: %s", step, total, label)
	}

	// Run default gather first, then each operator gather (skip default on subsequent runs)
	var gatherErrors []string
	for i, gt := range gatherTypes {
		log.Printf("=== Gathering: %s ===", gt)
		err := engine.Run(context.Background(), collector.RunOpts{
			GatherType:  gt,
			Detected:    detected,
			DestDir:     outputDir,
			SkipDefault: i > 0,
			OnStep:      stepFn,
			OnLog:       logFn,
		})
		if err != nil {
			log.Printf("Warning: gather for %s failed: %v", gt, err)
			gatherErrors = append(gatherErrors, fmt.Sprintf("%s: %v", gt, err))
		}
	}

	elapsed := time.Since(start).Round(time.Second)
	if len(gatherErrors) > 0 {
		log.Printf("Completed with %d errors in %s", len(gatherErrors), elapsed)
	} else {
		log.Printf("Completed successfully in %s", elapsed)
	}
}

// inClusterClient creates a k8s client from the pod's service account.
func inClusterClient() *k8s.Client {
	apiHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	apiPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if apiHost == "" || apiPort == "" {
		log.Fatal("Must run in-cluster (KUBERNETES_SERVICE_HOST/PORT not set)")
	}

	token := ""
	if tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		token = strings.TrimSpace(string(tokenBytes))
	}
	if token == "" {
		log.Fatal("Service account token required")
	}

	return k8s.NewClient(fmt.Sprintf("https://%s:%s", apiHost, apiPort), token, true)
}

// detectOperators probes CRDs to find installed operators.
func detectOperators(client *k8s.Client) map[string]bool {
	detected := make(map[string]bool)

	for _, p := range probes {
		if _, err := client.Get(p.apiPath); err == nil {
			detected[p.gatherType] = true
			ver := ""
			if p.csvNS != "" && p.csvPrefix != "" {
				ver = csvVersion(client, p.csvNS, p.csvPrefix)
			}
			if ver != "" {
				log.Printf("  Detected: %s %s", p.label, ver)
			} else {
				log.Printf("  Detected: %s", p.label)
			}
		}
	}

	return detected
}

// csvVersion looks up the CSV version for an operator.
func csvVersion(client *k8s.Client, ns, prefix string) string {
	data, err := client.Get("/apis/operators.coreos.com/v1alpha1/namespaces/" + ns + "/clusterserviceversions")
	if err != nil {
		return ""
	}
	for _, item := range k8s.JsonArray(data, "items") {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		meta, _ := m["metadata"].(map[string]interface{})
		name, _ := meta["name"].(string)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		spec, _ := m["spec"].(map[string]interface{})
		v, _ := spec["version"].(string)
		if v != "" {
			return v
		}
	}
	return ""
}
