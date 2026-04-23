package collector

import (
	"fmt"
	"log"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"go.yaml.in/yaml/v2"
)

// GatherDefinition describes what to collect for a single gather ConfigMap.
type GatherDefinition struct {
	DisplayName         string            `yaml:"displayName"`
	ClusterResources    []ResourceSpec    `yaml:"clusterResources"`
	NamespacedResources []ResourceSpec    `yaml:"namespacedResources"`
	PodLogs             []PodLogSpec      `yaml:"podLogs"`
	PodExecs            []PodExecSpec     `yaml:"podExecs"`
	NodeCommands        []NodeCommandSpec `yaml:"nodeCommands"`
}

// ResourceSpec identifies a Kubernetes resource type to collect.
type ResourceSpec struct {
	Group         string   `yaml:"group"`
	Version       string   `yaml:"version"`
	Resource      string   `yaml:"resource"`
	Namespaces    []string `yaml:"namespaces,omitempty"`
	LabelSelector string   `yaml:"labelSelector,omitempty"`
}

// APIPath returns the Kubernetes API path for listing this resource.
// If namespace is non-empty, returns the namespaced path.
func (r ResourceSpec) APIPath(namespace string) string {
	base := "/apis/" + r.Group + "/" + r.Version
	if r.Group == "" {
		base = "/api/" + r.Version
	}
	if namespace != "" {
		return base + "/namespaces/" + namespace + "/" + r.Resource
	}
	return base + "/" + r.Resource
}

// GroupDir returns the directory name for organizing resources by API group.
func (r ResourceSpec) GroupDir() string {
	if r.Group == "" {
		return "core"
	}
	return r.Group
}

// PodLogSpec describes which pod logs to collect.
type PodLogSpec struct {
	Namespaces    []string `yaml:"namespaces"`
	LabelSelector string   `yaml:"labelSelector,omitempty"`
	Previous      bool     `yaml:"previous,omitempty"`
	MaxLines      int      `yaml:"maxLines,omitempty"`
}

// PodExecSpec describes a command to exec into a running pod.
type PodExecSpec struct {
	Name        string   `yaml:"name"`
	Namespace   string   `yaml:"namespace"`
	PodSelector string   `yaml:"podSelector"`
	Container   string   `yaml:"container"`
	Command     []string `yaml:"command"`
	OutputFile  string   `yaml:"outputFile"`
}

// NodeCommandSpec describes a command to run on a node via a debug pod.
type NodeCommandSpec struct {
	Name         string   `yaml:"name"`
	NodeSelector string   `yaml:"nodeSelector"`
	Command      []string `yaml:"command"`
	OutputFile   string   `yaml:"outputFile"`
}

// allOpenShiftNamespaces uses wildcard matching to cover all openshift-* namespaces dynamically.
var allOpenShiftNamespaces = []string{"openshift-*"}

// builtinDefinitions maps gather types to their built-in definitions.
// These match the data collected by the official must-gather images.
// ConfigMaps with the same name can override these.
var builtinDefinitions = map[string]GatherDefinition{
	"default": {
		DisplayName: "Default cluster diagnostics",
		ClusterResources: []ResourceSpec{
			// config.openshift.io - all cluster configuration
			{Group: "config.openshift.io", Version: "v1", Resource: "clusterversions"},
			{Group: "config.openshift.io", Version: "v1", Resource: "clusteroperators"},
			{Group: "config.openshift.io", Version: "v1", Resource: "infrastructures"},
			{Group: "config.openshift.io", Version: "v1", Resource: "networks"},
			{Group: "config.openshift.io", Version: "v1", Resource: "oauths"},
			{Group: "config.openshift.io", Version: "v1", Resource: "proxies"},
			{Group: "config.openshift.io", Version: "v1", Resource: "schedulers"},
			{Group: "config.openshift.io", Version: "v1", Resource: "ingresses"},
			{Group: "config.openshift.io", Version: "v1", Resource: "apiservers"},
			{Group: "config.openshift.io", Version: "v1", Resource: "featuregates"},
			{Group: "config.openshift.io", Version: "v1", Resource: "dnses"},
			{Group: "config.openshift.io", Version: "v1", Resource: "builds"},
			{Group: "config.openshift.io", Version: "v1", Resource: "images"},
			{Group: "config.openshift.io", Version: "v1", Resource: "operatorhubs"},
			{Group: "config.openshift.io", Version: "v1", Resource: "projects"},
			// Core cluster resources
			{Group: "", Version: "v1", Resource: "nodes"},
			{Group: "", Version: "v1", Resource: "namespaces"},
			{Group: "", Version: "v1", Resource: "persistentvolumes"},
			{Group: "", Version: "v1", Resource: "componentstatuses"},
			// Storage
			{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"},
			{Group: "storage.k8s.io", Version: "v1", Resource: "volumeattachments"},
			{Group: "storage.k8s.io", Version: "v1", Resource: "csidrivers"},
			{Group: "storage.k8s.io", Version: "v1", Resource: "csinodes"},
			// Machine management
			{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigs"},
			{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"},
			{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "containerruntimeconfigs"},
			{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "kubeletconfigs"},
			{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machines"},
			{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinesets"},
			{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinehealthchecks"},
			{Group: "autoscaling.openshift.io", Version: "v1beta1", Resource: "machineautoscalers"},
			{Group: "autoscaling.openshift.io", Version: "v1", Resource: "clusterautoscalers"},
			// RBAC
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
			// Networking
			{Group: "networking.k8s.io", Version: "v1", Resource: "ingressclasses"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "ingresscontrollers"},
			{Group: "network.openshift.io", Version: "v1", Resource: "clusternetworks"},
			{Group: "network.openshift.io", Version: "v1", Resource: "hostsubnets"},
			{Group: "network.openshift.io", Version: "v1", Resource: "netnamespaces"},
			// Operator management
			{Group: "operator.openshift.io", Version: "v1", Resource: "kubeapiservers"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "kubecontrollermanagers"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "kubeschedulers"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "openshiftapiservers"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "authentications"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "consoles"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "dnses"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "etcds"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "networks"},
			{Group: "operator.openshift.io", Version: "v1", Resource: "storages"},
			{Group: "operator.openshift.io", Version: "v1alpha1", Resource: "imagecontentsourcepolicies"},
			// OLM
			{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"},
			{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "catalogsources"},
			{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "installplans"},
			{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions"},
			{Group: "operators.coreos.com", Version: "v1", Resource: "operatorgroups"},
			// Security
			{Group: "security.openshift.io", Version: "v1", Resource: "securitycontextconstraints"},
			// Monitoring
			{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"},
			{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"},
			{Group: "monitoring.coreos.com", Version: "v1alpha1", Resource: "alertmanagerconfigs"},
			// Image registry
			{Group: "imageregistry.operator.openshift.io", Version: "v1", Resource: "configs"},
			{Group: "image.openshift.io", Version: "v1", Resource: "images"},
			// Certificates
			{Group: "certificates.k8s.io", Version: "v1", Resource: "certificatesigningrequests"},
			// Node tuning
			{Group: "tuned.openshift.io", Version: "v1", Resource: "tuneds"},
			{Group: "tuned.openshift.io", Version: "v1", Resource: "profiles"},
			// Admission
			{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations"},
			{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingwebhookconfigurations"},
			// API services
			{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"},
			// Priority classes
			{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"},
		},
		NamespacedResources: []ResourceSpec{
			// Core resources across all openshift-* namespaces
			{Group: "", Version: "v1", Resource: "pods", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "services", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "secrets", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "events", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "serviceaccounts", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "persistentvolumeclaims", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "replicationcontrollers", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "resourcequotas", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "limitranges", Namespaces: allOpenShiftNamespaces},
			{Group: "", Version: "v1", Resource: "endpoints", Namespaces: allOpenShiftNamespaces},
			// Workloads
			{Group: "apps", Version: "v1", Resource: "deployments", Namespaces: allOpenShiftNamespaces},
			{Group: "apps", Version: "v1", Resource: "daemonsets", Namespaces: allOpenShiftNamespaces},
			{Group: "apps", Version: "v1", Resource: "statefulsets", Namespaces: allOpenShiftNamespaces},
			{Group: "apps", Version: "v1", Resource: "replicasets", Namespaces: allOpenShiftNamespaces},
			{Group: "batch", Version: "v1", Resource: "jobs", Namespaces: allOpenShiftNamespaces},
			{Group: "batch", Version: "v1", Resource: "cronjobs", Namespaces: allOpenShiftNamespaces},
			// Networking
			{Group: "route.openshift.io", Version: "v1", Resource: "routes", Namespaces: allOpenShiftNamespaces},
			{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses", Namespaces: allOpenShiftNamespaces},
			{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies", Namespaces: allOpenShiftNamespaces},
			{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices", Namespaces: allOpenShiftNamespaces},
			// RBAC per namespace
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles", Namespaces: allOpenShiftNamespaces},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings", Namespaces: allOpenShiftNamespaces},
			// Autoscaling
			{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers", Namespaces: allOpenShiftNamespaces},
			{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets", Namespaces: allOpenShiftNamespaces},
			// Also collect from kube-system and kube-public
			{Group: "", Version: "v1", Resource: "pods", Namespaces: []string{"kube-system", "kube-public"}},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: []string{"kube-system", "kube-public"}},
			{Group: "", Version: "v1", Resource: "events", Namespaces: []string{"kube-system", "kube-public"}},
		},
		PodLogs: []PodLogSpec{
			// Current logs from all openshift-* namespaces
			{Namespaces: allOpenShiftNamespaces, MaxLines: 5000},
			// Previous/crash logs from critical namespaces
			{Namespaces: allOpenShiftNamespaces, Previous: true, MaxLines: 2000},
		},
		PodExecs: []PodExecSpec{
			// === etcd diagnostics ===
			{Name: "etcd-member-list", Namespace: "openshift-etcd", PodSelector: "app=etcd", Container: "etcdctl", Command: []string{"etcdctl", "member", "list", "-w", "json"}, OutputFile: "etcd-member-list.json"},
			{Name: "etcd-endpoint-status", Namespace: "openshift-etcd", PodSelector: "app=etcd", Container: "etcdctl", Command: []string{"etcdctl", "endpoint", "status", "-w", "json"}, OutputFile: "etcd-endpoint-status.json"},
			{Name: "etcd-endpoint-health", Namespace: "openshift-etcd", PodSelector: "app=etcd", Container: "etcdctl", Command: []string{"etcdctl", "endpoint", "health", "-w", "json"}, OutputFile: "etcd-endpoint-health.json"},
			{Name: "etcd-alarm-list", Namespace: "openshift-etcd", PodSelector: "app=etcd", Container: "etcdctl", Command: []string{"etcdctl", "alarm", "list", "-w", "json"}, OutputFile: "etcd-alarm-list.json"},
			// === Prometheus internal API ===
			{Name: "prometheus-rules", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/rules"}, OutputFile: "prometheus-rules.json"},
			{Name: "prometheus-alertmanagers", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/alertmanagers"}, OutputFile: "prometheus-alertmanagers.json"},
			{Name: "prometheus-status-config", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/status/config"}, OutputFile: "prometheus-status-config.json"},
			{Name: "prometheus-status-flags", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/status/flags"}, OutputFile: "prometheus-status-flags.json"},
			{Name: "prometheus-targets-active", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/targets?state=active"}, OutputFile: "prometheus-targets-active.json"},
			{Name: "prometheus-tsdb-status", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/status/tsdb"}, OutputFile: "prometheus-tsdb-status.json"},
			{Name: "prometheus-runtime-info", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=prometheus", Container: "prometheus", Command: []string{"curl", "-s", "http://localhost:9090/api/v1/status/runtimeinfo"}, OutputFile: "prometheus-runtime-info.json"},
			// === Alertmanager internal API ===
			{Name: "alertmanager-status", Namespace: "openshift-monitoring", PodSelector: "app.kubernetes.io/name=alertmanager", Container: "alertmanager", Command: []string{"curl", "-s", "http://localhost:9093/api/v2/status"}, OutputFile: "alertmanager-status.json"},
			// === OVN-Kubernetes diagnostics ===
			{Name: "ovn-nbdb-status", Namespace: "openshift-ovn-kubernetes", PodSelector: "app=ovnkube-node", Container: "nbdb", Command: []string{"bash", "-c", "echo '=== sync-status ==='; ovs-appctl -t /var/run/ovn/ovnnb_db.ctl ovsdb-server/sync-status 2>&1 || true; echo '=== active-server ==='; ovs-appctl -t /var/run/ovn/ovnnb_db.ctl ovsdb-server/get-active-ovsdb-server 2>&1 || true; echo '=== list-dbs ==='; ovs-appctl -t /var/run/ovn/ovnnb_db.ctl ovsdb-server/list-dbs 2>&1 || true; echo '=== memory ==='; ovs-appctl -t /var/run/ovn/ovnnb_db.ctl memory/show 2>&1 || true; echo '=== cluster/status ==='; ovs-appctl -t /var/run/ovn/ovnnb_db.ctl cluster/status OVN_Northbound 2>&1 || echo 'not clustered'"}, OutputFile: "ovn-nbdb-status.txt"},
			{Name: "ovn-sbdb-status", Namespace: "openshift-ovn-kubernetes", PodSelector: "app=ovnkube-node", Container: "sbdb", Command: []string{"bash", "-c", "echo '=== sync-status ==='; ovs-appctl -t /var/run/ovn/ovnsb_db.ctl ovsdb-server/sync-status 2>&1 || true; echo '=== active-server ==='; ovs-appctl -t /var/run/ovn/ovnsb_db.ctl ovsdb-server/get-active-ovsdb-server 2>&1 || true; echo '=== list-dbs ==='; ovs-appctl -t /var/run/ovn/ovnsb_db.ctl ovsdb-server/list-dbs 2>&1 || true; echo '=== memory ==='; ovs-appctl -t /var/run/ovn/ovnsb_db.ctl memory/show 2>&1 || true; echo '=== cluster/status ==='; ovs-appctl -t /var/run/ovn/ovnsb_db.ctl cluster/status OVN_Southbound 2>&1 || echo 'not clustered'"}, OutputFile: "ovn-sbdb-status.txt"},
		},
		NodeCommands: []NodeCommandSpec{
			{
				Name:         "journal-kubelet",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "kubelet", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-kubelet.log",
			},
			{
				Name:         "journal-crio",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "crio", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-crio.log",
			},
			{
				Name:         "network-status",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "ip", "addr", "show"},
				OutputFile:   "ip-addr.txt",
			},
			{
				Name:         "network-routes",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "ip", "route", "show"},
				OutputFile:   "ip-route.txt",
			},
			{
				Name:         "df-output",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "df", "-h"},
				OutputFile:   "df.txt",
			},
			{
				Name:         "uptime",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "uptime"},
				OutputFile:   "uptime.txt",
			},
			{
				Name:         "dmesg",
				NodeSelector: "node-role.kubernetes.io/master=",
				Command:      []string{"chroot", "/host", "dmesg", "--time-format", "iso", "-T"},
				OutputFile:   "dmesg.log",
			},
			// Additional service logs (matching traditional must-gather)
			{
				Name:         "journal-networkmanager",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "NetworkManager", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-networkmanager.log",
			},
			{
				Name:         "journal-openvswitch",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "openvswitch", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-openvswitch.log",
			},
			{
				Name:         "journal-ovs-vswitchd",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "ovs-vswitchd", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-ovs-vswitchd.log",
			},
			{
				Name:         "journal-ovsdb-server",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "ovsdb-server", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-ovsdb-server.log",
			},
			{
				Name:         "journal-rpm-ostreed",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "rpm-ostreed", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-rpm-ostreed.log",
			},
			{
				Name:         "journal-mcd",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "journalctl", "-u", "machine-config-daemon-firstboot", "--no-pager", "-n", "5000"},
				OutputFile:   "journal-machine-config-daemon-firstboot.log",
			},
			// Additional node diagnostics
			{
				Name:         "lsblk",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "lsblk", "-o", "NAME,MAJ:MIN,RM,SIZE,RO,TYPE,MOUNTPOINT,FSTYPE"},
				OutputFile:   "lsblk.txt",
			},
			{
				Name:         "mount-output",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "mount"},
				OutputFile:   "mount.txt",
			},
			{
				Name:         "ss-output",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "ss", "-anop"},
				OutputFile:   "ss.txt",
			},
			{
				Name:         "proc-cmdline",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "cat", "/proc/cmdline"},
				OutputFile:   "proc-cmdline.txt",
			},
			{
				Name:         "ip-link-stats",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "ip", "-s", "link", "show"},
				OutputFile:   "ip-link-stats.txt",
			},
			{
				Name:         "iptables-filter",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "iptables-save"},
				OutputFile:   "iptables-save.txt",
			},
		},
	},
	"audit": {
		DisplayName: "API server audit logs",
		PodLogs: []PodLogSpec{
			{Namespaces: []string{"openshift-kube-apiserver"}, LabelSelector: "app=openshift-kube-apiserver", MaxLines: 50000},
			{Namespaces: []string{"openshift-apiserver"}, LabelSelector: "apiserver=true", MaxLines: 50000},
			{Namespaces: []string{"openshift-oauth-apiserver"}, MaxLines: 50000},
		},
	},
	"virtualization": {
		DisplayName: "OpenShift Virtualization",
		ClusterResources: []ResourceSpec{
			// HCO / KubeVirt operator CRDs (not in default)
			{Group: "hco.kubevirt.io", Version: "v1beta1", Resource: "hyperconvergeds"},
			{Group: "kubevirt.io", Version: "v1", Resource: "kubevirts"},
			{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "cdis"},
			{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "cdiconfigs"},
			{Group: "networkaddonsoperator.network.kubevirt.io", Version: "v1", Resource: "networkaddonsconfigs"},
			{Group: "ssp.kubevirt.io", Version: "v1beta2", Resource: "ssps"},
			// Instance types
			{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"},
			{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"},
			// Networking (not in default)
			{Group: "k8s.cni.cncf.io", Version: "v1", Resource: "network-attachment-definitions"},
			{Group: "nmstate.io", Version: "v1", Resource: "nodenetworkstates"},
			{Group: "nmstate.io", Version: "v1", Resource: "nodenetworkconfigurationpolicies"},
			{Group: "nmstate.io", Version: "v1", Resource: "nodenetworkconfigurationenactments"},
			// Storage (not in default)
			{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses"},
			{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotcontents"},
		},
		NamespacedResources: []ResourceSpec{
			// Operator-specific CRDs (all namespaces, not in default)
			{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"},
			{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstances"},
			{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstancemigrations"},
			{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstancereplicasets"},
			{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstancepresets"},
			{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datavolumes"},
			{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"},
			{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "dataimportcrons"},
			{Group: "export.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineexports"},
			{Group: "snapshot.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinesnapshots"},
			{Group: "snapshot.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinerestores"},
			{Group: "snapshot.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinesnapshotcontents"},
			{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"},
			{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"},
			{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"},
			// openshift-cnv resources are covered by default's openshift-* wildcard
		},
		PodLogs: []PodLogSpec{
			// openshift-cnv logs are covered by default's openshift-* wildcard
			// CDI pods with label selector (not covered by default)
			{Namespaces: allOpenShiftNamespaces, LabelSelector: "cdi.kubevirt.io", MaxLines: 5000},
		},
		PodExecs: []PodExecSpec{
			// virsh diagnostics via virt-handler — available on CNV ≤4.14, removed in 4.15+.
			// On newer versions these fail with warnings; VM data is still collected via KubeVirt API resources.
			{Name: "virsh-capabilities", Namespace: "openshift-cnv", PodSelector: "kubevirt.io=virt-handler", Container: "virt-handler", Command: []string{"virsh", "-r", "-c", "qemu:///system", "capabilities"}, OutputFile: "virsh-capabilities.xml"},
			{Name: "virsh-domcapabilities", Namespace: "openshift-cnv", PodSelector: "kubevirt.io=virt-handler", Container: "virt-handler", Command: []string{"virsh", "-r", "-c", "qemu:///system", "domcapabilities"}, OutputFile: "virsh-domcapabilities.xml"},
			{Name: "virsh-list-all", Namespace: "openshift-cnv", PodSelector: "kubevirt.io=virt-handler", Container: "virt-handler", Command: []string{"virsh", "-r", "-c", "qemu:///system", "list", "--all"}, OutputFile: "virsh-list.txt"},
			{Name: "virsh-dumpxml-all", Namespace: "openshift-cnv", PodSelector: "kubevirt.io=virt-handler", Container: "virt-handler", Command: []string{"bash", "-c", "for dom in $(virsh -r -c qemu:///system list --name 2>/dev/null); do [ -z \"$dom\" ] && continue; echo \"=== $dom ===\"; virsh -r -c qemu:///system dumpxml \"$dom\" 2>/dev/null; done"}, OutputFile: "virsh-dumpxml-all.xml"},
			{Name: "virsh-domblklist-all", Namespace: "openshift-cnv", PodSelector: "kubevirt.io=virt-handler", Container: "virt-handler", Command: []string{"bash", "-c", "for dom in $(virsh -r -c qemu:///system list --name 2>/dev/null); do [ -z \"$dom\" ] && continue; echo \"=== $dom ===\"; virsh -r -c qemu:///system domblklist \"$dom\" 2>/dev/null; done"}, OutputFile: "virsh-domblklist-all.txt"},
			{Name: "virsh-domjobinfo-all", Namespace: "openshift-cnv", PodSelector: "kubevirt.io=virt-handler", Container: "virt-handler", Command: []string{"bash", "-c", "for dom in $(virsh -r -c qemu:///system list --name 2>/dev/null); do [ -z \"$dom\" ] && continue; echo \"=== $dom ===\"; virsh -r -c qemu:///system domjobinfo \"$dom\" 2>/dev/null; done"}, OutputFile: "virsh-domjobinfo-all.txt"},
		},
		NodeCommands: []NodeCommandSpec{
			// lspci is CNV-specific (not in default)
			{
				Name:         "lspci",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "lspci", "-vv"},
				OutputFile:   "lspci.txt",
			},
			// VFIO/SR-IOV device info
			{
				Name:         "vfio-devices",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "bash", "-c", "ls -al /dev/vfio/ 2>/dev/null || echo 'no VFIO devices'"},
				OutputFile:   "vfio-devices.txt",
			},
			{
				Name:         "sriov-numvfs",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "bash", "-c", "for d in /sys/bus/pci/devices/*/sriov_numvfs; do [ -f \"$d\" ] && echo \"$d: $(cat $d)\"; done 2>/dev/null || echo 'no SR-IOV devices'"},
				OutputFile:   "sriov-numvfs.txt",
			},
			// Bridge and nftables info
			{
				Name:         "bridge-vlan",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "bridge", "-j", "vlan", "show"},
				OutputFile:   "bridge-vlan.json",
			},
			{
				Name:         "nft-ruleset",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "nft", "list", "ruleset"},
				OutputFile:   "nft-ruleset.txt",
			},
			// CNI config
			{
				Name:         "cni-config",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "bash", "-c", "ls -la /etc/cni/net.d/ 2>/dev/null; echo '---'; cat /etc/cni/net.d/*.conf /etc/cni/net.d/*.conflist 2>/dev/null || true"},
				OutputFile:   "cni-config.txt",
			},
			// Kernel dmesg (all nodes for CNV, not just masters)
			{
				Name:         "dmesg-all",
				NodeSelector: "",
				Command:      []string{"chroot", "/host", "dmesg", "--time-format", "iso", "-T"},
				OutputFile:   "dmesg.log",
			},
		},
	},
	"odf": {
		DisplayName: "OpenShift Data Foundation",
		ClusterResources: []ResourceSpec{
			// OCS CRDs (not in default)
			{Group: "ocs.openshift.io", Version: "v1", Resource: "storageclusters"},
			{Group: "ocs.openshift.io", Version: "v1", Resource: "ocsinitializations"},
			{Group: "ocs.openshift.io", Version: "v1", Resource: "storageconsumers"},
			{Group: "ocs.openshift.io", Version: "v1", Resource: "storageclients"},
			// Ceph CRDs (not in default)
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephclusters"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephblockpools"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephblockpoolradosnamespaces"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephfilesystems"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephfilesystemsubvolumegroups"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephobjectstores"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephobjectstoreusers"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephrbdmirrors"},
			{Group: "ceph.rook.io", Version: "v1", Resource: "cephclients"},
			// NooBaa CRDs (not in default)
			{Group: "noobaa.io", Version: "v1alpha1", Resource: "noobaas"},
			{Group: "noobaa.io", Version: "v1alpha1", Resource: "backingstores"},
			{Group: "noobaa.io", Version: "v1alpha1", Resource: "namespacestores"},
			{Group: "noobaa.io", Version: "v1alpha1", Resource: "bucketclasses"},
			{Group: "objectbucket.io", Version: "v1alpha1", Resource: "objectbucketclaims"},
			{Group: "objectbucket.io", Version: "v1alpha1", Resource: "objectbuckets"},
			// Volume snapshot CRDs (not in default)
			{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses"},
			{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotcontents"},
			// Volume replication CRDs (not in default)
			{Group: "replication.storage.openshift.io", Version: "v1alpha1", Resource: "volumereplicationclasses"},
			{Group: "replication.storage.openshift.io", Version: "v1alpha1", Resource: "volumereplications"},
			{Group: "replication.storage.openshift.io", Version: "v1alpha1", Resource: "volumegroupreplicationclasses"},
			{Group: "replication.storage.openshift.io", Version: "v1alpha1", Resource: "volumegroupreplications"},
			// CSI Addons CRDs (not in default)
			{Group: "csiaddons.openshift.io", Version: "v1alpha1", Resource: "csiaddonsnodes"},
			{Group: "csiaddons.openshift.io", Version: "v1alpha1", Resource: "reclaimspacejobs"},
			{Group: "csiaddons.openshift.io", Version: "v1alpha1", Resource: "reclaimspacecronjobs"},
			{Group: "csiaddons.openshift.io", Version: "v1alpha1", Resource: "networkfences"},
		},
		NamespacedResources: []ResourceSpec{
			// Volume snapshots across all namespaces (not in default)
			{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"},
			// openshift-storage namespaced resources are covered by default's openshift-* wildcard
		},
		PodLogs: []PodLogSpec{
			// openshift-storage logs are covered by default's openshift-* wildcard
			// NooBaa pods with label selector (not covered by default)
			{Namespaces: allOpenShiftNamespaces, LabelSelector: "app=noobaa", MaxLines: 5000},
		},
		PodExecs: []PodExecSpec{
			// Cluster health & status
			{Name: "ceph-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "status"}, OutputFile: "ceph-status.txt"},
			{Name: "ceph-status-json", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "status", "--format", "json-pretty"}, OutputFile: "ceph-status.json"},
			{Name: "ceph-health-detail", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "health", "detail"}, OutputFile: "ceph-health-detail.txt"},
			{Name: "ceph-df-detail", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "df", "detail"}, OutputFile: "ceph-df-detail.txt"},
			{Name: "ceph-df-json", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "df", "detail", "--format", "json-pretty"}, OutputFile: "ceph-df-detail.json"},
			{Name: "ceph-report", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "report"}, OutputFile: "ceph-report.json"},
			{Name: "ceph-versions", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "versions"}, OutputFile: "ceph-versions.txt"},
			// OSD
			{Name: "ceph-osd-tree", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "tree"}, OutputFile: "ceph-osd-tree.txt"},
			{Name: "ceph-osd-tree-json", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "tree", "--format", "json-pretty"}, OutputFile: "ceph-osd-tree.json"},
			{Name: "ceph-osd-df", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "df"}, OutputFile: "ceph-osd-df.txt"},
			{Name: "ceph-osd-df-tree", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "df", "tree"}, OutputFile: "ceph-osd-df-tree.txt"},
			{Name: "ceph-osd-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "dump"}, OutputFile: "ceph-osd-dump.txt"},
			{Name: "ceph-osd-stat", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "stat"}, OutputFile: "ceph-osd-stat.txt"},
			{Name: "ceph-osd-perf", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "perf"}, OutputFile: "ceph-osd-perf.txt"},
			{Name: "ceph-osd-blocked-by", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "blocked-by"}, OutputFile: "ceph-osd-blocked-by.txt"},
			{Name: "ceph-osd-pool-ls-detail", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "pool", "ls", "detail"}, OutputFile: "ceph-osd-pool-ls-detail.txt"},
			{Name: "ceph-osd-pool-autoscale", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "pool", "autoscale-status"}, OutputFile: "ceph-osd-pool-autoscale-status.txt"},
			{Name: "ceph-osd-crush-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "crush", "dump"}, OutputFile: "ceph-osd-crush-dump.json"},
			{Name: "ceph-osd-crush-rule-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "crush", "rule", "dump"}, OutputFile: "ceph-osd-crush-rule-dump.json"},
			{Name: "ceph-osd-crush-rule-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "crush", "rule", "ls"}, OutputFile: "ceph-osd-crush-rule-ls.txt"},
			{Name: "ceph-osd-crush-class-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "crush", "class", "ls"}, OutputFile: "ceph-osd-crush-class-ls.txt"},
			{Name: "ceph-osd-crush-show-tunables", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "crush", "show-tunables"}, OutputFile: "ceph-osd-crush-show-tunables.txt"},
			{Name: "ceph-osd-crush-weight-set-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "crush", "weight-set", "dump"}, OutputFile: "ceph-osd-crush-weight-set-dump.json"},
			{Name: "ceph-osd-getmaxosd", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "getmaxosd"}, OutputFile: "ceph-osd-getmaxosd.txt"},
			{Name: "ceph-osd-lspools", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "lspools"}, OutputFile: "ceph-osd-lspools.txt"},
			{Name: "ceph-osd-numa-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "numa-status"}, OutputFile: "ceph-osd-numa-status.txt"},
			{Name: "ceph-osd-utilization", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "utilization"}, OutputFile: "ceph-osd-utilization.txt"},
			{Name: "ceph-osd-blacklist-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "osd", "blacklist", "ls"}, OutputFile: "ceph-osd-blacklist-ls.txt"},
			// MON
			{Name: "ceph-mon-stat", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "mon", "stat"}, OutputFile: "ceph-mon-stat.txt"},
			{Name: "ceph-mon-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "mon", "dump"}, OutputFile: "ceph-mon-dump.txt"},
			{Name: "ceph-quorum-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "quorum_status"}, OutputFile: "ceph-quorum-status.json"},
			// MGR
			{Name: "ceph-mgr-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "mgr", "dump"}, OutputFile: "ceph-mgr-dump.json"},
			{Name: "ceph-mgr-module-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "mgr", "module", "ls"}, OutputFile: "ceph-mgr-module-ls.json"},
			{Name: "ceph-mgr-services", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "mgr", "services"}, OutputFile: "ceph-mgr-services.json"},
			// MDS (CephFS)
			{Name: "ceph-fs-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "fs", "dump"}, OutputFile: "ceph-fs-dump.txt"},
			{Name: "ceph-fs-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "fs", "ls"}, OutputFile: "ceph-fs-ls.txt"},
			{Name: "ceph-fs-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "fs", "status"}, OutputFile: "ceph-fs-status.txt"},
			{Name: "ceph-mds-stat", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "mds", "stat"}, OutputFile: "ceph-mds-stat.txt"},
			{Name: "ceph-fs-subvolumegroup-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "fs", "subvolumegroup", "ls", "ocs-storagecluster-cephfilesystem"}, OutputFile: "ceph-fs-subvolumegroup-ls.txt"},
			{Name: "ceph-fs-subvolume-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "fs", "subvolume", "ls", "ocs-storagecluster-cephfilesystem", "csi"}, OutputFile: "ceph-fs-subvolume-ls.txt"},
			// MDS tell commands (per-filesystem diagnostics)
			{Name: "ceph-tell-mds-client-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "tell", "mds.ocs-storagecluster-cephfilesystem:0", "client", "ls"}, OutputFile: "ceph-tell-mds-client-ls.json"},
			{Name: "ceph-tell-mds-session-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "tell", "mds.ocs-storagecluster-cephfilesystem:0", "session", "ls"}, OutputFile: "ceph-tell-mds-session-ls.json"},
			{Name: "ceph-tell-mds-damage-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "tell", "mds.ocs-storagecluster-cephfilesystem:0", "damage", "ls"}, OutputFile: "ceph-tell-mds-damage-ls.json"},
			{Name: "ceph-tell-mds-dump-ops-in-flight", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "tell", "mds.ocs-storagecluster-cephfilesystem:0", "dump_ops_in_flight"}, OutputFile: "ceph-tell-mds-dump-ops-in-flight.json"},
			{Name: "ceph-tell-mds-dump-blocked-ops", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "tell", "mds.ocs-storagecluster-cephfilesystem:0", "dump_blocked_ops"}, OutputFile: "ceph-tell-mds-dump-blocked-ops.json"},
			{Name: "ceph-tell-mds-dump-historic-ops", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "tell", "mds.ocs-storagecluster-cephfilesystem:0", "dump_historic_ops"}, OutputFile: "ceph-tell-mds-dump-historic-ops.json"},
			// PG
			{Name: "ceph-pg-stat", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "pg", "stat"}, OutputFile: "ceph-pg-stat.txt"},
			{Name: "ceph-pg-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "pg", "dump", "--format", "json-pretty"}, OutputFile: "ceph-pg-dump.json"},
			// Auth
			{Name: "ceph-auth-list", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "auth", "list"}, OutputFile: "ceph-auth-list.txt"},
			// Crash
			{Name: "ceph-crash-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "crash", "ls"}, OutputFile: "ceph-crash-ls.txt"},
			{Name: "ceph-crash-stat", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "crash", "stat"}, OutputFile: "ceph-crash-stat.txt"},
			// Config
			{Name: "ceph-config-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "config", "dump"}, OutputFile: "ceph-config-dump.txt"},
			{Name: "ceph-config-key-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "config-key", "ls"}, OutputFile: "ceph-config-key-ls.txt"},
			// Device
			{Name: "ceph-device-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "device", "ls"}, OutputFile: "ceph-device-ls.txt"},
			// Balancer
			{Name: "ceph-balancer-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "balancer", "status"}, OutputFile: "ceph-balancer-status.txt"},
			{Name: "ceph-balancer-pool-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "balancer", "pool", "ls"}, OutputFile: "ceph-balancer-pool-ls.txt"},
			// Time sync
			{Name: "ceph-time-sync-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "time-sync-status"}, OutputFile: "ceph-time-sync-status.txt"},
			// Health check history
			{Name: "ceph-healthcheck-history", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "healthcheck", "history", "ls"}, OutputFile: "ceph-healthcheck-history-ls.txt"},
			// Progress
			{Name: "ceph-progress", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "progress", "json"}, OutputFile: "ceph-progress.json"},
			// Service
			{Name: "ceph-service-dump", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "service", "dump"}, OutputFile: "ceph-service-dump.json"},
			// RBD
			{Name: "ceph-rbd-task-list", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "rbd", "task", "list"}, OutputFile: "ceph-rbd-task-list.txt"},
			// Rados
			{Name: "rados-lspools", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"rados", "lspools"}, OutputFile: "rados-lspools.txt"},
			{Name: "rados-df", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"rados", "df"}, OutputFile: "rados-df.txt"},
			// RBD per-pool iteration (matching traditional must-gather bash scripts)
			{Name: "rbd-ls-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rbd ls -p \"$pool\" 2>/dev/null; done"}, OutputFile: "rbd-ls-per-pool.txt"},
			{Name: "rbd-info-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do for img in $(rbd ls -p \"$pool\" 2>/dev/null); do echo \"=== $pool/$img ===\"; rbd info -p \"$pool\" \"$img\" 2>/dev/null; done; done"}, OutputFile: "rbd-info-per-pool.txt"},
			{Name: "rbd-status-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do for img in $(rbd ls -p \"$pool\" 2>/dev/null); do echo \"=== $pool/$img ===\"; rbd status -p \"$pool\" \"$img\" 2>/dev/null; done; done"}, OutputFile: "rbd-status-per-pool.txt"},
			{Name: "rbd-snap-ls-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do for img in $(rbd ls -p \"$pool\" 2>/dev/null); do echo \"=== $pool/$img ===\"; rbd snap ls --all -p \"$pool\" \"$img\" 2>/dev/null; done; done"}, OutputFile: "rbd-snap-ls-per-pool.txt"},
			{Name: "rbd-showmapped", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"rbd", "showmapped"}, OutputFile: "rbd-showmapped.txt"},
			{Name: "rbd-trash-ls-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rbd trash ls -p \"$pool\" 2>/dev/null; done"}, OutputFile: "rbd-trash-ls-per-pool.txt"},
			{Name: "rbd-group-ls-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rbd group ls -p \"$pool\" 2>/dev/null; done"}, OutputFile: "rbd-group-ls-per-pool.txt"},
			// RBD mirror
			{Name: "rbd-mirror-snapshot-schedule-status", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "rbd mirror snapshot schedule status 2>&1 || true"}, OutputFile: "rbd-mirror-snapshot-schedule-status.txt"},
			{Name: "rbd-mirror-snapshot-schedule-ls", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "rbd mirror snapshot schedule ls 2>&1 || true"}, OutputFile: "rbd-mirror-snapshot-schedule-ls.txt"},
			{Name: "rbd-mirror-pool-status-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rbd mirror pool status -p \"$pool\" 2>&1 || true; done"}, OutputFile: "rbd-mirror-pool-status-per-pool.txt"},
			{Name: "rbd-mirror-pool-info-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rbd mirror pool info -p \"$pool\" 2>&1 || true; done"}, OutputFile: "rbd-mirror-pool-info-per-pool.txt"},
			{Name: "rbd-mirror-group-status-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rbd mirror group status -p \"$pool\" 2>&1 || true; done"}, OutputFile: "rbd-mirror-group-status-per-pool.txt"},
			// Rados per-pool listing
			{Name: "rados-ls-per-pool", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for pool in $(rados lspools); do echo \"=== Pool: $pool ===\"; rados ls -p \"$pool\" 2>/dev/null | head -1000; done"}, OutputFile: "rados-ls-per-pool.txt"},
			// RGW (Object Gateway) commands
			{Name: "radosgw-admin-bucket-list", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "radosgw-admin bucket list 2>&1 || true"}, OutputFile: "radosgw-admin-bucket-list.json"},
			{Name: "radosgw-admin-bucket-stats", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "radosgw-admin bucket stats 2>&1 || true"}, OutputFile: "radosgw-admin-bucket-stats.json"},
			{Name: "radosgw-admin-realm-list", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "radosgw-admin realm list 2>&1 || true"}, OutputFile: "radosgw-admin-realm-list.json"},
			{Name: "radosgw-admin-zone-list", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "radosgw-admin zone list 2>&1 || true"}, OutputFile: "radosgw-admin-zone-list.json"},
			{Name: "radosgw-admin-zonegroup-list", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "radosgw-admin zonegroup list 2>&1 || true"}, OutputFile: "radosgw-admin-zonegroup-list.json"},
			// Per-OSD config show
			{Name: "ceph-config-show-per-osd", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for osd in $(ceph osd ls 2>/dev/null); do echo \"=== OSD.$osd ===\"; ceph config show osd.$osd 2>/dev/null; done"}, OutputFile: "ceph-config-show-per-osd.txt"},
			// CephFS subvolume info per subvolume
			{Name: "ceph-fs-subvolume-info", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"bash", "-c", "for sv in $(ceph fs subvolume ls ocs-storagecluster-cephfilesystem csi --format json 2>/dev/null | python3 -c 'import sys,json; [print(s[\"name\"]) for s in json.load(sys.stdin)]' 2>/dev/null); do echo \"=== $sv ===\"; ceph fs subvolume info ocs-storagecluster-cephfilesystem \"$sv\" csi 2>/dev/null; done"}, OutputFile: "ceph-fs-subvolume-info.txt"},
			// Ceph logs
			{Name: "ceph-log-cluster", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "log", "last", "10000", "debug", "cluster"}, OutputFile: "ceph-log-cluster.txt"},
			{Name: "ceph-log-audit", Namespace: "openshift-storage", PodSelector: "app=rook-ceph-tools", Container: "rook-ceph-tools", Command: []string{"ceph", "log", "last", "10000", "debug", "audit"}, OutputFile: "ceph-log-audit.txt"},
		},
	},
	"acm": {
		DisplayName: "Advanced Cluster Management",
		ClusterResources: []ResourceSpec{
			// ACM CRDs (not in default)
			{Group: "operator.open-cluster-management.io", Version: "v1", Resource: "multiclusterhubs"},
			{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta2", Resource: "managedclustersets"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta2", Resource: "managedclustersetbindings"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "placements"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "placementdecisions"},
			{Group: "internal.open-cluster-management.io", Version: "v1beta1", Resource: "managedclusterinfos"},
			// Policy / GRC
			{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"},
			{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "placementbindings"},
			{Group: "policy.open-cluster-management.io", Version: "v1beta1", Resource: "policysets"},
			{Group: "policy.open-cluster-management.io", Version: "v1beta1", Resource: "policyautomations"},
			// Application lifecycle
			{Group: "apps.open-cluster-management.io", Version: "v1", Resource: "channels"},
			{Group: "apps.open-cluster-management.io", Version: "v1", Resource: "subscriptions"},
			{Group: "apps.open-cluster-management.io", Version: "v1", Resource: "placementrules"},
			{Group: "app.k8s.io", Version: "v1beta1", Resource: "applications"},
			// Addons
			{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "clustermanagementaddons"},
			{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "managedclusteraddons"},
			{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "addondeploymentconfigs"},
			// Work
			{Group: "work.open-cluster-management.io", Version: "v1", Resource: "manifestworks"},
			{Group: "agent.open-cluster-management.io", Version: "v1", Resource: "klusterletaddonconfigs"},
			// Discovery
			{Group: "discovery.open-cluster-management.io", Version: "v1", Resource: "discoveredclusters"},
			{Group: "discovery.open-cluster-management.io", Version: "v1", Resource: "discoveryconfigs"},
			// Hive
			{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "clusterimagesets"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "clusterpools"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "machinepools"},
			// Search
			{Group: "search.open-cluster-management.io", Version: "v1alpha1", Resource: "searches"},
			// Observability
			{Group: "observability.open-cluster-management.io", Version: "v1beta2", Resource: "multiclusterobservabilities"},
			// Backup/Restore
			{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "backupschedules"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "restores"},
			// Submariner
			{Group: "submarineraddon.open-cluster-management.io", Version: "v1alpha1", Resource: "submarinerconfigs"},
			// Webhooks and OLM are already covered by default
		},
		NamespacedResources: []ResourceSpec{
			{Group: "", Version: "v1", Resource: "pods", Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "open-cluster-management-agent", "open-cluster-management-agent-addon", "open-cluster-management-backup", "open-cluster-management-observability", "multicluster-engine", "hive"}},
			{Group: "", Version: "v1", Resource: "services", Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "multicluster-engine"}},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "multicluster-engine", "open-cluster-management-observability"}},
			{Group: "", Version: "v1", Resource: "secrets", Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "multicluster-engine"}},
			{Group: "", Version: "v1", Resource: "events", Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "open-cluster-management-agent", "open-cluster-management-agent-addon", "multicluster-engine", "hive"}},
			{Group: "", Version: "v1", Resource: "serviceaccounts", Namespaces: []string{"open-cluster-management", "multicluster-engine"}},
			{Group: "apps", Version: "v1", Resource: "deployments", Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "multicluster-engine", "hive", "open-cluster-management-observability"}},
			{Group: "apps", Version: "v1", Resource: "statefulsets", Namespaces: []string{"open-cluster-management", "multicluster-engine", "open-cluster-management-observability"}},
			{Group: "apps", Version: "v1", Resource: "replicasets", Namespaces: []string{"open-cluster-management", "multicluster-engine"}},
			// OLM
			{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions", Namespaces: []string{"open-cluster-management", "multicluster-engine"}},
			{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions", Namespaces: []string{"open-cluster-management", "multicluster-engine"}},
			// RBAC
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles", Namespaces: []string{"open-cluster-management", "multicluster-engine"}},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings", Namespaces: []string{"open-cluster-management", "multicluster-engine"}},
		},
		PodLogs: []PodLogSpec{
			{Namespaces: []string{"open-cluster-management", "open-cluster-management-hub", "open-cluster-management-agent", "open-cluster-management-agent-addon", "open-cluster-management-observability", "multicluster-engine", "hive"}, MaxLines: 5000},
			{Namespaces: []string{"open-cluster-management", "multicluster-engine", "hive"}, Previous: true, MaxLines: 2000},
		},
	},
	"logging": {
		DisplayName: "OpenShift Logging",
		ClusterResources: []ResourceSpec{
			// Logging CRDs (not in default)
			{Group: "logging.openshift.io", Version: "v1", Resource: "clusterloggings"},
			{Group: "logging.openshift.io", Version: "v1", Resource: "clusterlogforwarders"},
			{Group: "observability.openshift.io", Version: "v1", Resource: "clusterlogforwarders"},
			{Group: "logging.openshift.io", Version: "v1", Resource: "elasticsearches"},
			{Group: "logging.openshift.io", Version: "v1", Resource: "kibanas"},
			{Group: "logging.openshift.io", Version: "v1", Resource: "logfilemetricexporters"},
			{Group: "loki.grafana.com", Version: "v1", Resource: "lokistacks"},
			{Group: "console.openshift.io", Version: "v1", Resource: "consoleplugins"},
		},
		// openshift-logging and openshift-operators-redhat namespaced resources + logs
		// are covered by default's openshift-* wildcard
		PodExecs: []PodExecSpec{
			// Elasticsearch internal API (TLS certs mounted in container)
			{Name: "es-cluster-health", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/health?v"}, OutputFile: "elasticsearch-health.txt"},
			{Name: "es-cat-nodes", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/nodes?v"}, OutputFile: "elasticsearch-nodes.txt"},
			{Name: "es-cat-indices", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca 'https://localhost:9200/_cat/indices?v&bytes=m'"}, OutputFile: "elasticsearch-indices.txt"},
			{Name: "es-cat-aliases", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/aliases?v"}, OutputFile: "elasticsearch-aliases.txt"},
			{Name: "es-cat-thread-pool", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/thread_pool?v"}, OutputFile: "elasticsearch-thread-pool.txt"},
			{Name: "es-hot-threads", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_nodes/hot_threads"}, OutputFile: "elasticsearch-hot-threads.txt"},
			{Name: "es-nodes-stats", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca 'https://localhost:9200/_nodes/stats?pretty'"}, OutputFile: "elasticsearch-nodes-stats.json"},
			{Name: "es-cat-shards", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/shards?v"}, OutputFile: "elasticsearch-shards.txt"},
			{Name: "es-cat-pending-tasks", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/pending_tasks?v"}, OutputFile: "elasticsearch-pending-tasks.txt"},
			{Name: "es-cat-recovery", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"bash", "-c", "curl -s --key /etc/elasticsearch/secret/admin-key --cert /etc/elasticsearch/secret/admin-cert --cacert /etc/elasticsearch/secret/admin-ca https://localhost:9200/_cat/recovery?v"}, OutputFile: "elasticsearch-recovery.txt"},
			// Elasticsearch storage info
			{Name: "es-storage-df", Namespace: "openshift-logging", PodSelector: "component=elasticsearch", Container: "elasticsearch", Command: []string{"df", "-h", "/elasticsearch/persistent"}, OutputFile: "elasticsearch-storage-df.txt"},
			// === Loki diagnostics (modern logging stack) ===
			// Loki ring status (distributor)
			{Name: "loki-distributor-ring", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=distributor", Container: "loki", Command: []string{"curl", "-s", "http://localhost:3100/distributor/ring"}, OutputFile: "loki-distributor-ring.txt"},
			// Loki ingester ring
			{Name: "loki-ingester-ring", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=ingester", Container: "loki", Command: []string{"curl", "-s", "http://localhost:3100/ring"}, OutputFile: "loki-ingester-ring.txt"},
			// Loki readiness/metrics
			{Name: "loki-compactor-metrics", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=compactor", Container: "loki", Command: []string{"curl", "-s", "http://localhost:3100/metrics"}, OutputFile: "loki-compactor-metrics.txt"},
			{Name: "loki-ingester-metrics", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=ingester", Container: "loki", Command: []string{"curl", "-s", "http://localhost:3100/metrics"}, OutputFile: "loki-ingester-metrics.txt"},
			// Loki config
			{Name: "loki-config", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=distributor", Container: "loki", Command: []string{"curl", "-s", "http://localhost:3100/config"}, OutputFile: "loki-config.yaml"},
			// Loki ready check
			{Name: "loki-ready", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=query-frontend", Container: "loki", Command: []string{"curl", "-s", "http://localhost:3100/ready"}, OutputFile: "loki-ready.txt"},
			// Vector diagnostics
			{Name: "vector-graph", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=collector", Container: "collector", Command: []string{"curl", "-s", "http://localhost:8686/api/graph"}, OutputFile: "vector-graph.json"},
			{Name: "vector-health", Namespace: "openshift-logging", PodSelector: "app.kubernetes.io/component=collector", Container: "collector", Command: []string{"curl", "-s", "http://localhost:8686/api/health"}, OutputFile: "vector-health.json"},
		},
	},
	"service-mesh": {
		DisplayName: "OpenShift Service Mesh",
		ClusterResources: []ResourceSpec{
			// Maistra / OSSM CRDs (not in default)
			{Group: "maistra.io", Version: "v2", Resource: "servicemeshcontrolplanes"},
			{Group: "maistra.io", Version: "v1", Resource: "servicemeshmemberrolls"},
			{Group: "maistra.io", Version: "v1", Resource: "servicemeshmembers"},
			// Istio networking (not in default)
			{Group: "networking.istio.io", Version: "v1", Resource: "virtualservices"},
			{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"},
			{Group: "networking.istio.io", Version: "v1", Resource: "gateways"},
			{Group: "networking.istio.io", Version: "v1", Resource: "serviceentries"},
			{Group: "networking.istio.io", Version: "v1", Resource: "envoyfilters"},
			{Group: "networking.istio.io", Version: "v1", Resource: "sidecars"},
			{Group: "networking.istio.io", Version: "v1", Resource: "workloadentries"},
			{Group: "networking.istio.io", Version: "v1", Resource: "workloadgroups"},
			// Istio security (not in default)
			{Group: "security.istio.io", Version: "v1", Resource: "peerauthentications"},
			{Group: "security.istio.io", Version: "v1", Resource: "authorizationpolicies"},
			{Group: "security.istio.io", Version: "v1", Resource: "requestauthentications"},
			// Istio telemetry (not in default)
			{Group: "telemetry.istio.io", Version: "v1alpha1", Resource: "telemetries"},
			// Kiali / Jaeger (not in default)
			{Group: "kiali.io", Version: "v1alpha1", Resource: "kialis"},
			{Group: "jaegertracing.io", Version: "v1", Resource: "jaegers"},
		},
		NamespacedResources: []ResourceSpec{
			// istio-system is NOT openshift-*, so we need to collect it here
			{Group: "", Version: "v1", Resource: "pods", Namespaces: []string{"istio-system"}},
			{Group: "", Version: "v1", Resource: "services", Namespaces: []string{"istio-system"}},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: []string{"istio-system"}},
			{Group: "", Version: "v1", Resource: "secrets", Namespaces: []string{"istio-system"}},
			{Group: "", Version: "v1", Resource: "events", Namespaces: []string{"istio-system"}},
			{Group: "", Version: "v1", Resource: "serviceaccounts", Namespaces: []string{"istio-system"}},
			{Group: "apps", Version: "v1", Resource: "deployments", Namespaces: []string{"istio-system"}},
			{Group: "apps", Version: "v1", Resource: "replicasets", Namespaces: []string{"istio-system"}},
			{Group: "apps", Version: "v1", Resource: "statefulsets", Namespaces: []string{"istio-system"}},
			{Group: "k8s.cni.cncf.io", Version: "v1", Resource: "network-attachment-definitions", Namespaces: []string{"istio-system"}},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles", Namespaces: []string{"istio-system"}},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings", Namespaces: []string{"istio-system"}},
			// openshift-operators is covered by default's openshift-* wildcard
		},
		PodLogs: []PodLogSpec{
			// istio-system logs (not covered by default's openshift-* wildcard)
			{Namespaces: []string{"istio-system"}, MaxLines: 5000},
			{Namespaces: []string{"istio-system"}, Previous: true, MaxLines: 2000},
			// openshift-operators logs are covered by default
		},
		PodExecs: []PodExecSpec{
			// Istiod/Pilot debug endpoints
			{Name: "pilot-syncz", Namespace: "istio-system", PodSelector: "app=istiod", Container: "discovery", Command: []string{"pilot-discovery", "request", "GET", "/debug/syncz"}, OutputFile: "pilot-syncz.json"},
			{Name: "pilot-adsz", Namespace: "istio-system", PodSelector: "app=istiod", Container: "discovery", Command: []string{"pilot-discovery", "request", "GET", "/debug/adsz"}, OutputFile: "pilot-adsz.json"},
			{Name: "pilot-registryz", Namespace: "istio-system", PodSelector: "app=istiod", Container: "discovery", Command: []string{"pilot-discovery", "request", "GET", "/debug/registryz"}, OutputFile: "pilot-registryz.json"},
			{Name: "pilot-endpointz", Namespace: "istio-system", PodSelector: "app=istiod", Container: "discovery", Command: []string{"pilot-discovery", "request", "GET", "/debug/endpointz"}, OutputFile: "pilot-endpointz.json"},
			{Name: "pilot-configz", Namespace: "istio-system", PodSelector: "app=istiod", Container: "discovery", Command: []string{"pilot-discovery", "request", "GET", "/debug/configz"}, OutputFile: "pilot-configz.json"},
		},
	},
	"compliance": {
		DisplayName: "Compliance Operator",
		ClusterResources: []ResourceSpec{
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "compliancescans"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "compliancesuites"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "scansettings"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "scansettingbindings"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "complianceremediations"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "compliancecheckresults"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "profiles"},
			{Group: "compliance.openshift.io", Version: "v1alpha1", Resource: "tailoredprofiles"},
		},
		// openshift-compliance resources + logs covered by default's openshift-* wildcard
	},
	"mtc": {
		DisplayName: "Migration Toolkit for Containers",
		ClusterResources: []ResourceSpec{
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "migrationcontrollers"},
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "migplans"},
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "migmigrations"},
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "migclusters"},
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "migstorages"},
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "directvolumemigrations"},
			{Group: "migration.openshift.io", Version: "v1alpha1", Resource: "directimagemigrations"},
			{Group: "velero.io", Version: "v1", Resource: "backups"},
			{Group: "velero.io", Version: "v1", Resource: "restores"},
		},
		// openshift-migration resources + logs covered by default's openshift-* wildcard
	},
	"gitops": {
		DisplayName: "OpenShift GitOps",
		ClusterResources: []ResourceSpec{
			{Group: "argoproj.io", Version: "v1beta1", Resource: "argocds"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "rolloutmanagers"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "analysistemplates"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "clusteranalysistemplates"},
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "analysisruns"},
			{Group: "pipelines.openshift.io", Version: "v1alpha1", Resource: "gitopsservices"},
		},
		// openshift-gitops resources + logs covered by default's openshift-* wildcard
	},
	"serverless": {
		DisplayName: "OpenShift Serverless",
		ClusterResources: []ResourceSpec{
			{Group: "operator.knative.dev", Version: "v1beta1", Resource: "knativeservings"},
			{Group: "operator.knative.dev", Version: "v1beta1", Resource: "knativeeventings"},
			{Group: "operator.serverless.openshift.io", Version: "v1alpha1", Resource: "knativekafkas"},
			{Group: "serving.knative.dev", Version: "v1", Resource: "services"},
			{Group: "serving.knative.dev", Version: "v1", Resource: "configurations"},
			{Group: "serving.knative.dev", Version: "v1", Resource: "revisions"},
			{Group: "serving.knative.dev", Version: "v1", Resource: "routes"},
			{Group: "eventing.knative.dev", Version: "v1", Resource: "brokers"},
			{Group: "eventing.knative.dev", Version: "v1", Resource: "triggers"},
			{Group: "eventing.knative.dev", Version: "v1", Resource: "eventtypes"},
			{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"},
			{Group: "sources.knative.dev", Version: "v1", Resource: "pingsources"},
			{Group: "sources.knative.dev", Version: "v1beta1", Resource: "kafkasources"},
			{Group: "messaging.knative.dev", Version: "v1", Resource: "channels"},
			{Group: "messaging.knative.dev", Version: "v1", Resource: "subscriptions"},
			{Group: "messaging.knative.dev", Version: "v1beta1", Resource: "kafkachannels"},
		},
		NamespacedResources: []ResourceSpec{
			// knative-* namespaces are NOT openshift-*, so we need them here
			{Group: "", Version: "v1", Resource: "pods", Namespaces: []string{"knative-serving", "knative-eventing", "knative-serving-ingress"}},
			{Group: "", Version: "v1", Resource: "services", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "", Version: "v1", Resource: "secrets", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "", Version: "v1", Resource: "events", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "", Version: "v1", Resource: "serviceaccounts", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "apps", Version: "v1", Resource: "deployments", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "apps", Version: "v1", Resource: "replicasets", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings", Namespaces: []string{"knative-serving", "knative-eventing"}},
			{Group: "coordination.k8s.io", Version: "v1", Resource: "leases", Namespaces: []string{"knative-serving", "knative-eventing"}},
			// openshift-serverless is covered by default's openshift-* wildcard
		},
		PodLogs: []PodLogSpec{
			// knative-* logs (not covered by default)
			{Namespaces: []string{"knative-serving", "knative-eventing", "knative-serving-ingress"}, MaxLines: 5000},
			{Namespaces: []string{"knative-serving", "knative-eventing"}, Previous: true, MaxLines: 2000},
			// openshift-serverless logs covered by default
		},
	},
	"mce": {
		DisplayName: "Multicluster Engine",
		ClusterResources: []ResourceSpec{
			// MCE CRDs (not in default)
			{Group: "multicluster.openshift.io", Version: "v1", Resource: "multiclusterengines"},
			{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "placements"},
			{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "placementdecisions"},
			{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "clustermanagementaddons"},
			{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "managedclusteraddons"},
			{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "addondeploymentconfigs"},
			{Group: "agent.open-cluster-management.io", Version: "v1", Resource: "klusterletaddonconfigs"},
			{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedclusters"},
			{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "nodepools"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "clusterimagesets"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "clusterpools"},
			{Group: "hive.openshift.io", Version: "v1", Resource: "machinepools"},
			{Group: "agent-install.openshift.io", Version: "v1beta1", Resource: "agentserviceconfigs"},
			{Group: "agent-install.openshift.io", Version: "v1beta1", Resource: "infraenvs"},
			{Group: "extensions.hive.openshift.io", Version: "v1beta1", Resource: "agentclusterinstalls"},
			{Group: "metal3.io", Version: "v1alpha1", Resource: "baremetalhosts"},
			{Group: "discovery.open-cluster-management.io", Version: "v1", Resource: "discoveredclusters"},
			{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "clusters"},
			{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machinedeployments"},
			{Group: "console.openshift.io", Version: "v1", Resource: "consoleplugins"},
		},
		NamespacedResources: []ResourceSpec{
			// multicluster-engine, hypershift, hive are NOT openshift-*, so we need them here
			{Group: "", Version: "v1", Resource: "pods", Namespaces: []string{"multicluster-engine", "hypershift", "hive"}},
			{Group: "", Version: "v1", Resource: "services", Namespaces: []string{"multicluster-engine", "hypershift"}},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: []string{"multicluster-engine", "hypershift", "hive"}},
			{Group: "", Version: "v1", Resource: "secrets", Namespaces: []string{"multicluster-engine"}},
			{Group: "", Version: "v1", Resource: "events", Namespaces: []string{"multicluster-engine", "hypershift", "hive"}},
			{Group: "", Version: "v1", Resource: "serviceaccounts", Namespaces: []string{"multicluster-engine"}},
			{Group: "apps", Version: "v1", Resource: "deployments", Namespaces: []string{"multicluster-engine", "hypershift", "hive"}},
			{Group: "apps", Version: "v1", Resource: "statefulsets", Namespaces: []string{"multicluster-engine"}},
			{Group: "apps", Version: "v1", Resource: "replicasets", Namespaces: []string{"multicluster-engine"}},
		},
		PodLogs: []PodLogSpec{
			{Namespaces: []string{"multicluster-engine", "hypershift", "hive"}, MaxLines: 5000},
			{Namespaces: []string{"multicluster-engine", "hive"}, Previous: true, MaxLines: 2000},
		},
	},
	"netobserv": {
		DisplayName: "Network Observability",
		ClusterResources: []ResourceSpec{
			{Group: "flows.netobserv.io", Version: "v1beta2", Resource: "flowcollectors"},
			{Group: "flows.netobserv.io", Version: "v1beta1", Resource: "flowcollectors"},
		},
		NamespacedResources: []ResourceSpec{
			// netobserv and netobserv-privileged are NOT openshift-*, so we need them here
			{Group: "", Version: "v1", Resource: "pods", Namespaces: []string{"netobserv", "netobserv-privileged"}},
			{Group: "", Version: "v1", Resource: "services", Namespaces: []string{"netobserv", "netobserv-privileged"}},
			{Group: "", Version: "v1", Resource: "configmaps", Namespaces: []string{"netobserv", "netobserv-privileged"}},
			{Group: "", Version: "v1", Resource: "events", Namespaces: []string{"netobserv", "netobserv-privileged"}},
			{Group: "apps", Version: "v1", Resource: "deployments", Namespaces: []string{"netobserv"}},
			{Group: "apps", Version: "v1", Resource: "daemonsets", Namespaces: []string{"netobserv-privileged"}},
			// openshift-operators is covered by default
		},
		PodLogs: []PodLogSpec{
			{Namespaces: []string{"netobserv", "netobserv-privileged"}, MaxLines: 5000},
			{Namespaces: []string{"netobserv", "netobserv-privileged"}, Previous: true, MaxLines: 2000},
		},
	},
	"local-storage": {
		DisplayName: "Local Storage Operator",
		ClusterResources: []ResourceSpec{
			{Group: "local.storage.openshift.io", Version: "v1", Resource: "localvolumes"},
			{Group: "local.storage.openshift.io", Version: "v1", Resource: "localvolumesets"},
			{Group: "local.storage.openshift.io", Version: "v1alpha1", Resource: "localvolumediscoveries"},
			{Group: "local.storage.openshift.io", Version: "v1alpha1", Resource: "localvolumediscoveryresults"},
		},
		// openshift-local-storage resources + logs covered by default's openshift-* wildcard
	},
	"sandboxed": {
		DisplayName: "OpenShift Sandboxed Containers",
		ClusterResources: []ResourceSpec{
			{Group: "kataconfiguration.openshift.io", Version: "v1", Resource: "kataconfigs"},
		},
		// openshift-sandboxed-containers-operator resources + logs covered by default's openshift-* wildcard
	},
	"nhc": {
		DisplayName: "Node Health Check",
		ClusterResources: []ResourceSpec{
			{Group: "remediation.medik8s.io", Version: "v1alpha1", Resource: "nodehealthchecks"},
			{Group: "self-node-remediation.medik8s.io", Version: "v1alpha1", Resource: "selfnoderemediations"},
			{Group: "self-node-remediation.medik8s.io", Version: "v1alpha1", Resource: "selfnoderemediationconfigs"},
			{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinehealthchecks"},
		},
		// openshift-workload-availability resources + logs covered by default's openshift-* wildcard
	},
	"numa": {
		DisplayName: "NUMA Resources Operator",
		ClusterResources: []ResourceSpec{
			{Group: "nodetopology.openshift.io", Version: "v2", Resource: "numaresourcesschedulers"},
			{Group: "nodetopology.openshift.io", Version: "v2", Resource: "numaresourcesoperators"},
			{Group: "topology.node.k8s.io", Version: "v1alpha2", Resource: "noderesourcetopologies"},
		},
		// openshift-numaresources resources + logs covered by default's openshift-* wildcard
	},
	"ptp": {
		DisplayName: "PTP Operator",
		ClusterResources: []ResourceSpec{
			{Group: "ptp.openshift.io", Version: "v1", Resource: "ptpconfigs"},
			{Group: "ptp.openshift.io", Version: "v1", Resource: "ptpoperatorconfigs"},
			{Group: "ptp.openshift.io", Version: "v1", Resource: "nodeptpdevices"},
		},
		// openshift-ptp resources + logs covered by default's openshift-* wildcard
	},
	"secrets-store": {
		DisplayName: "Secrets Store CSI Driver",
		ClusterResources: []ResourceSpec{
			{Group: "secrets-store.csi.x-k8s.io", Version: "v1", Resource: "secretproviderclasses"},
			{Group: "secrets-store.csi.x-k8s.io", Version: "v1", Resource: "secretproviderclasspodstatuses"},
		},
		// openshift-cluster-csi-drivers resources + logs covered by default's openshift-* wildcard
	},
	"lvms": {
		DisplayName: "LVM Storage",
		ClusterResources: []ResourceSpec{
			{Group: "lvm.topolvm.io", Version: "v1alpha1", Resource: "lvmclusters"},
			{Group: "lvm.topolvm.io", Version: "v1alpha1", Resource: "lvmvolumegroups"},
			{Group: "lvm.topolvm.io", Version: "v1alpha1", Resource: "lvmvolumegroupnodestatuses"},
			{Group: "topolvm.io", Version: "v1", Resource: "logicalvolumes"},
		},
		// openshift-lvm-storage resources + logs covered by default's openshift-* wildcard
	},
}

// getDefinition returns the definition for a gather type.
// It starts with the built-in definition (if any) and merges in any ConfigMap
// additions. This lets users add custom resources, commands, and log specs
// via ConfigMap without losing the built-in defaults.
func getDefinition(client *k8s.Client, namespace, gatherType string) (*GatherDefinition, error) {
	cmName := "gather-" + gatherType
	if gatherType == "default" {
		cmName = "gather-common"
	}

	builtin, hasBuiltin := builtinDefinitions[gatherType]
	cmDef, cmErr := loadDefinition(client, namespace, cmName)

	if hasBuiltin && cmErr == nil {
		merged := mergeDefinitions(&builtin, cmDef)
		return merged, nil
	}
	if hasBuiltin {
		return &builtin, nil
	}
	if cmErr == nil {
		return cmDef, nil
	}

	return nil, fmt.Errorf("no definition for gather type: %s", gatherType)
}

// mergeDefinitions appends overlay entries onto a copy of base.
func mergeDefinitions(base, overlay *GatherDefinition) *GatherDefinition {
	result := *base
	if overlay.DisplayName != "" {
		result.DisplayName = overlay.DisplayName
	}
	result.ClusterResources = append(result.ClusterResources, overlay.ClusterResources...)
	result.NamespacedResources = append(result.NamespacedResources, overlay.NamespacedResources...)
	result.PodLogs = append(result.PodLogs, overlay.PodLogs...)
	result.PodExecs = append(result.PodExecs, overlay.PodExecs...)
	result.NodeCommands = append(result.NodeCommands, overlay.NodeCommands...)
	return &result
}

// loadDefinition reads a single gather ConfigMap and parses its spec.
func loadDefinition(client *k8s.Client, namespace, configMapName string) (*GatherDefinition, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", namespace, configMapName)
	data, err := client.Get(path)
	if err != nil {
		return nil, fmt.Errorf("read ConfigMap %s: %w", configMapName, err)
	}

	cmData := k8s.JsonMap(data, "data")
	if cmData == nil {
		return nil, fmt.Errorf("ConfigMap %s has no data", configMapName)
	}

	specYAML := k8s.StringOrEmpty(cmData, "spec")
	if specYAML == "" {
		return nil, fmt.Errorf("ConfigMap %s has no 'spec' key in data", configMapName)
	}

	var def GatherDefinition
	if err := yaml.Unmarshal([]byte(specYAML), &def); err != nil {
		return nil, fmt.Errorf("parse ConfigMap %s spec: %w", configMapName, err)
	}

	return &def, nil
}

// loadDefinitions returns the gather definitions needed for a given gather type.
// All definitions are built-in. ConfigMaps can optionally override any definition.
// When skipDefault is true, the default definition is not included (used in multi-select
// where default is collected as a separate type).
func loadDefinitions(client *k8s.Client, namespace, gatherType string, detected map[string]bool, skipDefault bool) ([]GatherDefinition, error) {
	var defs []GatherDefinition

	// Include the default/common definition unless skipped
	if !skipDefault {
		common, err := getDefinition(client, namespace, "default")
		if err != nil {
			return nil, fmt.Errorf("default gather definition: %w", err)
		}
		defs = append(defs, *common)
	}

	if gatherType == "all" {
		// Add all detected operator definitions
		for typ := range builtinDefinitions {
			if typ == "default" || typ == "audit" {
				continue
			}
			if !detected[typ] {
				continue
			}
			def, err := getDefinition(client, namespace, typ)
			if err != nil {
				log.Printf("Skipping %s gather: %v", typ, err)
				continue
			}
			defs = append(defs, *def)
		}
		// Always include audit for gather-all
		audit, _ := getDefinition(client, namespace, "audit")
		if audit != nil {
			defs = append(defs, *audit)
		}
	} else if gatherType == "audit" {
		def, err := getDefinition(client, namespace, "audit")
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	} else if gatherType != "default" {
		def, err := getDefinition(client, namespace, gatherType)
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	}

	return defs, nil
}
