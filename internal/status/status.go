package status

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Client struct {
	apiURL     string
	token      string
	httpClient *http.Client
	argoNS     string

	mu             sync.Mutex
	outOfSyncSince map[string]time.Time // app name -> first seen OutOfSync
	lastArgoApps   []ArgoApp
}

func NewClient(apiURL, token string, insecureSkipTLS bool) *Client {
	c := &Client{
		apiURL: apiURL,
		token:  token,
		argoNS: "openshift-gitops",
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipTLS},
			},
		},
		outOfSyncSince: make(map[string]time.Time),
	}

	go func() {
		c.pollArgo()
		for range time.Tick(30 * time.Second) {
			c.pollArgo()
		}
	}()

	return c
}

type ClusterHealth struct {
	Version        string           `json:"version"`
	Status         string           `json:"status"`
	Platform       string           `json:"platform,omitempty"`
	Operators      []OperatorStatus `json:"operators"`
	Nodes          []NodeStatus     `json:"nodes"`
	ODF            *ODFStatus       `json:"odf"`
	ControlPlane   []OperatorStatus `json:"controlPlane"`
	EtcdEncryption string           `json:"etcdEncryption"`
}

type OperatorStatus struct {
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	Degraded    bool   `json:"degraded"`
	Progressing bool   `json:"progressing"`
	Message     string `json:"message,omitempty"`
}

type NodeStatus struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Roles  []string `json:"roles"`
}

type NodeInfo struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

func (c *Client) GetNodes() ([]NodeInfo, error) {
	data, err := c.get("/api/v1/nodes")
	if err != nil {
		return nil, err
	}
	var nodes []NodeInfo
	for _, item := range jsonArray(data, "items") {
		node := item.(map[string]interface{})
		name := jsonPath(node, "metadata", "name")
		labels := jsonMap(node, "metadata", "labels")
		labelMap := make(map[string]string)
		for k, v := range labels {
			if s, ok := v.(string); ok {
				labelMap[k] = s
			}
		}
		nodes = append(nodes, NodeInfo{Name: name, Labels: labelMap})
	}
	return nodes, nil
}

type ODFStatus struct {
	Installed bool   `json:"installed"`
	Name      string `json:"name,omitempty"`
	Phase     string `json:"phase,omitempty"`
}

type EtcdHealth struct {
	Healthy bool         `json:"healthy"`
	Members []EtcdMember `json:"members"`
}

type EtcdMember struct {
	Name     string  `json:"name"`
	Pod      string  `json:"pod"`
	IsLeader bool    `json:"isLeader"`
	Revision int64   `json:"revision"`
	DBSizeMB float64 `json:"dbSizeMB"`
	Keys     int64   `json:"keys"`
	Events   int64   `json:"events"`
}

type NodeUtilization struct {
	Name             string   `json:"name"`
	Roles            []string `json:"roles"`
	Status           string   `json:"status"`
	CPUCapacity      int64    `json:"cpuCapacity"`
	CPUAllocatable   int64    `json:"cpuAllocatable"`
	CPURequests      int64    `json:"cpuRequests"`
	CPUUsage         int64    `json:"cpuUsage"`
	MemCapacity      int64    `json:"memCapacity"`
	MemAllocatable   int64    `json:"memAllocatable"`
	MemRequests      int64    `json:"memRequests"`
	MemUsage         int64    `json:"memUsage"`
	CPUOvercommitPct float64  `json:"cpuOvercommitPct"`
	MemOvercommitPct float64  `json:"memOvercommitPct"`
	CPUUsagePct      float64  `json:"cpuUsagePct"`
	MemUsagePct      float64  `json:"memUsagePct"`
	PodCount         int      `json:"podCount"`
	PodCapacity      int      `json:"podCapacity"`
}

type TopConsumer struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	CPUUsage   int64  `json:"cpuUsage"`
	MemUsage   int64  `json:"memUsage"`
	CPURequest int64  `json:"cpuRequest"`
	MemRequest int64  `json:"memRequest"`
	CPUStr     string `json:"cpuStr"`
	MemStr     string `json:"memStr"`
	CPUReqStr  string `json:"cpuReqStr"`
	MemReqStr  string `json:"memReqStr"`
}

type TopConsumers struct {
	Pods []TopConsumer `json:"pods"`
	VMs  []TopConsumer `json:"vms"`
}

type ArgoApp struct {
	Name             string         `json:"name"`
	Namespace        string         `json:"namespace"`
	SyncStatus       string         `json:"syncStatus"`
	HealthStatus     string         `json:"healthStatus"`
	RepoURL          string         `json:"repoURL,omitempty"`
	Path             string         `json:"path,omitempty"`
	OutOfSyncSince   *time.Time     `json:"outOfSyncSince,omitempty"`
	OutOfSyncSeconds int            `json:"outOfSyncSeconds,omitempty"`
	Error            bool           `json:"error"`
	Resources        []ArgoResource `json:"resources,omitempty"`
	Conditions       []string       `json:"conditions,omitempty"`
}

type ArgoResource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Health    string `json:"health,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (c *Client) GetClusterHealth() (*ClusterHealth, error) {
	health := &ClusterHealth{}

	cv, err := c.get("/apis/config.openshift.io/v1/clusterversions/version")
	if err != nil {
		return nil, fmt.Errorf("cluster version: %w", err)
	}

	health.Version = jsonPath(cv, "status", "desired", "version")
	health.Platform = jsonPath(cv, "spec", "platform", "type")

	conditions := jsonArray(cv, "status", "conditions")
	health.Status = "Available"
	for _, cond := range conditions {
		cm := cond.(map[string]interface{})
		if cm["type"] == "Available" && cm["status"] == "False" {
			health.Status = "Unavailable"
		}
		if cm["type"] == "Degraded" && cm["status"] == "True" {
			health.Status = "Degraded"
		}
		if cm["type"] == "Progressing" && cm["status"] == "True" {
			health.Status = "Updating"
		}
	}

	cops, err := c.get("/apis/config.openshift.io/v1/clusteroperators")
	if err != nil {
		return nil, fmt.Errorf("cluster operators: %w", err)
	}

	controlPlaneOps := map[string]bool{
		"etcd": true, "kube-apiserver": true, "kube-controller-manager": true,
		"kube-scheduler": true, "openshift-apiserver": true,
	}
	importantOps := map[string]bool{
		"authentication": true, "console": true, "dns": true, "ingress": true,
		"network": true, "storage": true, "monitoring": true,
		"image-registry": true, "operator-lifecycle-manager": true,
	}

	for _, item := range jsonArray(cops, "items") {
		op := item.(map[string]interface{})
		name := jsonPath(op, "metadata", "name")
		if !controlPlaneOps[name] && !importantOps[name] {
			continue
		}

		conds := map[string]string{}
		var msg string
		for _, c := range jsonArray(op, "status", "conditions") {
			cm := c.(map[string]interface{})
			conds[cm["type"].(string)] = cm["status"].(string)
			if cm["type"] == "Degraded" && cm["status"] == "True" {
				if m, ok := cm["message"].(string); ok {
					msg = m
				}
			}
		}

		os := OperatorStatus{
			Name:        name,
			Available:   conds["Available"] == "True",
			Degraded:    conds["Degraded"] == "True",
			Progressing: conds["Progressing"] == "True",
			Message:     msg,
		}

		if controlPlaneOps[name] {
			health.ControlPlane = append(health.ControlPlane, os)
		} else {
			health.Operators = append(health.Operators, os)
		}
	}

	nodes, err := c.get("/api/v1/nodes")
	if err != nil {
		return nil, fmt.Errorf("nodes: %w", err)
	}

	for _, item := range jsonArray(nodes, "items") {
		node := item.(map[string]interface{})
		name := jsonPath(node, "metadata", "name")

		var roles []string
		labels := jsonMap(node, "metadata", "labels")
		for k := range labels {
			if len(k) > 24 && k[:24] == "node-role.kubernetes.io/" {
				roles = append(roles, k[24:])
			}
		}

		status := "NotReady"
		for _, c := range jsonArray(node, "status", "conditions") {
			cm := c.(map[string]interface{})
			if cm["type"] == "Ready" && cm["status"] == "True" {
				status = "Ready"
			}
		}

		health.Nodes = append(health.Nodes, NodeStatus{
			Name:   name,
			Status: status,
			Roles:  roles,
		})
	}

	odf, err := c.get("/apis/ocs.openshift.io/v1/storageclusters")
	if err != nil {
		health.ODF = &ODFStatus{Installed: false}
	} else {
		items := jsonArray(odf, "items")
		if len(items) == 0 {
			health.ODF = &ODFStatus{Installed: false}
		} else {
			sc := items[0].(map[string]interface{})
			health.ODF = &ODFStatus{
				Installed: true,
				Name:      jsonPath(sc, "metadata", "name"),
				Phase:     jsonPath(sc, "status", "phase"),
			}
		}
	}

	apiServer, err := c.get("/apis/config.openshift.io/v1/apiservers/cluster")
	if err != nil {
		health.EtcdEncryption = "Unknown"
	} else {
		encType := jsonPath(apiServer, "spec", "encryption", "type")
		switch encType {
		case "aescbc", "aesgcm":
			health.EtcdEncryption = "Encrypted (" + encType + ")"
		case "identity", "":
			health.EtcdEncryption = "Not encrypted"
		default:
			health.EtcdEncryption = encType
		}
	}

	return health, nil
}

type ClusterCapabilities struct {
	CNV            bool   `json:"cnv"`
	CNVVersion     string `json:"cnvVersion,omitempty"`
	ODF            bool   `json:"odf"`
	ODFVersion     string `json:"odfVersion,omitempty"`
	ACM            bool   `json:"acm"`
	ACMVersion     string `json:"acmVersion,omitempty"`
	Logging        bool   `json:"logging"`
	LoggingVersion string `json:"loggingVersion,omitempty"`
	ServiceMesh bool   `json:"serviceMesh"`
	Compliance  bool   `json:"compliance"`
	MTC         bool   `json:"mtc"`
	GitOps            bool   `json:"gitops"`
	GitOpsVersion     string `json:"gitopsVersion,omitempty"`
	Serverless        bool   `json:"serverless"`
	ServerlessVersion string `json:"serverlessVersion,omitempty"`
	ServiceMeshVersion   string `json:"serviceMeshVersion,omitempty"`
	MTCVersion           string `json:"mtcVersion,omitempty"`
	MCE                  bool   `json:"mce"`
	MCEVersion           string `json:"mceVersion,omitempty"`
	NetObserv            bool   `json:"netObserv"`
	LocalStorage         bool   `json:"localStorage"`
	LocalStorageVersion  string `json:"localStorageVersion,omitempty"`
	Sandboxed            bool   `json:"sandboxed"`
	SandboxedVersion     string `json:"sandboxedVersion,omitempty"`
	NHC                  bool   `json:"nhc"`
	NHCVersion           string `json:"nhcVersion,omitempty"`
	NUMA                 bool   `json:"numa"`
	NUMAVersion          string `json:"numaVersion,omitempty"`
	PTP                  bool   `json:"ptp"`
	PTPVersion           string `json:"ptpVersion,omitempty"`
	SecretsStore         bool   `json:"secretsStore"`
	SecretsStoreVersion  string `json:"secretsStoreVersion,omitempty"`
	LVMS                 bool   `json:"lvms"`
	LVMSVersion          string `json:"lvmsVersion,omitempty"`
}

func (c *Client) GetCapabilities() *ClusterCapabilities {
	caps := &ClusterCapabilities{}

	// Check for CNV (HyperConverged)
	if _, err := c.get("/apis/hco.kubevirt.io/v1beta1/hyperconvergeds"); err == nil {
		caps.CNV = true
		caps.CNVVersion = c.csvVersion("openshift-cnv", "kubevirt-hyperconverged-operator.v")
	}

	// Check for ODF (StorageCluster)
	if data, err := c.get("/apis/ocs.openshift.io/v1/storageclusters"); err == nil {
		if items := jsonArray(data, "items"); len(items) > 0 {
			caps.ODF = true
			caps.ODFVersion = c.csvVersion("openshift-storage", "ocs-operator.v")
		}
	}

	// Check for ACM (MultiClusterHub) and extract version
	if data, err := c.get("/apis/operator.open-cluster-management.io/v1/multiclusterhubs"); err == nil {
		if items := jsonArray(data, "items"); len(items) > 0 {
			caps.ACM = true
			if item, ok := items[0].(map[string]interface{}); ok {
				if st, ok := item["status"].(map[string]interface{}); ok {
					if v, ok := st["currentVersion"].(string); ok {
						caps.ACMVersion = v
					}
				}
			}
		}
	}

	// Check for OpenShift Logging
	if _, err := c.get("/apis/logging.openshift.io/v1/clusterloggings"); err == nil {
		caps.Logging = true
		caps.LoggingVersion = c.csvVersion("openshift-logging", "cluster-logging.v")
	}

	// Check for Service Mesh
	if _, err := c.get("/apis/maistra.io/v2/servicemeshcontrolplanes"); err == nil {
		caps.ServiceMesh = true
		caps.ServiceMeshVersion = c.csvVersion("openshift-operators", "servicemeshoperator.v")
	}

	// Check for Compliance Operator
	if _, err := c.get("/apis/compliance.openshift.io/v1alpha1/compliancescans"); err == nil {
		caps.Compliance = true
	}

	// Check for Migration Toolkit for Containers
	if _, err := c.get("/apis/migration.openshift.io/v1alpha1/migrationcontrollers"); err == nil {
		caps.MTC = true
		caps.MTCVersion = c.csvVersion("openshift-migration", "mtc-operator.v")
	}

	// Check for OpenShift GitOps
	if _, err := c.get("/apis/argoproj.io/v1beta1/argocds"); err == nil {
		caps.GitOps = true
		caps.GitOpsVersion = c.csvVersion("openshift-gitops-operator", "openshift-gitops-operator.v")
	}

	// Check for OpenShift Serverless
	if _, err := c.get("/apis/operator.knative.dev/v1beta1/knativeservings"); err == nil {
		caps.Serverless = true
		caps.ServerlessVersion = c.csvVersion("openshift-serverless", "serverless-operator.v")
	}

	// Check for Multicluster Engine (hosted control planes)
	if _, err := c.get("/apis/multicluster.openshift.io/v1/multiclusterengines"); err == nil {
		caps.MCE = true
		caps.MCEVersion = c.csvVersion("multicluster-engine", "multicluster-engine.v")
	}

	// Check for Network Observability
	if _, err := c.get("/apis/flows.netobserv.io/v1beta2/flowcollectors"); err == nil {
		caps.NetObserv = true
	}

	// Check for Local Storage Operator
	if _, err := c.get("/apis/local.storage.openshift.io/v1/localvolumes"); err == nil {
		caps.LocalStorage = true
		caps.LocalStorageVersion = c.csvVersion("openshift-local-storage", "local-storage-operator.v")
	}

	// Check for OpenShift Sandboxed Containers
	if _, err := c.get("/apis/kataconfiguration.openshift.io/v1/kataconfigs"); err == nil {
		caps.Sandboxed = true
		caps.SandboxedVersion = c.csvVersion("openshift-sandboxed-containers-operator", "sandboxed-containers-operator.v")
	}

	// Check for Node Health Check
	if _, err := c.get("/apis/remediation.medik8s.io/v1alpha1/nodehealthchecks"); err == nil {
		caps.NHC = true
		caps.NHCVersion = c.csvVersion("openshift-workload-availability", "node-healthcheck-operator.v")
	}

	// Check for NUMA Resources Operator
	if _, err := c.get("/apis/nodetopology.openshift.io/v2/numaresourcesschedulers"); err == nil {
		caps.NUMA = true
		caps.NUMAVersion = c.csvVersion("openshift-numaresources", "numaresources-operator.v")
	}

	// Check for PTP Operator
	if _, err := c.get("/apis/ptp.openshift.io/v1/ptpconfigs"); err == nil {
		caps.PTP = true
		caps.PTPVersion = c.csvVersion("openshift-ptp", "ptp-operator.v")
	}

	// Check for Secrets Store CSI Driver
	if _, err := c.get("/apis/secrets-store.csi.x-k8s.io/v1/secretproviderclasses"); err == nil {
		caps.SecretsStore = true
		caps.SecretsStoreVersion = c.csvVersion("openshift-cluster-csi-drivers", "secrets-store-csi-driver-operator.v")
	}

	// Check for LVM Storage (LVMS)
	if _, err := c.get("/apis/lvm.topolvm.io/v1alpha1/lvmclusters"); err == nil {
		caps.LVMS = true
		caps.LVMSVersion = c.csvVersion("openshift-lvm-storage", "lvms-operator.v")
	}

	return caps
}

// csvVersion queries the CSVs in a namespace and returns the full version
// string of the first CSV whose name starts with the given prefix.
func (c *Client) csvVersion(ns, prefix string) string {
	data, err := c.get("/apis/operators.coreos.com/v1alpha1/namespaces/" + ns + "/clusterserviceversions")
	if err != nil {
		return ""
	}
	for _, item := range jsonArray(data, "items") {
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

type NMStateNetwork struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	State   string   `json:"state"`
	Nodes   []string `json:"nodes"`
	Missing []string `json:"missing,omitempty"`
}

// IsNMStateInstalled checks if NMState operator is present on the cluster.
func (c *Client) IsNMStateInstalled() bool {
	_, err := c.get("/apis/nmstate.io/v1beta1/nodenetworkstates")
	return err == nil
}

func (c *Client) GetNMStateNetworks() ([]NMStateNetwork, error) {
	data, err := c.get("/apis/nmstate.io/v1beta1/nodenetworkstates")
	if err != nil {
		return nil, fmt.Errorf("nmstate: %w", err)
	}

	// Only show user-configured network types
	interestingTypes := map[string]bool{
		"vlan": true, "bond": true, "linux-bridge": true,
	}
	// Skip OVN/OVS internal and infrastructure interfaces
	skipPrefixes := []string{
		"br-int", "br-ex", "br-local", "ovn-k8s", "ovs-system",
		"patch-", "ovn", "genev_sys", "vxlan_sys",
	}

	var allNodes []string
	items := jsonArray(data, "items")

	type netInfo struct {
		netType string
		nodes   map[string]bool
	}
	networks := map[string]*netInfo{}

	for _, item := range items {
		nns := item.(map[string]interface{})
		nodeName := jsonPath(nns, "metadata", "name")
		allNodes = append(allNodes, nodeName)

		ifaces := jsonArray(nns, "status", "currentState", "interfaces")
		for _, iface := range ifaces {
			ifMap := iface.(map[string]interface{})
			name, _ := ifMap["name"].(string)
			ifType, _ := ifMap["type"].(string)
			state, _ := ifMap["state"].(string)

			if name == "" || !interestingTypes[ifType] {
				continue
			}

			skip := false
			for _, prefix := range skipPrefixes {
				if strings.HasPrefix(name, prefix) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}

			if _, ok := networks[name]; !ok {
				networks[name] = &netInfo{netType: ifType, nodes: map[string]bool{}}
			}
			if state == "up" {
				networks[name].nodes[nodeName] = true
			}
		}
	}

	var result []NMStateNetwork
	for name, info := range networks {
		net := NMStateNetwork{
			Name: name,
			Type: info.netType,
		}

		if len(info.nodes) == len(allNodes) {
			net.State = "up"
		} else if len(info.nodes) == 0 {
			net.State = "down"
		} else {
			net.State = "partial"
		}

		for _, n := range allNodes {
			if info.nodes[n] {
				net.Nodes = append(net.Nodes, n)
			} else {
				net.Missing = append(net.Missing, n)
			}
		}

		result = append(result, net)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Name < result[j].Name
	})

	// Return empty slice (not nil) so the section shows with "no VLANs" message
	if result == nil {
		result = []NMStateNetwork{}
	}
	return result, nil
}

type StorageClass struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	IsDefault         bool   `json:"isDefault"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
}

func (c *Client) GetStorageClasses() ([]StorageClass, error) {
	data, err := c.get("/apis/storage.k8s.io/v1/storageclasses")
	if err != nil {
		return nil, fmt.Errorf("storage classes: %w", err)
	}

	var result []StorageClass
	for _, item := range jsonArray(data, "items") {
		sc := item.(map[string]interface{})
		name := jsonPath(sc, "metadata", "name")
		annotations := jsonMap(sc, "metadata", "annotations")
		isDefault := false
		if annotations != nil {
			if v, ok := annotations["storageclass.kubernetes.io/is-default-class"].(string); ok && v == "true" {
				isDefault = true
			}
		}

		result = append(result, StorageClass{
			Name:              name,
			Provisioner:       jsonPath(sc, "provisioner"),
			IsDefault:         isDefault,
			ReclaimPolicy:     jsonPath(sc, "reclaimPolicy"),
			VolumeBindingMode: jsonPath(sc, "volumeBindingMode"),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDefault != result[j].IsDefault {
			return result[i].IsDefault
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (c *Client) GetClusterID() (string, error) {
	cv, err := c.get("/apis/config.openshift.io/v1/clusterversions/version")
	if err != nil {
		return "", fmt.Errorf("cluster version: %w", err)
	}
	return jsonPath(cv, "spec", "clusterID"), nil
}

// GPU resource keys for each vendor's device plugin
var gpuResourceKeys = []struct {
	key   string
	label string
}{
	{"nvidia.com/gpu", "NVIDIA"},
	{"amd.com/gpu", "AMD"},
	{"gpu.intel.com/i915", "Intel"},
	{"gpu.intel.com/xe", "Intel"},
}

type GPUNode struct {
	Name         string        `json:"name"`
	Roles        []string      `json:"roles"`
	Status       string        `json:"status"`
	GPUType      string        `json:"gpuType"`
	GPUCapacity  int           `json:"gpuCapacity"`
	GPUUsed      int           `json:"gpuUsed"`
	GPUFree      int           `json:"gpuFree"`
	GPUUsagePct  float64       `json:"gpuUsagePct"`
	GPUConsumers []GPUConsumer `json:"gpuConsumers"`
}

type GPUConsumer struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	GPUs      int    `json:"gpus"`
}

// gpuCount checks a resource map for any known GPU resource key and returns the count and vendor label.
func gpuCount(resources map[string]interface{}) (int, string) {
	if resources == nil {
		return 0, ""
	}
	for _, gk := range gpuResourceKeys {
		if v, ok := resources[gk.key].(string); ok {
			n := parseInt(v)
			if n > 0 {
				return n, gk.label
			}
		}
	}
	return 0, ""
}

func (c *Client) GetGPUNodes() ([]GPUNode, error) {
	nodesData, err := c.get("/api/v1/nodes")
	if err != nil {
		return nil, fmt.Errorf("nodes: %w", err)
	}

	podsData, err := c.get("/api/v1/pods?fieldSelector=status.phase%3DRunning")
	if err != nil {
		return nil, fmt.Errorf("pods: %w", err)
	}

	type gpuPodInfo struct {
		gpuReq    int
		consumers []GPUConsumer
	}
	gpuByNode := map[string]*gpuPodInfo{}
	for _, item := range jsonArray(podsData, "items") {
		pod := item.(map[string]interface{})
		nodeName := jsonPath(pod, "spec", "nodeName")
		if nodeName == "" {
			continue
		}
		podName := jsonPath(pod, "metadata", "name")
		ns := jsonPath(pod, "metadata", "namespace")

		var podGPU int
		for _, c := range jsonArray(pod, "spec", "containers") {
			container := c.(map[string]interface{})
			if n, _ := gpuCount(jsonMap(container, "resources", "requests")); n > 0 {
				podGPU += n
			} else if n, _ := gpuCount(jsonMap(container, "resources", "limits")); n > 0 {
				podGPU += n
			}
		}

		if podGPU > 0 {
			if gpuByNode[nodeName] == nil {
				gpuByNode[nodeName] = &gpuPodInfo{}
			}
			gpuByNode[nodeName].gpuReq += podGPU
			name := podName
			if strings.HasPrefix(podName, "virt-launcher-") {
				parts := strings.Split(podName, "-")
				if len(parts) > 2 {
					name = strings.Join(parts[2:len(parts)-1], "-")
				}
			}
			gpuByNode[nodeName].consumers = append(gpuByNode[nodeName].consumers, GPUConsumer{
				Name:      name,
				Namespace: ns,
				GPUs:      podGPU,
			})
		}
	}

	var result []GPUNode
	for _, item := range jsonArray(nodesData, "items") {
		node := item.(map[string]interface{})
		capacity := jsonMap(node, "status", "capacity")
		gpuCap, gpuType := gpuCount(capacity)
		if gpuCap == 0 {
			continue
		}

		name := jsonPath(node, "metadata", "name")

		var roles []string
		labels := jsonMap(node, "metadata", "labels")
		for k := range labels {
			if len(k) > 24 && k[:24] == "node-role.kubernetes.io/" {
				roles = append(roles, k[24:])
			}
		}

		status := "NotReady"
		for _, cond := range jsonArray(node, "status", "conditions") {
			cm := cond.(map[string]interface{})
			if cm["type"] == "Ready" && cm["status"] == "True" {
				status = "Ready"
			}
		}

		gpuUsed := 0
		var consumers []GPUConsumer
		if info, ok := gpuByNode[name]; ok {
			gpuUsed = info.gpuReq
			consumers = info.consumers
		}
		if consumers == nil {
			consumers = []GPUConsumer{}
		}

		gn := GPUNode{
			Name:         name,
			Roles:        roles,
			Status:       status,
			GPUType:      gpuType,
			GPUCapacity:  gpuCap,
			GPUUsed:      gpuUsed,
			GPUFree:      gpuCap - gpuUsed,
			GPUConsumers: consumers,
		}
		if gpuCap > 0 {
			gn.GPUUsagePct = float64(gpuUsed) / float64(gpuCap) * 100
		}
		result = append(result, gn)
	}

	return result, nil
}

func (c *Client) GetNodeUtilization() ([]NodeUtilization, error) {
	nodesData, err := c.get("/api/v1/nodes")
	if err != nil {
		return nil, fmt.Errorf("nodes: %w", err)
	}

	metricsData, err := c.get("/apis/metrics.k8s.io/v1beta1/nodes")
	if err != nil {
		return nil, fmt.Errorf("node metrics: %w", err)
	}

	podsData, err := c.get("/api/v1/pods?fieldSelector=status.phase%3DRunning")
	if err != nil {
		return nil, fmt.Errorf("pods: %w", err)
	}

	metricsMap := map[string]map[string]interface{}{}
	for _, item := range jsonArray(metricsData, "items") {
		m := item.(map[string]interface{})
		name := jsonPath(m, "metadata", "name")
		metricsMap[name] = m
	}

	type podResources struct {
		cpuReq int64
		memReq int64
		count  int
	}
	podsByNode := map[string]*podResources{}
	for _, item := range jsonArray(podsData, "items") {
		pod := item.(map[string]interface{})
		nodeName := jsonPath(pod, "spec", "nodeName")
		if nodeName == "" {
			continue
		}
		if podsByNode[nodeName] == nil {
			podsByNode[nodeName] = &podResources{}
		}
		podsByNode[nodeName].count++

		for _, c := range jsonArray(pod, "spec", "containers") {
			container := c.(map[string]interface{})
			requests := jsonMap(container, "resources", "requests")
			if requests != nil {
				if cpu, ok := requests["cpu"].(string); ok {
					podsByNode[nodeName].cpuReq += parseCPU(cpu)
				}
				if mem, ok := requests["memory"].(string); ok {
					podsByNode[nodeName].memReq += parseMemory(mem)
				}
			}
		}
	}

	var result []NodeUtilization
	for _, item := range jsonArray(nodesData, "items") {
		node := item.(map[string]interface{})
		name := jsonPath(node, "metadata", "name")

		var roles []string
		labels := jsonMap(node, "metadata", "labels")
		for k := range labels {
			if len(k) > 24 && k[:24] == "node-role.kubernetes.io/" {
				roles = append(roles, k[24:])
			}
		}

		status := "NotReady"
		for _, cond := range jsonArray(node, "status", "conditions") {
			cm := cond.(map[string]interface{})
			if cm["type"] == "Ready" && cm["status"] == "True" {
				status = "Ready"
			}
		}

		capacity := jsonMap(node, "status", "capacity")
		allocatable := jsonMap(node, "status", "allocatable")

		nu := NodeUtilization{
			Name:           name,
			Roles:          roles,
			Status:         status,
			CPUCapacity:    parseCPU(stringOrEmpty(capacity, "cpu")),
			CPUAllocatable: parseCPU(stringOrEmpty(allocatable, "cpu")),
			MemCapacity:    parseMemory(stringOrEmpty(capacity, "memory")),
			MemAllocatable: parseMemory(stringOrEmpty(allocatable, "memory")),
			PodCapacity:    parseInt(stringOrEmpty(capacity, "pods")),
		}

		if m, ok := metricsMap[name]; ok {
			usage := jsonMap(m, "usage")
			if usage != nil {
				nu.CPUUsage = parseCPU(stringOrEmpty(usage, "cpu"))
				nu.MemUsage = parseMemory(stringOrEmpty(usage, "memory"))
			}
		}

		if pr, ok := podsByNode[name]; ok {
			nu.CPURequests = pr.cpuReq
			nu.MemRequests = pr.memReq
			nu.PodCount = pr.count
		}

		if nu.CPUAllocatable > 0 {
			nu.CPUOvercommitPct = float64(nu.CPURequests) / float64(nu.CPUAllocatable) * 100
			nu.CPUUsagePct = float64(nu.CPUUsage) / float64(nu.CPUAllocatable) * 100
		}
		if nu.MemAllocatable > 0 {
			nu.MemOvercommitPct = float64(nu.MemRequests) / float64(nu.MemAllocatable) * 100
			nu.MemUsagePct = float64(nu.MemUsage) / float64(nu.MemAllocatable) * 100
		}

		result = append(result, nu)
	}

	return result, nil
}

func parseCPU(s string) int64 {
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "n") {
		v := parseInt64(strings.TrimSuffix(s, "n"))
		return v / 1000000
	}
	if strings.HasSuffix(s, "u") {
		v := parseInt64(strings.TrimSuffix(s, "u"))
		return v / 1000
	}
	if strings.HasSuffix(s, "m") {
		return parseInt64(strings.TrimSuffix(s, "m"))
	}
	return parseInt64(s) * 1000
}

func parseMemory(s string) int64 {
	if s == "" {
		return 0
	}
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"Ei", 1 << 60}, {"Pi", 1 << 50}, {"Ti", 1 << 40},
		{"Gi", 1 << 30}, {"Mi", 1 << 20}, {"Ki", 1 << 10},
		{"E", 1e18}, {"P", 1e15}, {"T", 1e12},
		{"G", 1e9}, {"M", 1e6}, {"k", 1e3},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			return parseInt64(strings.TrimSuffix(s, sf.suffix)) * sf.mult
		}
	}
	return parseInt64(s)
}

func parseInt(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func parseInt64(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func (c *Client) GetTopConsumers(limit int) (*TopConsumers, error) {
	metricsData, err := c.get("/apis/metrics.k8s.io/v1beta1/pods")
	if err != nil {
		return nil, fmt.Errorf("pod metrics: %w", err)
	}

	podsData, err := c.get("/api/v1/pods?fieldSelector=status.phase%3DRunning")
	if err != nil {
		return nil, fmt.Errorf("pods: %w", err)
	}

	type reqInfo struct {
		cpu int64
		mem int64
	}
	reqMap := map[string]*reqInfo{}
	for _, item := range jsonArray(podsData, "items") {
		pod := item.(map[string]interface{})
		key := jsonPath(pod, "metadata", "namespace") + "/" + jsonPath(pod, "metadata", "name")
		ri := &reqInfo{}
		for _, c := range jsonArray(pod, "spec", "containers") {
			container := c.(map[string]interface{})
			requests := jsonMap(container, "resources", "requests")
			if requests != nil {
				if cpu, ok := requests["cpu"].(string); ok {
					ri.cpu += parseCPU(cpu)
				}
				if mem, ok := requests["memory"].(string); ok {
					ri.mem += parseMemory(mem)
				}
			}
		}
		reqMap[key] = ri
	}

	var pods []TopConsumer
	var vms []TopConsumer

	for _, item := range jsonArray(metricsData, "items") {
		pm := item.(map[string]interface{})
		podName := jsonPath(pm, "metadata", "name")
		ns := jsonPath(pm, "metadata", "namespace")

		var totalCPU, totalMem int64
		for _, c := range jsonArray(pm, "containers") {
			container := c.(map[string]interface{})
			usage := jsonMap(container, "usage")
			if usage != nil {
				totalCPU += parseCPU(stringOrEmpty(usage, "cpu"))
				totalMem += parseMemory(stringOrEmpty(usage, "memory"))
			}
		}

		tc := TopConsumer{
			Namespace: ns,
			CPUUsage:  totalCPU,
			MemUsage:  totalMem,
			CPUStr:    formatCPUStr(totalCPU),
			MemStr:    formatMemStr(totalMem),
		}

		if ri, ok := reqMap[ns+"/"+podName]; ok {
			tc.CPURequest = ri.cpu
			tc.MemRequest = ri.mem
			tc.CPUReqStr = formatCPUStr(ri.cpu)
			tc.MemReqStr = formatMemStr(ri.mem)
		}

		if strings.HasPrefix(podName, "virt-launcher-") {
			parts := strings.Split(podName, "-")
			if len(parts) > 2 {
				vmParts := parts[2 : len(parts)-1]
				tc.Name = strings.Join(vmParts, "-")
			} else {
				tc.Name = podName
			}
			vms = append(vms, tc)
		} else {
			tc.Name = podName
			pods = append(pods, tc)
		}
	}

	sort.Slice(pods, func(i, j int) bool { return pods[i].CPUUsage > pods[j].CPUUsage })
	sort.Slice(vms, func(i, j int) bool { return vms[i].CPUUsage > vms[j].CPUUsage })

	if len(pods) > limit {
		pods = pods[:limit]
	}
	if len(vms) > limit {
		vms = vms[:limit]
	}

	return &TopConsumers{Pods: pods, VMs: vms}, nil
}

func formatCPUStr(millicores int64) string {
	return fmt.Sprintf("%.2f cores", float64(millicores)/1000)
}

func formatMemStr(bytes int64) string {
	gb := float64(bytes) / 1e9
	if gb >= 1 {
		return fmt.Sprintf("%.1f GB", gb)
	}
	return fmt.Sprintf("%.0f MB", float64(bytes)/1e6)
}

func (c *Client) pollArgo() {
	apps, err := c.fetchArgoApps()
	if err != nil {
		log.Printf("ArgoCD poll error: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	currentApps := map[string]bool{}

	for i := range apps {
		app := &apps[i]
		currentApps[app.Name] = true

		if app.SyncStatus != "Synced" {
			if _, exists := c.outOfSyncSince[app.Name]; !exists {
				c.outOfSyncSince[app.Name] = now
			}
			since := c.outOfSyncSince[app.Name]
			app.OutOfSyncSince = &since
			app.OutOfSyncSeconds = int(now.Sub(since).Seconds())
			app.Error = app.OutOfSyncSeconds > 180 // 3 minutes
		} else {
			delete(c.outOfSyncSince, app.Name)
		}
	}

	for name := range c.outOfSyncSince {
		if !currentApps[name] {
			delete(c.outOfSyncSince, name)
		}
	}

	c.lastArgoApps = apps
}

func (c *Client) GetArgoApps() []ArgoApp {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	result := make([]ArgoApp, len(c.lastArgoApps))
	copy(result, c.lastArgoApps)
	for i := range result {
		if result[i].OutOfSyncSince != nil {
			result[i].OutOfSyncSeconds = int(now.Sub(*result[i].OutOfSyncSince).Seconds())
			result[i].Error = result[i].OutOfSyncSeconds > 180
		}
	}
	return result
}

func (c *Client) fetchArgoApps() ([]ArgoApp, error) {
	data, err := c.get("/apis/argoproj.io/v1alpha1/namespaces/" + c.argoNS + "/applications")
	if err != nil {
		return nil, err
	}

	var apps []ArgoApp
	for _, item := range jsonArray(data, "items") {
		app := item.(map[string]interface{})

		a := ArgoApp{
			Name:         jsonPath(app, "metadata", "name"),
			Namespace:    jsonPath(app, "metadata", "namespace"),
			SyncStatus:   jsonPath(app, "status", "sync", "status"),
			HealthStatus: jsonPath(app, "status", "health", "status"),
			RepoURL:      jsonPath(app, "spec", "source", "repoURL"),
			Path:         jsonPath(app, "spec", "source", "path"),
		}

		for _, r := range jsonArray(app, "status", "resources") {
			res := r.(map[string]interface{})
			syncStatus := ""
			if s, ok := res["status"].(string); ok {
				syncStatus = s
			}
			healthStatus := ""
			if h, ok := res["health"].(map[string]interface{}); ok {
				if s, ok := h["status"].(string); ok {
					healthStatus = s
				}
			}
			msg := ""
			if h, ok := res["health"].(map[string]interface{}); ok {
				if m, ok := h["message"].(string); ok {
					msg = m
				}
			}

			if syncStatus != "" && syncStatus != "Synced" || healthStatus == "Degraded" || healthStatus == "Missing" {
				a.Resources = append(a.Resources, ArgoResource{
					Kind:      res["kind"].(string),
					Namespace: stringOrEmpty(res, "namespace"),
					Name:      res["name"].(string),
					Status:    syncStatus,
					Health:    healthStatus,
					Message:   msg,
				})
			}
		}

		for _, cond := range jsonArray(app, "status", "conditions") {
			cm := cond.(map[string]interface{})
			if msg, ok := cm["message"].(string); ok {
				a.Conditions = append(a.Conditions, msg)
			}
		}

		apps = append(apps, a)
	}

	return apps, nil
}

func (c *Client) DeleteArgoApp(appName string) error {
	url := fmt.Sprintf("%s/apis/argoproj.io/v1alpha1/namespaces/%s/applications/%s",
		c.apiURL, c.argoNS, appName)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned %d: %s", resp.StatusCode, string(body))
	}

	c.mu.Lock()
	delete(c.outOfSyncSince, appName)
	c.mu.Unlock()

	return nil
}

func (c *Client) SyncArgoApp(appName string) error {
	url := fmt.Sprintf("%s/apis/argoproj.io/v1alpha1/namespaces/%s/applications/%s",
		c.apiURL, c.argoNS, appName)

	appData, err := c.get(fmt.Sprintf("/apis/argoproj.io/v1alpha1/namespaces/%s/applications/%s", c.argoNS, appName))
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	revision := jsonPath(appData, "status", "sync", "revision")

	patchBody := fmt.Sprintf(`{"operation":{"initiatedBy":{"username":"gitops-ui"},"sync":{"revision":"%s"}}}`, revision)

	req, err := http.NewRequest("PATCH", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/merge-patch+json")
	req.Body = io.NopCloser(stringReader(patchBody))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) get(path string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", c.apiURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func jsonPath(data map[string]interface{}, keys ...string) string {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			if v, ok := current[key].(string); ok {
				return v
			}
			return ""
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return ""
		}
	}
	return ""
}

func jsonArray(data map[string]interface{}, keys ...string) []interface{} {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			if v, ok := current[key].([]interface{}); ok {
				return v
			}
			return nil
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return nil
}

func jsonMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, key := range keys {
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return current
}

func stringOrEmpty(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

type stringReaderType struct {
	s string
	i int
}

func (r *stringReaderType) Read(p []byte) (n int, err error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n = copy(p, r.s[r.i:])
	r.i += n
	return
}

func stringReader(s string) io.Reader {
	return &stringReaderType{s: s}
}
