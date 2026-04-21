package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/redhat-consulting-services/ocp-support-web/internal/acm"
	"github.com/redhat-consulting-services/ocp-support-web/internal/mustgather"
)

func (h *Handler) handleACMPage(w http.ResponseWriter, r *http.Request) {
	if err := h.tmpl.ExecuteTemplate(w, "acm.html", h.getPageVars(r)); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func (h *Handler) handleACMClusters(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusters, err := h.acm.ListManagedClusters()
	if err != nil {
		log.Printf("ACM clusters error: %v", err)
		jsonError(w, "failed to list managed clusters", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clusters)
}

func (h *Handler) handleClusterOperators(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusterName := r.PathValue("cluster")
	if !validNamespace.MatchString(clusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}

	data, err := h.acm.GetClusterOperatorsRaw(clusterName)
	if err != nil {
		log.Printf("ACM operators error for %s: %v", clusterName, err)
		jsonError(w, "failed to get cluster operators", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *Handler) handleClusterNamespaces(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusterName := r.PathValue("cluster")
	if !validNamespace.MatchString(clusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}

	data, err := h.acm.GetClusterNamespacesRaw(clusterName)
	if err != nil {
		log.Printf("ACM namespaces error for %s: %v", clusterName, err)
		jsonError(w, "failed to get cluster namespaces", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *Handler) handleClusterAgentVersion(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusterName := r.PathValue("cluster")
	if !validNamespace.MatchString(clusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}

	ver, err := h.acm.GetClusterAgentVersion(clusterName)
	if err != nil {
		log.Printf("ACM agent version error for %s: %v", clusterName, err)
		jsonError(w, "failed to get agent version", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": ver})
}

func (h *Handler) handleDeployAgent(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusterName := r.PathValue("cluster")
	if !validNamespace.MatchString(clusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}
	if err := h.acm.DeployAgent(clusterName); err != nil {
		log.Printf("Deploy agent error for %s: %v", clusterName, err)
		jsonError(w, "failed to deploy agent", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deployed"})
}

func (h *Handler) handleRemoveAgent(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusterName := r.PathValue("cluster")
	if !validNamespace.MatchString(clusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}
	if err := h.acm.RemoveAgent(clusterName); err != nil {
		log.Printf("Remove agent error for %s: %v", clusterName, err)
		jsonError(w, "failed to remove agent", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
}

func (h *Handler) handleRedeployAgent(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	clusterName := r.PathValue("cluster")
	if !validNamespace.MatchString(clusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}
	if err := h.acm.RedeployAgent(clusterName); err != nil {
		log.Printf("Redeploy agent error for %s: %v", clusterName, err)
		jsonError(w, "failed to redeploy agent", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "redeployed"})
}

func (h *Handler) handleStartRemoteGather(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	var req struct {
		ClusterName   string                 `json:"clusterName"`
		GatherTypes   []string               `json:"gatherTypes"`
		Since         string                 `json:"since"`
		Anonymize     bool                   `json:"anonymize"`
		AnonOpts      mustgather.AnonOptions `json:"anonOpts"`
		Namespaces    []string               `json:"namespaces"`
		ResourceTypes []string               `json:"resourceTypes"`
		IncludeLogs   bool                   `json:"includeLogs"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}
	if req.ClusterName == "" {
		jsonError(w, "clusterName is required", 400)
		return
	}
	if !validNamespace.MatchString(req.ClusterName) {
		jsonError(w, "invalid cluster name", 400)
		return
	}
	if len(req.GatherTypes) == 0 {
		req.GatherTypes = []string{"default"}
	}

	// Validate gather types
	validRemoteTypes := map[string]bool{
		"default": true, "virtualization": true, "odf": true, "acm": true,
		"logging": true, "service-mesh": true, "compliance": true, "mtc": true,
		"gitops": true, "serverless": true, "mce": true, "netobserv": true,
		"local-storage": true, "sandboxed": true, "nhc": true, "numa": true,
		"ptp": true, "secrets-store": true, "lvms": true, "audit": true,
		"all": true, "custom": true,
	}
	for _, gt := range req.GatherTypes {
		if !validRemoteTypes[gt] {
			jsonError(w, fmt.Sprintf("invalid gather type: %s", gt), 400)
			return
		}
	}

	// Validate since parameter
	if req.Since != "" {
		validSince := regexp.MustCompile(`^[0-9]+h$`)
		if !validSince.MatchString(req.Since) {
			jsonError(w, "invalid since format (expected e.g. '48h')", 400)
			return
		}
	}

	// Validate custom namespaces
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

	id, err := h.acm.StartRemoteGather(req.ClusterName, req.GatherTypes, req.Since, req.Anonymize, req.AnonOpts, req.Namespaces, req.ResourceTypes, req.IncludeLogs)
	if err != nil {
		log.Printf("ACM remote gather error: %v", err)
		jsonError(w, "failed to start remote gather", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "gathering"})
}

func (h *Handler) handleACMGatherImages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(acm.ListGatherImages())
}

func (h *Handler) handleListRemoteGatherJobs(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	jobs := h.acm.ListJobs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (h *Handler) handleRemoteGatherStatus(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}
	job := h.acm.GetJob(jobID)
	if job == nil {
		jsonError(w, "job not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) handleRemoteGatherDownload(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}

	filePath := h.acm.GetFilePath(jobID)
	if filePath == "" {
		jsonError(w, "file not available", 404)
		return
	}

	job := h.acm.GetJob(jobID)
	fileName := jobID + ".tar.gz"
	if job != nil && job.FileName != "" {
		fileName = job.FileName
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, filePath)
}

func (h *Handler) handleDeleteRemoteGather(w http.ResponseWriter, r *http.Request) {
	if h.acm == nil {
		jsonError(w, "ACM not available", 404)
		return
	}
	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}
	if err := h.acm.DeleteJob(jobID); err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
