package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/collector"
	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

// GatherRequest is the JSON body for POST /gather.
type GatherRequest struct {
	GatherTypes   []string `json:"gatherTypes"`
	Since         string   `json:"since,omitempty"`
	ClusterName   string   `json:"clusterName,omitempty"`
	Namespaces    []string `json:"namespaces,omitempty"`
	ResourceTypes []string `json:"resourceTypes,omitempty"`
	IncludeLogs   bool     `json:"includeLogs,omitempty"`
}

// GatherStatus represents the agent's current state.
type GatherStatus struct {
	Status    string `json:"status"` // idle, gathering, serving, error
	StartedAt string `json:"startedAt,omitempty"`
	Error     string `json:"error,omitempty"`
	LogOutput string `json:"logOutput,omitempty"`
	FileName  string `json:"fileName,omitempty"`
	Version   string `json:"version,omitempty"`
}

// DetectedOperator is returned by GET /operators.
type DetectedOperator struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Version string `json:"version,omitempty"`
}

// Agent runs on managed clusters to handle must-gather collection.
type Agent struct {
	k8sClient *k8s.Client
	engine    *collector.Engine
	workDir   string
	version   string

	mu        sync.Mutex
	status    string // idle, gathering, serving, error
	startedAt time.Time
	gatherErr string
	logBuf    strings.Builder
	archivePath string

	operators     []DetectedOperator
	operatorsMu   sync.RWMutex
	detected      map[string]bool
}

var validAgentGatherTypes = map[string]bool{
	"default": true, "all": true, "custom": true,
	"virtualization": true, "odf": true, "acm": true,
	"logging": true, "service-mesh": true, "compliance": true,
	"mtc": true, "gitops": true, "serverless": true,
	"mce": true, "netobserv": true, "local-storage": true,
	"sandboxed": true, "nhc": true, "numa": true,
	"ptp": true, "secrets-store": true, "lvms": true,
	"audit": true,
}

// Run starts the agent. This blocks forever.
func Run(version string) {
	apiURL := os.Getenv("KUBERNETES_SERVICE_HOST")
	apiPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if apiURL == "" || apiPort == "" {
		log.Fatal("Agent must run in-cluster (KUBERNETES_SERVICE_HOST/PORT not set)")
	}

	fullURL := fmt.Sprintf("https://%s:%s", apiURL, apiPort)
	token := ""
	if tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		token = strings.TrimSpace(string(tokenBytes))
	}
	if token == "" {
		log.Fatal("Agent requires a service account token")
	}

	k8sClient := k8s.NewClient(fullURL, token, true)

	// Use the agent's own namespace for creating debug pods
	agentNS := "ocp-support-web-agent"
	if nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		agentNS = strings.TrimSpace(string(nsBytes))
	}
	engine := collector.NewEngine(k8sClient, agentNS)

	workDir := "/tmp/ocp-support-agent"
	os.MkdirAll(workDir, 0700)

	a := &Agent{
		k8sClient: k8sClient,
		engine:    engine,
		workDir:   workDir,
		version:   version,
		status:    "idle",
		detected:  make(map[string]bool),
	}

	// Detect operators on startup
	a.detectOperators()

	// Periodically refresh operator detection
	go func() {
		for range time.Tick(5 * time.Minute) {
			a.detectOperators()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/operators", a.handleOperators)
	mux.HandleFunc("/status", a.handleStatus)
	mux.HandleFunc("/gather", a.handleGather)
	mux.HandleFunc("/download", a.handleDownload)
	mux.HandleFunc("/version", a.handleVersion)
	mux.HandleFunc("/namespaces", a.handleNamespaces)

	log.Printf("Agent listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Agent server error: %v", err)
	}
}

func (a *Agent) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (a *Agent) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": a.version})
}

func (a *Agent) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	data, err := a.k8sClient.Get("/api/v1/namespaces")
	if err != nil {
		http.Error(w, `{"error":"failed to list namespaces"}`, 500)
		return
	}
	items, _ := data["items"].([]interface{})
	var names []string
	for _, item := range items {
		if ns, ok := item.(map[string]interface{}); ok {
			if meta, ok := ns["metadata"].(map[string]interface{}); ok {
				if name, ok := meta["name"].(string); ok {
					names = append(names, name)
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

func (a *Agent) handleOperators(w http.ResponseWriter, r *http.Request) {
	a.operatorsMu.RLock()
	ops := a.operators
	a.operatorsMu.RUnlock()

	if ops == nil {
		ops = []DetectedOperator{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ops)
}

func (a *Agent) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	s := GatherStatus{
		Status:    a.status,
		Error:     a.gatherErr,
		LogOutput: a.logBuf.String(),
		Version:   a.version,
	}
	if !a.startedAt.IsZero() {
		s.StartedAt = a.startedAt.Format(time.RFC3339)
	}
	if a.archivePath != "" {
		s.FileName = filepath.Base(a.archivePath)
	}
	a.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (a *Agent) handleGather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	a.mu.Lock()
	if a.status == "gathering" {
		a.mu.Unlock()
		http.Error(w, `{"error":"gather already in progress"}`, http.StatusConflict)
		return
	}
	a.mu.Unlock()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req GatherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.GatherTypes) == 0 {
		req.GatherTypes = []string{"default"}
	}
	for _, gt := range req.GatherTypes {
		if !validAgentGatherTypes[gt] {
			http.Error(w, `{"error":"invalid gather type"}`, http.StatusBadRequest)
			return
		}
	}

	go a.runGather(req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "gathering"})
}

func (a *Agent) handleDownload(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if a.status != "serving" || a.archivePath == "" {
		a.mu.Unlock()
		http.Error(w, `{"error":"no archive available"}`, http.StatusNotFound)
		return
	}
	archivePath := a.archivePath
	a.mu.Unlock()

	f, err := os.Open(archivePath)
	if err != nil {
		http.Error(w, `{"error":"archive file not found"}`, http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, `{"error":"cannot stat archive"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(archivePath)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	io.Copy(w, f)
}

func (a *Agent) runGather(req GatherRequest) {
	a.mu.Lock()
	a.status = "gathering"
	a.startedAt = time.Now()
	a.gatherErr = ""
	a.logBuf.Reset()
	// Clean up previous archive
	if a.archivePath != "" {
		os.Remove(a.archivePath)
		a.archivePath = ""
	}
	a.mu.Unlock()

	ts := time.Now().Format("20060102-150405")
	gatherDir := filepath.Join(a.workDir, "gather-"+ts)
	os.MkdirAll(gatherDir, 0700)

	logFn := func(msg string) {
		a.mu.Lock()
		a.logBuf.WriteString(msg + "\n")
		a.mu.Unlock()
		log.Printf("[gather] %s", msg)
	}

	stepFn := func(step, total int, label string) {
		logFn(fmt.Sprintf("Step %d/%d: %s", step, total, label))
	}

	a.operatorsMu.RLock()
	detected := make(map[string]bool)
	for k, v := range a.detected {
		detected[k] = v
	}
	a.operatorsMu.RUnlock()

	ctx := context.Background()

	// Run each gather type
	hasDefault := false
	for _, gt := range req.GatherTypes {
		if gt == "default" {
			hasDefault = true
			break
		}
	}

	if len(req.GatherTypes) == 1 && req.GatherTypes[0] == "all" {
		// Gather all detected
		logFn("Running gather for all detected operators...")
		if err := a.engine.Run(ctx, collector.RunOpts{
			GatherType: "all",
			Detected:   detected,
			DestDir:    gatherDir,
			Since:      req.Since,
			OnStep:     stepFn,
			OnLog:      logFn,
		}); err != nil {
			a.setError(fmt.Sprintf("gather failed: %v", err))
			return
		}
	} else if len(req.GatherTypes) == 1 {
		gt := req.GatherTypes[0]
		logFn(fmt.Sprintf("Running gather for type: %s", gt))
		opts := collector.RunOpts{
			GatherType: gt,
			Detected:   detected,
			DestDir:    gatherDir,
			Since:      req.Since,
			OnStep:     stepFn,
			OnLog:      logFn,
		}
		if gt == "custom" && len(req.Namespaces) > 0 {
			opts.CustomNamespaces = req.Namespaces
			opts.CustomResourceTypes = req.ResourceTypes
			opts.CustomIncludeLogs = req.IncludeLogs
		}
		if err := a.engine.Run(ctx, opts); err != nil {
			a.setError(fmt.Sprintf("gather failed: %v", err))
			return
		}
	} else {
		// Multi-type: run default first, then each additional type
		for i, gt := range req.GatherTypes {
			if ctx.Err() != nil {
				a.setError("gather cancelled")
				return
			}
			skipDefault := i > 0 && hasDefault
			logFn(fmt.Sprintf("Running gather for type: %s (skipDefault=%v)", gt, skipDefault))
			if err := a.engine.Run(ctx, collector.RunOpts{
				GatherType:  gt,
				Detected:    detected,
				DestDir:     gatherDir,
				Since:       req.Since,
				SkipDefault: skipDefault,
				OnStep:      stepFn,
				OnLog:       logFn,
			}); err != nil {
				logFn(fmt.Sprintf("Warning: gather for %s failed: %v", gt, err))
			}
		}
	}

	// Create tar.gz archive with standard must-gather directory structure
	logFn("Creating archive...")
	archivePath := filepath.Join(a.workDir, "must-gather-"+ts+".tar.gz")
	innerName := req.ClusterName
	if innerName == "" {
		innerName = "ocp-support-web"
	}
	wrapperDir := "must-gather.local." + ts + "/" + innerName
	if err := createTarGz(archivePath, gatherDir, wrapperDir); err != nil {
		a.setError(fmt.Sprintf("archive creation failed: %v", err))
		return
	}

	// Clean up gather dir (keep only the archive)
	os.RemoveAll(gatherDir)

	stat, _ := os.Stat(archivePath)
	sizeMB := float64(0)
	if stat != nil {
		sizeMB = float64(stat.Size()) / 1024 / 1024
	}
	logFn(fmt.Sprintf("Archive ready: %s (%.1f MB)", filepath.Base(archivePath), sizeMB))

	a.mu.Lock()
	a.status = "serving"
	a.archivePath = archivePath
	a.mu.Unlock()
}

func (a *Agent) setError(msg string) {
	a.mu.Lock()
	a.status = "error"
	a.gatherErr = msg
	a.logBuf.WriteString("ERROR: " + msg + "\n")
	a.mu.Unlock()
	log.Printf("[gather] ERROR: %s", msg)
}

// detectOperators probes CRDs to find installed operators (same logic as status.go:GetCapabilities).
func (a *Agent) detectOperators() {
	log.Printf("Detecting installed operators...")

	type probe struct {
		apiPath string
		opType  string
		label   string
		csvNS   string
		csvPfx  string
	}

	probes := []probe{
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

	var ops []DetectedOperator
	det := make(map[string]bool)

	for _, p := range probes {
		if _, err := a.k8sClient.Get(p.apiPath); err == nil {
			ver := ""
			if p.csvNS != "" && p.csvPfx != "" {
				ver = a.csvVersion(p.csvNS, p.csvPfx)
			}
			ops = append(ops, DetectedOperator{
				Type:    p.opType,
				Label:   p.label,
				Version: ver,
			})
			det[p.opType] = true
			log.Printf("  Detected: %s (version: %s)", p.label, ver)
		}
	}

	a.operatorsMu.Lock()
	a.operators = ops
	a.detected = det
	a.operatorsMu.Unlock()

	log.Printf("Operator detection complete: %d operators found", len(ops))
}

func (a *Agent) csvVersion(ns, prefix string) string {
	data, err := a.k8sClient.Get("/apis/operators.coreos.com/v1alpha1/namespaces/" + ns + "/clusterserviceversions")
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

// createTarGz creates a gzipped tar archive of the given directory.
// wrapperDir is prepended to all paths to match the standard must-gather structure:
//
//	must-gather.local.XXXXX/image-name/cluster-scoped-resources/...
func createTarGz(archivePath, sourceDir, wrapperDir string) error {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Write wrapper directory entries
	for _, dir := range wrapperDirs(wrapperDir) {
		tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeDir,
			Name:     dir + "/",
			Mode:     0755,
		})
	}

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = wrapperDir + "/" + relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}

// wrapperDirs returns each segment of a path as cumulative directory entries.
// e.g. "a/b/c" → ["a", "a/b", "a/b/c"]
func wrapperDirs(dir string) []string {
	parts := strings.Split(dir, "/")
	var dirs []string
	for i := range parts {
		dirs = append(dirs, strings.Join(parts[:i+1], "/"))
	}
	return dirs
}
