package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"regexp"

	"github.com/redhat-consulting-services/ocp-support-web/internal/monitoring"
	"github.com/redhat-consulting-services/ocp-support-web/internal/mustgather"
	"github.com/redhat-consulting-services/ocp-support-web/internal/status"
)

var validJobID = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

type Handler struct {
	mg      *mustgather.Manager
	st      *status.Client
	mon     *monitoring.Client
	tmpl    *template.Template
	static  fs.FS
	version string
}

func New(mg *mustgather.Manager, st *status.Client, mon *monitoring.Client, webFS fs.FS, version string) (*Handler, error) {
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
	mux.HandleFunc("GET /api/support/jobs", h.handleListGatherJobs)
	mux.HandleFunc("POST /api/support/gather", h.handleStartGather)
	mux.HandleFunc("GET /api/support/gather/{jobId}", h.handleGatherStatus)
	mux.HandleFunc("GET /api/support/gather/{jobId}/download", h.handleGatherDownload)
	mux.HandleFunc("POST /api/support/etcd-diag", h.handleStartDiag)
	mux.HandleFunc("GET /api/support/etcd-diag/{jobId}", h.handleDiagStatus)

	if h.st != nil {
		mux.HandleFunc("GET /api/support/cluster-id", h.handleClusterID)
		mux.HandleFunc("GET /api/support/capabilities", h.handleCapabilities)
		mux.HandleFunc("GET /api/status/cluster", h.handleClusterHealth)
		mux.HandleFunc("GET /api/status/nodes", h.handleNodeUtilization)
		mux.HandleFunc("GET /api/status/top", h.handleTopConsumers)
		mux.HandleFunc("GET /api/status/networks", h.handleNetworks)
		mux.HandleFunc("GET /api/status/storageclasses", h.handleStorageClasses)
		if h.mon != nil {
			mux.HandleFunc("GET /api/status/etcd", h.handleEtcdHealth)
		}
	}

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

func (h *Handler) handleListGatherJobs(w http.ResponseWriter, r *http.Request) {
	jobs := h.mg.ListJobs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (h *Handler) handleStartGather(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type      string `json:"type"`
		Anonymize bool   `json:"anonymize"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	gatherType := mustgather.GatherType(req.Type)
	switch gatherType {
	case mustgather.GatherDefault, mustgather.GatherVirtualization, mustgather.GatherODF,
		mustgather.GatherAudit, mustgather.GatherAll, mustgather.GatherEtcdBackup:
		// valid
	default:
		jsonError(w, "invalid gather type", 400)
		return
	}

	id := h.mg.StartGather(gatherType, req.Anonymize)
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

	if (req.Type == "creation-timeline" || req.Type == "ns-object-counts") && req.ObjectType == "" {
		jsonError(w, "objectType is required for this diagnostic", 400)
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

	leaderData, err := h.mon.Query("etcd_server_is_leader")
	if err != nil {
		log.Printf("etcd leader query error: %v", err)
		jsonError(w, "failed to query etcd leader", 500)
		return
	}
	revisionData, err := h.mon.Query("etcd_debugging_mvcc_current_revision")
	if err != nil {
		log.Printf("etcd revision query error: %v", err)
		jsonError(w, "failed to query etcd revision", 500)
		return
	}
	sizeData, err := h.mon.Query("etcd_mvcc_db_total_size_in_bytes")
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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
