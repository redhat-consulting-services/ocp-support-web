package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/redhat-consulting-services/ocp-support-web/internal/acm"
	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"github.com/redhat-consulting-services/ocp-support-web/internal/monitoring"
	"github.com/redhat-consulting-services/ocp-support-web/internal/mustgather"
	"github.com/redhat-consulting-services/ocp-support-web/internal/status"
)

var validJobID = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
var validNodeName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
var validNodeSelector = regexp.MustCompile(`^[a-zA-Z0-9./_=-]+(?:,[a-zA-Z0-9./_=-]+)*$`)
var validNamespace = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type Handler struct {
	mg      *mustgather.Manager
	st      *status.Client
	mon     *monitoring.Client
	acm     *acm.Client
	k8s     *k8s.Client
	tmpl    *template.Template
	static  fs.FS
	version string
}

func New(mg *mustgather.Manager, st *status.Client, mon *monitoring.Client, acmClient *acm.Client, k8sClient *k8s.Client, webFS fs.FS, version string) (*Handler, error) {
	tmplFS, err := fs.Sub(webFS, "templates")
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, err
	}

	tmpl, err := template.ParseFS(tmplFS, "*.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		mg:      mg,
		st:      st,
		mon:     mon,
		acm:     acmClient,
		k8s:     k8sClient,
		tmpl:    tmpl,
		static:  staticFS,
		version: version,
	}, nil
}

type pageVars struct {
	Username string
	Version  string
}

func (h *Handler) getPageVars(r *http.Request) pageVars {
	return pageVars{
		Username: r.Header.Get("X-Forwarded-User"),
		Version:  h.version,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /", h.handleSupportPage)
	mux.HandleFunc("GET /status", h.handleStatusPage)
	mux.HandleFunc("GET /advanced", h.handleAdvancedPage)
	mux.HandleFunc("GET /api/support/namespaces", h.handleNamespaces)
	mux.HandleFunc("GET /api/support/jobs", h.handleListGatherJobs)
	mux.HandleFunc("POST /api/support/gather", h.handleStartGather)
	mux.HandleFunc("GET /api/support/gather/{jobId}", h.handleGatherStatus)
	mux.HandleFunc("POST /api/support/gather/{jobId}/stop", h.handleStopGather)
	mux.HandleFunc("GET /api/support/gather/{jobId}/download", h.handleGatherDownload)
	mux.HandleFunc("POST /api/support/etcd-diag", h.handleStartDiag)
	mux.HandleFunc("GET /api/support/etcd-diag/{jobId}", h.handleDiagStatus)

	if h.st != nil {
		mux.HandleFunc("GET /api/support/cluster-id", h.handleClusterID)
		mux.HandleFunc("GET /api/support/capabilities", h.handleCapabilities)
		mux.HandleFunc("GET /api/support/nodes", h.handleNodes)
		mux.HandleFunc("GET /api/status/cluster", h.handleClusterHealth)
		mux.HandleFunc("GET /api/status/nodes", h.handleNodeUtilization)
		mux.HandleFunc("GET /api/status/top", h.handleTopConsumers)
		mux.HandleFunc("GET /api/status/networks", h.handleNetworks)
		mux.HandleFunc("GET /api/status/storageclasses", h.handleStorageClasses)
		mux.HandleFunc("GET /api/status/gpus", h.handleGPUNodes)
		if h.mon != nil {
			mux.HandleFunc("GET /api/status/etcd", h.handleEtcdHealth)
		}
	}

	mux.HandleFunc("GET /resources", h.handleResourcesPage)
	mux.HandleFunc("GET /api/resources/apiresources", h.handleAPIResources)
	mux.HandleFunc("GET /api/resources/ns", h.handleNamespaceResources)
	mux.HandleFunc("GET /api/resources/list", h.handleListNamespacedResources)
	mux.HandleFunc("GET /api/resources/get", h.handleGetResource)

	mux.HandleFunc("GET /acm", h.handleACMPage)
	mux.HandleFunc("GET /api/acm/clusters", h.handleACMClusters)
	mux.HandleFunc("GET /api/acm/clusters/{cluster}/namespaces", h.handleClusterNamespaces)
	mux.HandleFunc("GET /api/acm/clusters/{cluster}/operators", h.handleClusterOperators)
	mux.HandleFunc("GET /api/acm/clusters/{cluster}/version", h.handleClusterAgentVersion)
	mux.HandleFunc("GET /api/acm/gather/images", h.handleACMGatherImages)
	mux.HandleFunc("POST /api/acm/gather", h.handleStartRemoteGather)
	mux.HandleFunc("GET /api/acm/gather/jobs", h.handleListRemoteGatherJobs)
	mux.HandleFunc("GET /api/acm/gather/{jobId}", h.handleRemoteGatherStatus)
	mux.HandleFunc("GET /api/acm/gather/{jobId}/download", h.handleRemoteGatherDownload)
	mux.HandleFunc("DELETE /api/acm/gather/{jobId}", h.handleDeleteRemoteGather)
	mux.HandleFunc("POST /api/acm/clusters/{cluster}/agent", h.handleDeployAgent)
	mux.HandleFunc("DELETE /api/acm/clusters/{cluster}/agent", h.handleRemoveAgent)
	mux.HandleFunc("POST /api/acm/clusters/{cluster}/agent/redeploy", h.handleRedeployAgent)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(h.static)))
}

func (h *Handler) handleSupportPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := h.tmpl.ExecuteTemplate(w, "support.html", h.getPageVars(r)); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func (h *Handler) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	if err := h.tmpl.ExecuteTemplate(w, "status.html", h.getPageVars(r)); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func (h *Handler) handleAdvancedPage(w http.ResponseWriter, r *http.Request) {
	if err := h.tmpl.ExecuteTemplate(w, "advanced.html", h.getPageVars(r)); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func (h *Handler) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := h.st.GetNamespaces()
	if err != nil {
		log.Printf("namespaces error: %v", err)
		jsonError(w, "failed to list namespaces", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(namespaces)
}

func (h *Handler) handleListGatherJobs(w http.ResponseWriter, r *http.Request) {
	jobs := h.mg.ListJobs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// allowedResourceTypes is the whitelist of resource types for custom gather.
var allowedResourceTypes = map[string]bool{
	"pods": true, "services": true, "configmaps": true, "events": true,
	"deployments": true, "statefulsets": true, "daemonsets": true,
	"jobs": true, "cronjobs": true, "replicasets": true,
	"persistentvolumeclaims": true, "serviceaccounts": true,
	"roles": true, "rolebindings": true, "routes": true,
	"ingresses": true, "endpointslices": true, "networkpolicies": true,
	"horizontalpodautoscalers": true, "secrets": true,
}

func (h *Handler) handleStartGather(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type         string   `json:"type"`
		Types        []string `json:"types"`
		Anonymize    bool     `json:"anonymize"`
		AnonOpts     struct {
			IPs      bool `json:"ips"`
			MACs     bool `json:"macs"`
			Domains  bool `json:"domains"`
			Services bool `json:"services"`
			Secrets  bool `json:"secrets"`
		} `json:"anonOpts"`
		Since         string   `json:"since"`
		NodeName      string   `json:"nodeName"`
		NodeSelector  string   `json:"nodeSelector"`
		HostNetwork   bool     `json:"hostNetwork"`
		Namespaces    []string `json:"namespaces"`
		ResourceTypes []string `json:"resourceTypes"`
		IncludeLogs   bool     `json:"includeLogs"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	validTypes := map[mustgather.GatherType]bool{
		mustgather.GatherDefault: true, mustgather.GatherVirtualization: true,
		mustgather.GatherODF: true, mustgather.GatherACM: true,
		mustgather.GatherLogging: true, mustgather.GatherServiceMesh: true,
		mustgather.GatherCompliance: true, mustgather.GatherMTC: true,
		mustgather.GatherGitOps: true, mustgather.GatherServerless: true,
		mustgather.GatherMCE: true, mustgather.GatherNetObserv: true,
		mustgather.GatherLocalStorage: true, mustgather.GatherSandboxed: true,
		mustgather.GatherNHC: true, mustgather.GatherNUMA: true,
		mustgather.GatherPTP: true, mustgather.GatherSecretsStore: true,
		mustgather.GatherLVMS: true, mustgather.GatherAudit: true,
		mustgather.GatherAll: true, mustgather.GatherEtcdBackup: true,
		mustgather.GatherCustom: true,
	}

	// Multi-select: if types array is provided with >1 entry, use GatherMulti
	var gatherType mustgather.GatherType
	var selectedTypes []mustgather.GatherType
	if len(req.Types) > 1 {
		gatherType = mustgather.GatherMulti
		for _, t := range req.Types {
			gt := mustgather.GatherType(t)
			if !validTypes[gt] {
				jsonError(w, fmt.Sprintf("invalid gather type: %s", t), 400)
				return
			}
			selectedTypes = append(selectedTypes, gt)
		}
	} else {
		typStr := req.Type
		if len(req.Types) == 1 {
			typStr = req.Types[0]
		}
		gatherType = mustgather.GatherType(typStr)
		if !validTypes[gatherType] {
			jsonError(w, "invalid gather type", 400)
			return
		}
	}

	if req.NodeName != "" && !validNodeName.MatchString(req.NodeName) {
		jsonError(w, "invalid node name", 400)
		return
	}
	if req.NodeSelector != "" && !validNodeSelector.MatchString(req.NodeSelector) {
		jsonError(w, "invalid node selector", 400)
		return
	}
	if req.NodeName != "" && req.NodeSelector != "" {
		jsonError(w, "node name and node selector cannot be used together", 400)
		return
	}

	opts := mustgather.GatherOpts{
		NodeName:      req.NodeName,
		NodeSelector:  req.NodeSelector,
		HostNetwork:   req.HostNetwork,
		SelectedTypes: selectedTypes,
	}

	// Validate and attach custom gather options
	if gatherType == mustgather.GatherCustom {
		if len(req.Namespaces) == 0 {
			jsonError(w, "at least one namespace is required", 400)
			return
		}
		if len(req.Namespaces) > 50 {
			jsonError(w, "maximum 50 namespaces allowed", 400)
			return
		}
		for _, ns := range req.Namespaces {
			if !validNamespace.MatchString(ns) {
				jsonError(w, fmt.Sprintf("invalid namespace name: %s", ns), 400)
				return
			}
		}
		for _, rt := range req.ResourceTypes {
			if !allowedResourceTypes[rt] {
				jsonError(w, fmt.Sprintf("invalid resource type: %s", rt), 400)
				return
			}
		}
		opts.Custom = &mustgather.CustomGatherOpts{
			Namespaces:    req.Namespaces,
			ResourceTypes: req.ResourceTypes,
			IncludeLogs:   req.IncludeLogs,
		}
	}

	if h.mg.ActiveJobCount() >= 5 {
		jsonError(w, "too many active jobs, please wait for existing jobs to complete", 429)
		return
	}

	anonOpts := mustgather.AnonOptions{
		IPs:      req.AnonOpts.IPs,
		MACs:     req.AnonOpts.MACs,
		Domains:  req.AnonOpts.Domains,
		Services: req.AnonOpts.Services,
		Secrets:  req.AnonOpts.Secrets,
	}
	if req.Anonymize && !anonOpts.Any() {
		anonOpts = mustgather.AnonOptions{IPs: true, MACs: true, Domains: true, Services: true}
	}
	id := h.mg.StartGather(gatherType, anonOpts, req.Since, opts)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "running"})
}

func (h *Handler) handleGatherStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}
	job := h.mg.GetJob(jobID)
	if job == nil {
		jsonError(w, "job not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) handleStopGather(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}
	if !h.mg.StopJob(jobID) {
		jsonError(w, "job not found or already finished", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

func (h *Handler) handleGatherDownload(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}
	filePath := h.mg.GetFilePath(jobID)
	if filePath == "" {
		jsonError(w, "file not available", 404)
		return
	}

	job := h.mg.GetJob(jobID)
	fileName := jobID + ".tar.gz"
	if job != nil && job.FileName != "" {
		fileName = job.FileName
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, filePath)
}

func (h *Handler) handleStartDiag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type       string `json:"type"`
		ObjectType string `json:"objectType"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	validTypes := map[string]bool{
		"object-sizes": true, "object-counts": true, "ns-breakdown": true,
		"creation-timeline": true, "ns-object-counts": true,
	}
	if !validTypes[req.Type] {
		jsonError(w, "invalid diagnostic type", 400)
		return
	}

	if (req.Type == "creation-timeline" || req.Type == "ns-object-counts") && !mustgather.AllowedDiagObjects[req.ObjectType] {
		jsonError(w, "invalid or unsupported resource type", 400)
		return
	}

	id := h.mg.StartDiag(req.Type, req.ObjectType)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "running"})
}

func (h *Handler) handleDiagStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}
	dj := h.mg.GetDiagJob(jobID)
	if dj == nil {
		jsonError(w, "job not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dj)
}

func (h *Handler) handleClusterID(w http.ResponseWriter, r *http.Request) {
	id, err := h.st.GetClusterID()
	if err != nil {
		log.Printf("cluster ID error: %v", err)
		jsonError(w, "failed to get cluster ID", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"clusterID": id})
}

func (h *Handler) handleClusterHealth(w http.ResponseWriter, r *http.Request) {
	health, err := h.st.GetClusterHealth()
	if err != nil {
		log.Printf("cluster health error: %v", err)
		jsonError(w, "failed to get cluster health", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (h *Handler) handleNodeUtilization(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.st.GetNodeUtilization()
	if err != nil {
		log.Printf("node utilization error: %v", err)
		jsonError(w, "failed to get node utilization", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func (h *Handler) handleGPUNodes(w http.ResponseWriter, r *http.Request) {
	gpus, err := h.st.GetGPUNodes()
	if err != nil {
		log.Printf("gpu nodes error: %v", err)
		jsonError(w, "failed to get GPU nodes", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gpus)
}

func (h *Handler) handleTopConsumers(w http.ResponseWriter, r *http.Request) {
	top, err := h.st.GetTopConsumers(10)
	if err != nil {
		log.Printf("top consumers error: %v", err)
		jsonError(w, "failed to get top consumers", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(top)
}

func (h *Handler) handleEtcdHealth(w http.ResponseWriter, r *http.Request) {
	result := status.EtcdHealth{Healthy: true}

	leaderData, err := h.mon.Query(`etcd_server_is_leader{namespace="openshift-etcd"}`)
	if err != nil {
		log.Printf("etcd leader query error: %v", err)
		jsonError(w, "failed to query etcd leader", 500)
		return
	}
	revisionData, err := h.mon.Query(`etcd_debugging_mvcc_current_revision{namespace="openshift-etcd"}`)
	if err != nil {
		log.Printf("etcd revision query error: %v", err)
		jsonError(w, "failed to query etcd revision", 500)
		return
	}
	sizeData, err := h.mon.Query(`etcd_mvcc_db_total_size_in_bytes{namespace="openshift-etcd"}`)
	if err != nil {
		log.Printf("etcd db size query error: %v", err)
		jsonError(w, "failed to query etcd db size", 500)
		return
	}

	type promResult struct {
		Metric map[string]string `json:"metric"`
		Value  []interface{}     `json:"value"`
	}
	type promData struct {
		ResultType string       `json:"resultType"`
		Result     []promResult `json:"result"`
	}

	parsePromData := func(raw json.RawMessage) (*promData, error) {
		var d promData
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		return &d, nil
	}

	getFloat := func(val []interface{}) float64 {
		if len(val) < 2 {
			return 0
		}
		if s, ok := val[1].(string); ok {
			var f float64
			fmt.Sscanf(s, "%f", &f)
			return f
		}
		return 0
	}

	members := map[string]*status.EtcdMember{}

	leaderParsed, err := parsePromData(leaderData)
	if err == nil {
		for _, r := range leaderParsed.Result {
			pod := r.Metric["pod"]
			if pod == "" {
				continue
			}
			members[pod] = &status.EtcdMember{
				Pod:      pod,
				Name:     pod,
				IsLeader: getFloat(r.Value) == 1,
			}
		}
	}

	revParsed, err := parsePromData(revisionData)
	if err == nil {
		for _, r := range revParsed.Result {
			pod := r.Metric["pod"]
			if pod == "" {
				continue
			}
			if m, ok := members[pod]; ok {
				m.Revision = int64(getFloat(r.Value))
			} else {
				members[pod] = &status.EtcdMember{Pod: pod, Name: pod, Revision: int64(getFloat(r.Value))}
			}
		}
	}

	sizeParsed, err := parsePromData(sizeData)
	if err == nil {
		for _, r := range sizeParsed.Result {
			pod := r.Metric["pod"]
			if pod == "" {
				continue
			}
			if m, ok := members[pod]; ok {
				m.DBSizeMB = getFloat(r.Value) / (1024 * 1024)
			} else {
				members[pod] = &status.EtcdMember{Pod: pod, Name: pod, DBSizeMB: getFloat(r.Value) / (1024 * 1024)}
			}
		}
	}

	for _, m := range members {
		result.Members = append(result.Members, *m)
	}

	hasLeader := false
	for _, m := range result.Members {
		if m.IsLeader {
			hasLeader = true
			break
		}
	}
	if !hasLeader || len(result.Members) == 0 {
		result.Healthy = false
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	caps := h.st.GetCapabilities()
	if caps.CNV {
		h.mg.SetDetected(mustgather.GatherVirtualization)
		if caps.CNVVersion != "" {
			h.mg.SetImage("cnv", "registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v"+caps.CNVVersion)
		}
	}
	if caps.ODF {
		h.mg.SetDetected(mustgather.GatherODF)
		if caps.ODFVersion != "" {
			h.mg.SetImage("odf", "registry.redhat.io/odf4/odf-must-gather-rhel9:v"+majorMinor(caps.ODFVersion))
		}
	}
	if caps.ACM {
		h.mg.SetDetected(mustgather.GatherACM)
		if caps.ACMVersion != "" {
			acmRhel := "rhel9"
			if versionLessThan(caps.ACMVersion, "2.10") {
				acmRhel = "rhel8"
			}
			h.mg.SetImage("acm", "registry.redhat.io/rhacm2/acm-must-gather-"+acmRhel+":v"+caps.ACMVersion)
		}
	}
	if caps.Logging {
		h.mg.SetDetected(mustgather.GatherLogging)
		if caps.LoggingVersion != "" {
			h.mg.SetImage("logging", "registry.redhat.io/openshift-logging/cluster-logging-rhel9-operator:v"+caps.LoggingVersion)
		}
	}
	if caps.ServiceMesh {
		h.mg.SetDetected(mustgather.GatherServiceMesh)
		if caps.ServiceMeshVersion != "" {
			h.mg.SetImage("service-mesh", "registry.redhat.io/openshift-service-mesh/istio-must-gather-rhel8:v"+caps.ServiceMeshVersion)
		}
	}
	if caps.Compliance {
		h.mg.SetDetected(mustgather.GatherCompliance)
	}
	if caps.MTC {
		h.mg.SetDetected(mustgather.GatherMTC)
		if caps.MTCVersion != "" {
			h.mg.SetImage("mtc", "registry.redhat.io/rhmtc/openshift-migration-must-gather-rhel8:v"+caps.MTCVersion)
		}
	}
	if caps.GitOps {
		h.mg.SetDetected(mustgather.GatherGitOps)
		if caps.GitOpsVersion != "" {
			h.mg.SetImage("gitops", "registry.redhat.io/openshift-gitops-1/must-gather-rhel8:v"+caps.GitOpsVersion)
		}
	}
	if caps.Serverless {
		h.mg.SetDetected(mustgather.GatherServerless)
		if caps.ServerlessVersion != "" {
			h.mg.SetImage("serverless", "registry.redhat.io/openshift-serverless-1/svls-must-gather-rhel8:v"+caps.ServerlessVersion)
		}
	}
	if caps.MCE {
		h.mg.SetDetected(mustgather.GatherMCE)
		if caps.MCEVersion != "" {
			mceRhel := "rhel9"
			if versionLessThan(caps.MCEVersion, "2.7") {
				mceRhel = "rhel8"
			}
			h.mg.SetImage("mce", "registry.redhat.io/multicluster-engine/must-gather-"+mceRhel+":v"+caps.MCEVersion)
		}
	}
	if caps.NetObserv {
		h.mg.SetDetected(mustgather.GatherNetObserv)
	}
	if caps.LocalStorage {
		h.mg.SetDetected(mustgather.GatherLocalStorage)
		if caps.LocalStorageVersion != "" {
			h.mg.SetImage("local-storage", "registry.redhat.io/openshift4/ose-local-storage-mustgather-rhel9:v"+caps.LocalStorageVersion)
		}
	}
	if caps.Sandboxed {
		h.mg.SetDetected(mustgather.GatherSandboxed)
		if caps.SandboxedVersion != "" {
			h.mg.SetImage("sandboxed", "registry.redhat.io/openshift-sandboxed-containers/osc-must-gather-rhel8:v"+caps.SandboxedVersion)
		}
	}
	if caps.NHC {
		h.mg.SetDetected(mustgather.GatherNHC)
		if caps.NHCVersion != "" {
			h.mg.SetImage("nhc", "registry.redhat.io/workload-availability/node-healthcheck-must-gather-rhel9:v"+caps.NHCVersion)
		}
	}
	if caps.NUMA {
		h.mg.SetDetected(mustgather.GatherNUMA)
		if caps.NUMAVersion != "" {
			h.mg.SetImage("numa", "registry.redhat.io/numaresources/numaresources-must-gather-rhel9:v"+caps.NUMAVersion)
		}
	}
	if caps.PTP {
		h.mg.SetDetected(mustgather.GatherPTP)
		if caps.PTPVersion != "" {
			h.mg.SetImage("ptp", "registry.redhat.io/openshift4/ptp-must-gather-rhel8:v"+caps.PTPVersion)
		}
	}
	if caps.SecretsStore {
		h.mg.SetDetected(mustgather.GatherSecretsStore)
		if caps.SecretsStoreVersion != "" {
			h.mg.SetImage("secrets-store", "registry.redhat.io/openshift4/ose-secrets-store-csi-mustgather-rhel9:v"+caps.SecretsStoreVersion)
		}
	}
	if caps.LVMS {
		h.mg.SetDetected(mustgather.GatherLVMS)
		if caps.LVMSVersion != "" {
			h.mg.SetImage("lvms", "registry.redhat.io/lvms4/lvms-must-gather-rhel9:v"+caps.LVMSVersion)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(caps)
}

func (h *Handler) handleNetworks(w http.ResponseWriter, r *http.Request) {
	if !h.st.IsNMStateInstalled() {
		jsonError(w, "NMState not installed", 404)
		return
	}
	networks, err := h.st.GetNMStateNetworks()
	if err != nil {
		log.Printf("nmstate networks error: %v", err)
		jsonError(w, "NMState not available", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networks)
}

func (h *Handler) handleStorageClasses(w http.ResponseWriter, r *http.Request) {
	scs, err := h.st.GetStorageClasses()
	if err != nil {
		log.Printf("storage classes error: %v", err)
		jsonError(w, "failed to get storage classes", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scs)
}

func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.st.GetNodes()
	if err != nil {
		jsonError(w, "failed to get nodes", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// majorMinor extracts "X.Y" from version strings like "4.20.7-rhodf".
func majorMinor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

// versionLessThan returns true if version "a.b[.c]" is less than "x.y".
func versionLessThan(version, threshold string) bool {
	parse := func(v string) (int, int) {
		parts := strings.SplitN(v, ".", 3)
		major, _ := strconv.Atoi(parts[0])
		minor := 0
		if len(parts) >= 2 {
			minor, _ = strconv.Atoi(parts[1])
		}
		return major, minor
	}
	aMajor, aMinor := parse(version)
	bMajor, bMinor := parse(threshold)
	return aMajor < bMajor || (aMajor == bMajor && aMinor < bMinor)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
