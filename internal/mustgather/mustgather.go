package mustgather

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"github.com/redhat-consulting-services/ocp-support-web/internal/metrics"
	"go.yaml.in/yaml/v2"
)

const gatherTimeout = 60 * time.Minute

var validSince = regexp.MustCompile(`^[0-9]+h$`)
var validJobIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
var validBackupDir = regexp.MustCompile(`^/home/core/etcd-backup-[0-9-]+$`)

type GatherType string

const (
	GatherDefault        GatherType = "default"
	GatherVirtualization GatherType = "virtualization"
	GatherODF            GatherType = "odf"
	GatherAudit          GatherType = "audit"
	GatherACM            GatherType = "acm"
	GatherLogging        GatherType = "logging"
	GatherServiceMesh    GatherType = "service-mesh"
	GatherCompliance     GatherType = "compliance"
	GatherMTC            GatherType = "mtc"
	GatherGitOps         GatherType = "gitops"
	GatherServerless     GatherType = "serverless"
	GatherMCE            GatherType = "mce"
	GatherNetObserv      GatherType = "netobserv"
	GatherLocalStorage   GatherType = "local-storage"
	GatherSandboxed      GatherType = "sandboxed"
	GatherNHC            GatherType = "nhc"
	GatherNUMA           GatherType = "numa"
	GatherPTP            GatherType = "ptp"
	GatherSecretsStore   GatherType = "secrets-store"
	GatherLVMS           GatherType = "lvms"
	GatherAll            GatherType = "all"
	GatherMulti          GatherType = "multi"
	GatherCustom         GatherType = "custom"
	GatherEtcdBackup     GatherType = "etcd-backup"
)

type GatherOpts struct {
	NodeName      string           `json:"nodeName,omitempty"`
	NodeSelector  string           `json:"nodeSelector,omitempty"`
	HostNetwork   bool             `json:"hostNetwork,omitempty"`
	Custom        *CustomGatherOpts `json:"custom,omitempty"`
	SelectedTypes []GatherType     `json:"selectedTypes,omitempty"`
}

// CustomGatherOpts configures a custom namespace gather.
type CustomGatherOpts struct {
	Namespaces    []string `json:"namespaces"`
	ResourceTypes []string `json:"resourceTypes"`
	IncludeLogs   bool     `json:"includeLogs"`
}

type AnonOptions struct {
	IPs      bool `json:"ips"`
	MACs     bool `json:"macs"`
	Domains  bool `json:"domains"`
	Services bool `json:"services"`
	Secrets  bool `json:"secrets"`
}

func (a AnonOptions) Any() bool {
	return a.IPs || a.MACs || a.Domains || a.Services || a.Secrets
}

type Job struct {
	ID          string            `json:"id"`
	Type        GatherType        `json:"type"`
	Status      string            `json:"status"` // running, complete, failed
	StartedAt   time.Time         `json:"startedAt"`
	Error       string            `json:"error,omitempty"`
	Warning     string            `json:"warning,omitempty"`
	FilePath    string            `json:"-"`
	FileName    string            `json:"fileName,omitempty"`
	Anonymize   bool              `json:"anonymize"`
	AnonOpts    AnonOptions       `json:"anonOpts,omitempty"`
	Since       string            `json:"since,omitempty"`
	LogOutput   string            `json:"logOutput,omitempty"`
	Step        int               `json:"step"`
	TotalSteps  int               `json:"totalSteps"`
	StepLabel   string            `json:"stepLabel,omitempty"`
	CustomOpts  *CustomGatherOpts `json:"-"`
}

type DiagJob struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"` // running, complete, failed
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

type ImageConfig struct {
	DefaultMustGather      string
	CNVMustGather          string
	ODFMustGather          string
	ACMMustGather          string
	LoggingMustGather      string
	ServiceMeshMustGather  string
	ComplianceMustGather   string
	MTCMustGather          string
	GitOpsMustGather       string
	ServerlessMustGather   string
	MCEMustGather          string
	NetObservMustGather    string
	LocalStorageMustGather string
	SandboxedMustGather    string
	NHCMustGather          string
	NUMAMustGather         string
	PTPMustGather          string
	SecretsStoreMustGather string
	LVMSMustGather         string
}

// Collector is the interface for native Go collection.
type Collector interface {
	Run(ctx context.Context, opts CollectorRunOpts) error
	RunEtcdBackup(ctx context.Context, destDir string, logFn func(string), stepFn func(int, int, string)) error
}

// CollectorRunOpts configures a native collection run.
// Mirrors collector.RunOpts to avoid circular imports.
type CollectorRunOpts struct {
	GatherType  string
	Detected    map[string]bool
	DestDir     string
	Since       string
	SkipDefault bool // skip the default definition (for multi-select addon types)
	OnStep      func(step, total int, label string)
	OnLog       func(msg string)

	// Custom gather options
	CustomNamespaces    []string
	CustomResourceTypes []string
	CustomIncludeLogs   bool
}

type Manager struct {
	workDir       string
	images        ImageConfig
	clusterDomain string
	clusterName   string
	detected      map[GatherType]bool
	mu            sync.Mutex
	jobs          map[string]*Job
	cancels       map[string]context.CancelFunc
	diagMu        sync.Mutex
	diagJobs      map[string]*DiagJob
	collector    Collector
	nativeGather bool
	k8sClient    *k8s.Client
}

func NewManager(workDir string, images ImageConfig, collector Collector, nativeGather bool, k8sClient *k8s.Client) (*Manager, error) {
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return nil, err
	}
	mgr := &Manager{
		workDir:      workDir,
		images:       images,
		detected:     make(map[GatherType]bool),
		jobs:         make(map[string]*Job),
		cancels:      make(map[string]context.CancelFunc),
		diagJobs:     make(map[string]*DiagJob),
		collector:    collector,
		nativeGather: nativeGather,
		k8sClient:    k8sClient,
	}
	go mgr.cleanupLoop()
	return mgr, nil
}

const jobRetention = 24 * time.Hour

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.cleanupOldJobs()
	}
}

func (m *Manager) cleanupOldJobs() {
	cutoff := time.Now().Add(-jobRetention)

	m.mu.Lock()
	var toDelete []string
	for id, j := range m.jobs {
		if j.Status != "running" && j.StartedAt.Before(cutoff) {
			toDelete = append(toDelete, id)
			if j.FilePath != "" {
				os.Remove(j.FilePath)
			}
		}
	}
	for _, id := range toDelete {
		delete(m.jobs, id)
	}
	m.mu.Unlock()

	m.diagMu.Lock()
	var diagDelete []string
	for id, dj := range m.diagJobs {
		if dj.Status != "running" && dj.StartedAt.Before(cutoff) {
			diagDelete = append(diagDelete, id)
		}
	}
	for _, id := range diagDelete {
		delete(m.diagJobs, id)
	}
	m.diagMu.Unlock()
}

func (m *Manager) SetClusterDomain(domain string) {
	m.clusterDomain = domain
}

func (m *Manager) SetClusterName(name string) {
	m.clusterName = name
}

// SetDetected marks a gather type as available on this cluster.
func (m *Manager) SetDetected(t GatherType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detected[t] = true
}

// SetImage sets a must-gather image, overriding any existing value.
// Used when auto-detection finds a versioned image that should take
// precedence over env-var defaults (which may be stale or wrong).
func (m *Manager) SetImage(name, image string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setImageLocked(name, image)
}

// SetImageIfEmpty sets a must-gather image only if not already configured.
func (m *Manager) SetImageIfEmpty(name, image string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch name {
	case "cnv":
		if m.images.CNVMustGather != "" {
			return
		}
	case "odf":
		if m.images.ODFMustGather != "" {
			return
		}
	case "acm":
		if m.images.ACMMustGather != "" {
			return
		}
	case "logging":
		if m.images.LoggingMustGather != "" {
			return
		}
	case "gitops":
		if m.images.GitOpsMustGather != "" {
			return
		}
	case "service-mesh":
		if m.images.ServiceMeshMustGather != "" {
			return
		}
	case "mtc":
		if m.images.MTCMustGather != "" {
			return
		}
	case "serverless":
		if m.images.ServerlessMustGather != "" {
			return
		}
	case "mce":
		if m.images.MCEMustGather != "" {
			return
		}
	case "local-storage":
		if m.images.LocalStorageMustGather != "" {
			return
		}
	case "sandboxed":
		if m.images.SandboxedMustGather != "" {
			return
		}
	case "nhc":
		if m.images.NHCMustGather != "" {
			return
		}
	case "numa":
		if m.images.NUMAMustGather != "" {
			return
		}
	case "ptp":
		if m.images.PTPMustGather != "" {
			return
		}
	case "secrets-store":
		if m.images.SecretsStoreMustGather != "" {
			return
		}
	case "lvms":
		if m.images.LVMSMustGather != "" {
			return
		}
	}
	m.setImageLocked(name, image)
}

func (m *Manager) setImageLocked(name, image string) {
	switch name {
	case "cnv":
		m.images.CNVMustGather = image
	case "odf":
		m.images.ODFMustGather = image
	case "acm":
		m.images.ACMMustGather = image
	case "logging":
		m.images.LoggingMustGather = image
	case "gitops":
		m.images.GitOpsMustGather = image
	case "service-mesh":
		m.images.ServiceMeshMustGather = image
	case "mtc":
		m.images.MTCMustGather = image
	case "serverless":
		m.images.ServerlessMustGather = image
	case "mce":
		m.images.MCEMustGather = image
	case "local-storage":
		m.images.LocalStorageMustGather = image
	case "sandboxed":
		m.images.SandboxedMustGather = image
	case "nhc":
		m.images.NHCMustGather = image
	case "numa":
		m.images.NUMAMustGather = image
	case "ptp":
		m.images.PTPMustGather = image
	case "secrets-store":
		m.images.SecretsStoreMustGather = image
	case "lvms":
		m.images.LVMSMustGather = image
	}
}

// StopJob cancels a running job.
func (m *Manager) StopJob(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cancel, ok := m.cancels[id]
	if !ok {
		return false
	}
	cancel()
	return true
}

func (m *Manager) GetJob(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		cp := *j
		return &cp
	}
	return nil
}

func (m *Manager) ListJobs() []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		cp := *j
		jobs = append(jobs, &cp)
	}
	return jobs
}

// sanitizeID ensures a job ID is safe for use in file paths.
func sanitizeID(id string) string {
	id = filepath.Base(id)
	if !validJobIDPattern.MatchString(id) {
		return "invalid"
	}
	return id
}

const maxConcurrentJobs = 5

func (m *Manager) ActiveJobCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, j := range m.jobs {
		if j.Status == "running" {
			count++
		}
	}
	return count
}

func (m *Manager) StartGather(gatherType GatherType, anonOpts AnonOptions, since string, opts GatherOpts) string {
	if since != "" && !validSince.MatchString(since) {
		since = ""
	}
	// Map to safe prefix via switch — breaks taint from user input for CodeQL.
	var prefix string
	switch gatherType {
	case GatherDefault:
		prefix = "default"
	case GatherVirtualization:
		prefix = "virtualization"
	case GatherODF:
		prefix = "odf"
	case GatherACM:
		prefix = "acm"
	case GatherLogging:
		prefix = "logging"
	case GatherServiceMesh:
		prefix = "service-mesh"
	case GatherCompliance:
		prefix = "compliance"
	case GatherMTC:
		prefix = "mtc"
	case GatherGitOps:
		prefix = "gitops"
	case GatherServerless:
		prefix = "serverless"
	case GatherMCE:
		prefix = "mce"
	case GatherNetObserv:
		prefix = "netobserv"
	case GatherLocalStorage:
		prefix = "local-storage"
	case GatherSandboxed:
		prefix = "sandboxed"
	case GatherNHC:
		prefix = "nhc"
	case GatherNUMA:
		prefix = "numa"
	case GatherPTP:
		prefix = "ptp"
	case GatherSecretsStore:
		prefix = "secrets-store"
	case GatherLVMS:
		prefix = "lvms"
	case GatherAudit:
		prefix = "audit"
	case GatherAll:
		prefix = "all"
	case GatherMulti:
		prefix = "multi"
	case GatherCustom:
		prefix = "custom"
	case GatherEtcdBackup:
		prefix = "etcd-backup"
	default:
		prefix = "unknown"
	}
	id := fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
	job := &Job{
		ID:         id,
		Type:       gatherType,
		Status:     "running",
		StartedAt:  time.Now(),
		Anonymize:  anonOpts.Any(),
		AnonOpts:   anonOpts,
		Since:      since,
		CustomOpts: opts.Custom,
	}

	ctx, cancel := context.WithTimeout(context.Background(), gatherTimeout)

	m.mu.Lock()
	m.jobs[id] = job
	m.cancels[id] = cancel
	m.mu.Unlock()

	metrics.MustGatherJobsTotal.WithLabelValues(string(gatherType)).Inc()
	metrics.MustGatherJobsActive.Inc()
	go m.runGather(ctx, job, opts)
	return id
}

func (m *Manager) appendLog(job *Job, msg string) {
	m.mu.Lock()
	job.LogOutput += msg + "\n"
	m.mu.Unlock()
}

func (m *Manager) setStep(job *Job, step, total int, label string) {
	m.mu.Lock()
	job.Step = step
	job.TotalSteps = total
	job.StepLabel = label
	m.mu.Unlock()
}

var gatherErrorPatterns = []string{
	"error: unable to connect to",
	"error: tcp dial",
	"unable to retrieve container logs",
	"error gathering",
	"fatal error",
	"panic:",
	"connection refused",
	"connection reset by peer",
	"i/o timeout",
	"TLS handshake timeout",
	"no route to host",
}

func (m *Manager) runCommand(ctx context.Context, job *Job, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			m.appendLog(job, scanner.Text())
		}
		close(done)
	}()

	err := cmd.Wait()
	pw.Close()
	<-done // wait for all output to be consumed

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %v — the process was killed", gatherTimeout)
	}
	if ctx.Err() == context.Canceled {
		return fmt.Errorf("stopped by user")
	}
	if err != nil {
		return err
	}
	return nil
}

// gatherHadErrors checks job log output for error patterns and returns
// a summary if errors were detected during the gather.
func gatherHadErrors(logOutput string) string {
	var found []string
	for _, line := range strings.Split(logOutput, "\n") {
		lower := strings.ToLower(line)
		for _, pattern := range gatherErrorPatterns {
			if strings.Contains(lower, pattern) {
				found = append(found, strings.TrimSpace(line))
				break
			}
		}
	}
	if len(found) == 0 {
		return ""
	}
	if len(found) > 5 {
		return fmt.Sprintf("%d errors detected during gather. First: %s", len(found), found[0])
	}
	return fmt.Sprintf("%d error(s) detected during gather", len(found))
}

func (m *Manager) runGather(ctx context.Context, job *Job, opts GatherOpts) {
	defer metrics.MustGatherJobsActive.Dec()
	defer func() {
		m.mu.Lock()
		delete(m.cancels, job.ID)
		m.mu.Unlock()
	}()

	if m.nativeGather && m.collector != nil {
		m.runNativeGather(ctx, job, opts)
		return
	}

	if job.Type == GatherEtcdBackup {
		m.runEtcdBackup(ctx, job)
		return
	}

	destDir := filepath.Join(m.workDir, job.ID)
	if err := os.MkdirAll(destDir, 0700); err != nil {
		m.setError(job, fmt.Sprintf("create dir: %v", err))
		return
	}

	type gatherStep struct {
		label string
		args  []string
	}

	var steps []gatherStep

	sinceArg := ""
	if job.Since != "" {
		sinceArg = "--since=" + job.Since
	}

	// For Gather All, each step gets its own subdirectory for cleaner archives.
	stepDestDir := func(name string) string {
		if job.Type == GatherAll {
			d := filepath.Join(destDir, "must-gather-"+name)
			os.MkdirAll(d, 0700)
			return d
		}
		return destDir
	}

	addCommonArgs := func(args []string) []string {
		if sinceArg != "" {
			args = append(args, sinceArg)
		}
		if opts.NodeName != "" {
			args = append(args, "--node-name="+opts.NodeName)
		}
		if opts.NodeSelector != "" {
			args = append(args, "--node-selector="+opts.NodeSelector)
		}
		if opts.HostNetwork {
			args = append(args, "--host-network=true")
		}
		return args
	}

	mkDefaultArgs := func(dir string) []string {
		args := []string{"adm", "must-gather", "--dest-dir=" + dir}
		if m.images.DefaultMustGather != "" {
			args = append(args, "--image="+m.images.DefaultMustGather)
		}
		return addCommonArgs(args)
	}

	mkAuditArgs := func(dir string) []string {
		args := []string{"adm", "must-gather", "--dest-dir=" + dir}
		if m.images.DefaultMustGather != "" {
			args = append(args, "--image="+m.images.DefaultMustGather)
		}
		args = addCommonArgs(args)
		args = append(args, "--", "/usr/bin/gather_audit_logs")
		return args
	}

	imageArgs := func(dir, image string) []string {
		args := []string{"adm", "must-gather", "--dest-dir=" + dir, "--image=" + image}
		return addCommonArgs(args)
	}

	switch job.Type {
	case GatherDefault:
		steps = append(steps, gatherStep{"Default must-gather", mkDefaultArgs(destDir)})
	case GatherVirtualization:
		steps = append(steps, gatherStep{"Virtualization must-gather", imageArgs(destDir, m.images.CNVMustGather)})
	case GatherODF:
		steps = append(steps, gatherStep{"ODF must-gather", imageArgs(destDir, m.images.ODFMustGather)})
	case GatherACM:
		steps = append(steps, gatherStep{"ACM must-gather", imageArgs(destDir, m.images.ACMMustGather)})
	case GatherLogging:
		steps = append(steps, gatherStep{"Logging must-gather", imageArgs(destDir, m.images.LoggingMustGather)})
	case GatherServiceMesh:
		steps = append(steps, gatherStep{"Service Mesh must-gather", imageArgs(destDir, m.images.ServiceMeshMustGather)})
	case GatherCompliance:
		steps = append(steps, gatherStep{"Compliance must-gather", imageArgs(destDir, m.images.ComplianceMustGather)})
	case GatherMTC:
		steps = append(steps, gatherStep{"MTC must-gather", imageArgs(destDir, m.images.MTCMustGather)})
	case GatherGitOps:
		steps = append(steps, gatherStep{"GitOps must-gather", imageArgs(destDir, m.images.GitOpsMustGather)})
	case GatherServerless:
		steps = append(steps, gatherStep{"Serverless must-gather", imageArgs(destDir, m.images.ServerlessMustGather)})
	case GatherMCE:
		steps = append(steps, gatherStep{"MCE must-gather", imageArgs(destDir, m.images.MCEMustGather)})
	case GatherNetObserv:
		steps = append(steps, gatherStep{"Network Observability must-gather", imageArgs(destDir, m.images.NetObservMustGather)})
	case GatherLocalStorage:
		steps = append(steps, gatherStep{"Local Storage must-gather", imageArgs(destDir, m.images.LocalStorageMustGather)})
	case GatherSandboxed:
		steps = append(steps, gatherStep{"Sandboxed Containers must-gather", imageArgs(destDir, m.images.SandboxedMustGather)})
	case GatherNHC:
		steps = append(steps, gatherStep{"Node Health Check must-gather", imageArgs(destDir, m.images.NHCMustGather)})
	case GatherNUMA:
		steps = append(steps, gatherStep{"NUMA Resources must-gather", imageArgs(destDir, m.images.NUMAMustGather)})
	case GatherPTP:
		steps = append(steps, gatherStep{"PTP must-gather", imageArgs(destDir, m.images.PTPMustGather)})
	case GatherSecretsStore:
		steps = append(steps, gatherStep{"Secrets Store CSI must-gather", imageArgs(destDir, m.images.SecretsStoreMustGather)})
	case GatherLVMS:
		steps = append(steps, gatherStep{"LVMS must-gather", imageArgs(destDir, m.images.LVMSMustGather)})
	case GatherAudit:
		steps = append(steps, gatherStep{"Audit logs", mkAuditArgs(destDir)})
	case GatherCustom:
		if opts.Custom != nil {
			for _, ns := range opts.Custom.Namespaces {
				args := []string{"adm", "inspect", "ns/" + ns, "--dest-dir=" + destDir}
				if sinceArg != "" {
					args = append(args, sinceArg)
				}
				steps = append(steps, gatherStep{fmt.Sprintf("Inspect namespace %s", ns), args})
			}
		}
	case GatherMulti:
		selected := map[GatherType]bool{}
		for _, t := range opts.SelectedTypes {
			selected[t] = true
		}
		if selected[GatherDefault] {
			steps = append(steps, gatherStep{"Default must-gather", mkDefaultArgs(stepDestDir("default"))})
		}
		if selected[GatherVirtualization] && m.images.CNVMustGather != "" {
			steps = append(steps, gatherStep{"Virtualization must-gather", imageArgs(stepDestDir("virtualization"), m.images.CNVMustGather)})
		}
		if selected[GatherODF] && m.images.ODFMustGather != "" {
			steps = append(steps, gatherStep{"ODF must-gather", imageArgs(stepDestDir("odf"), m.images.ODFMustGather)})
		}
		if selected[GatherACM] && m.images.ACMMustGather != "" {
			steps = append(steps, gatherStep{"ACM must-gather", imageArgs(stepDestDir("acm"), m.images.ACMMustGather)})
		}
		if selected[GatherLogging] && m.images.LoggingMustGather != "" {
			steps = append(steps, gatherStep{"Logging must-gather", imageArgs(stepDestDir("logging"), m.images.LoggingMustGather)})
		}
		if selected[GatherServiceMesh] && m.images.ServiceMeshMustGather != "" {
			steps = append(steps, gatherStep{"Service Mesh must-gather", imageArgs(stepDestDir("service-mesh"), m.images.ServiceMeshMustGather)})
		}
		if selected[GatherCompliance] && m.images.ComplianceMustGather != "" {
			steps = append(steps, gatherStep{"Compliance must-gather", imageArgs(stepDestDir("compliance"), m.images.ComplianceMustGather)})
		}
		if selected[GatherMTC] && m.images.MTCMustGather != "" {
			steps = append(steps, gatherStep{"MTC must-gather", imageArgs(stepDestDir("mtc"), m.images.MTCMustGather)})
		}
		if selected[GatherGitOps] && m.images.GitOpsMustGather != "" {
			steps = append(steps, gatherStep{"GitOps must-gather", imageArgs(stepDestDir("gitops"), m.images.GitOpsMustGather)})
		}
		if selected[GatherServerless] && m.images.ServerlessMustGather != "" {
			steps = append(steps, gatherStep{"Serverless must-gather", imageArgs(stepDestDir("serverless"), m.images.ServerlessMustGather)})
		}
		if selected[GatherMCE] && m.images.MCEMustGather != "" {
			steps = append(steps, gatherStep{"MCE must-gather", imageArgs(stepDestDir("mce"), m.images.MCEMustGather)})
		}
		if selected[GatherNetObserv] && m.images.NetObservMustGather != "" {
			steps = append(steps, gatherStep{"Network Observability must-gather", imageArgs(stepDestDir("netobserv"), m.images.NetObservMustGather)})
		}
		if selected[GatherLocalStorage] && m.images.LocalStorageMustGather != "" {
			steps = append(steps, gatherStep{"Local Storage must-gather", imageArgs(stepDestDir("local-storage"), m.images.LocalStorageMustGather)})
		}
		if selected[GatherSandboxed] && m.images.SandboxedMustGather != "" {
			steps = append(steps, gatherStep{"Sandboxed Containers must-gather", imageArgs(stepDestDir("sandboxed"), m.images.SandboxedMustGather)})
		}
		if selected[GatherNHC] && m.images.NHCMustGather != "" {
			steps = append(steps, gatherStep{"Node Health Check must-gather", imageArgs(stepDestDir("nhc"), m.images.NHCMustGather)})
		}
		if selected[GatherNUMA] && m.images.NUMAMustGather != "" {
			steps = append(steps, gatherStep{"NUMA Resources must-gather", imageArgs(stepDestDir("numa"), m.images.NUMAMustGather)})
		}
		if selected[GatherPTP] && m.images.PTPMustGather != "" {
			steps = append(steps, gatherStep{"PTP must-gather", imageArgs(stepDestDir("ptp"), m.images.PTPMustGather)})
		}
		if selected[GatherSecretsStore] && m.images.SecretsStoreMustGather != "" {
			steps = append(steps, gatherStep{"Secrets Store CSI must-gather", imageArgs(stepDestDir("secrets-store"), m.images.SecretsStoreMustGather)})
		}
		if selected[GatherLVMS] && m.images.LVMSMustGather != "" {
			steps = append(steps, gatherStep{"LVMS must-gather", imageArgs(stepDestDir("lvms"), m.images.LVMSMustGather)})
		}
		if selected[GatherAudit] {
			steps = append(steps, gatherStep{"Audit logs", mkAuditArgs(stepDestDir("audit"))})
		}
	case GatherAll:
		steps = append(steps, gatherStep{"Default must-gather", mkDefaultArgs(stepDestDir("default"))})
		if m.detected[GatherVirtualization] && m.images.CNVMustGather != "" {
			steps = append(steps, gatherStep{"Virtualization must-gather", imageArgs(stepDestDir("virtualization"), m.images.CNVMustGather)})
		}
		if m.detected[GatherODF] && m.images.ODFMustGather != "" {
			steps = append(steps, gatherStep{"ODF must-gather", imageArgs(stepDestDir("odf"), m.images.ODFMustGather)})
		}
		if m.detected[GatherACM] && m.images.ACMMustGather != "" {
			steps = append(steps, gatherStep{"ACM must-gather", imageArgs(stepDestDir("acm"), m.images.ACMMustGather)})
		}
		if m.detected[GatherLogging] && m.images.LoggingMustGather != "" {
			steps = append(steps, gatherStep{"Logging must-gather", imageArgs(stepDestDir("logging"), m.images.LoggingMustGather)})
		}
		if m.detected[GatherServiceMesh] && m.images.ServiceMeshMustGather != "" {
			steps = append(steps, gatherStep{"Service Mesh must-gather", imageArgs(stepDestDir("service-mesh"), m.images.ServiceMeshMustGather)})
		}
		if m.detected[GatherCompliance] {
			steps = append(steps, gatherStep{"Compliance must-gather", imageArgs(stepDestDir("compliance"), m.images.ComplianceMustGather)})
		}
		if m.detected[GatherMTC] && m.images.MTCMustGather != "" {
			steps = append(steps, gatherStep{"MTC must-gather", imageArgs(stepDestDir("mtc"), m.images.MTCMustGather)})
		}
		if m.detected[GatherGitOps] && m.images.GitOpsMustGather != "" {
			steps = append(steps, gatherStep{"GitOps must-gather", imageArgs(stepDestDir("gitops"), m.images.GitOpsMustGather)})
		}
		if m.detected[GatherServerless] && m.images.ServerlessMustGather != "" {
			steps = append(steps, gatherStep{"Serverless must-gather", imageArgs(stepDestDir("serverless"), m.images.ServerlessMustGather)})
		}
		if m.detected[GatherMCE] && m.images.MCEMustGather != "" {
			steps = append(steps, gatherStep{"MCE must-gather", imageArgs(stepDestDir("mce"), m.images.MCEMustGather)})
		}
		if m.detected[GatherNetObserv] && m.images.NetObservMustGather != "" {
			steps = append(steps, gatherStep{"Network Observability must-gather", imageArgs(stepDestDir("netobserv"), m.images.NetObservMustGather)})
		}
		if m.detected[GatherLocalStorage] && m.images.LocalStorageMustGather != "" {
			steps = append(steps, gatherStep{"Local Storage must-gather", imageArgs(stepDestDir("local-storage"), m.images.LocalStorageMustGather)})
		}
		if m.detected[GatherSandboxed] && m.images.SandboxedMustGather != "" {
			steps = append(steps, gatherStep{"Sandboxed Containers must-gather", imageArgs(stepDestDir("sandboxed"), m.images.SandboxedMustGather)})
		}
		if m.detected[GatherNHC] && m.images.NHCMustGather != "" {
			steps = append(steps, gatherStep{"Node Health Check must-gather", imageArgs(stepDestDir("nhc"), m.images.NHCMustGather)})
		}
		if m.detected[GatherNUMA] && m.images.NUMAMustGather != "" {
			steps = append(steps, gatherStep{"NUMA Resources must-gather", imageArgs(stepDestDir("numa"), m.images.NUMAMustGather)})
		}
		if m.detected[GatherPTP] && m.images.PTPMustGather != "" {
			steps = append(steps, gatherStep{"PTP must-gather", imageArgs(stepDestDir("ptp"), m.images.PTPMustGather)})
		}
		if m.detected[GatherSecretsStore] && m.images.SecretsStoreMustGather != "" {
			steps = append(steps, gatherStep{"Secrets Store CSI must-gather", imageArgs(stepDestDir("secrets-store"), m.images.SecretsStoreMustGather)})
		}
		if m.detected[GatherLVMS] && m.images.LVMSMustGather != "" {
			steps = append(steps, gatherStep{"LVMS must-gather", imageArgs(stepDestDir("lvms"), m.images.LVMSMustGather)})
		}
		steps = append(steps, gatherStep{"Audit logs", mkAuditArgs(stepDestDir("audit"))})
	}

	extraSteps := 1
	if job.Anonymize {
		extraSteps = 2
	}
	totalSteps := len(steps) + extraSteps

	for i, s := range steps {
		stepNum := i + 1
		m.setStep(job, stepNum, totalSteps, s.label)
		m.appendLog(job, fmt.Sprintf("=== Step %d/%d: %s ===", stepNum, totalSteps, s.label))

		err := m.runCommand(ctx, job, "oc", s.args...)
		if err != nil {
			if ctx.Err() != nil {
				m.setError(job, "Stopped by user")
				return
			}
			if job.Type == GatherAll || job.Type == GatherMulti {
				m.appendLog(job, fmt.Sprintf("Warning: %s failed (continuing): %v", s.label, err))
				m.appendLog(job, fmt.Sprintf("=== %s failed ===", s.label))
				continue
			}
			m.setError(job, fmt.Sprintf("%s failed: %v", s.label, err))
			return
		}
		m.appendLog(job, fmt.Sprintf("=== %s complete ===", s.label))
	}

	finalDir := destDir
	if job.Anonymize {
		stepNum := len(steps) + 1
		m.setStep(job, stepNum, totalSteps, "Anonymizing data")
		m.appendLog(job, fmt.Sprintf("=== Step %d/%d: Anonymizing data ===", stepNum, totalSteps))

		nodeMapping := m.buildAndLogNodeMapping(job)

		m.appendLog(job, "Obfuscating data...")
		redacted, err := FastAnonymize(destDir, m.workDir, m.clusterDomain, job.AnonOpts, nodeMapping)
		if err != nil {
			m.appendLog(job, fmt.Sprintf("Warning: anonymization error: %v", err))
		} else {
			m.appendLog(job, "Obfuscation complete.")
			if redacted > 0 {
				m.appendLog(job, fmt.Sprintf("Redacted %d secret value(s).", redacted))
			}
		}
		m.appendLog(job, "=== Anonymizing data complete ===")
	}

	tarStepNum := totalSteps
	m.setStep(job, tarStepNum, totalSteps, "Creating archive")
	tarName := job.ID
	if job.Anonymize {
		tarName += "-anonymized"
	}
	m.appendLog(job, fmt.Sprintf("=== Step %d/%d: Creating tar.gz archive ===", tarStepNum, totalSteps))

	tarFile := filepath.Join(m.workDir, tarName+".tar.gz")
	err := m.runCommand(ctx, job, "tar", "-czf", tarFile, "-C", filepath.Dir(finalDir), filepath.Base(finalDir))
	if err != nil {
		m.setError(job, fmt.Sprintf("tar failed: %v", err))
		return
	}
	m.appendLog(job, "=== Creating tar.gz archive complete ===")

	m.mu.Lock()
	logSnapshot := job.LogOutput
	m.mu.Unlock()

	warning := gatherHadErrors(logSnapshot)

	m.mu.Lock()
	job.Status = "complete"
	job.FilePath = tarFile
	job.FileName = tarName + ".tar.gz"
	job.Step = totalSteps
	job.Warning = warning
	if warning != "" {
		job.StepLabel = "Completed with errors"
		job.LogOutput += "WARNING: " + warning + "\n"
		job.LogOutput += "=== Done! Archive ready for download (errors detected during gather). ===\n"
	} else {
		job.StepLabel = "Complete"
		job.LogOutput += "=== Done! Archive ready for download. ===\n"
	}
	m.mu.Unlock()
}

func (m *Manager) runNativeGather(ctx context.Context, job *Job, opts GatherOpts) {
	if job.Type == GatherEtcdBackup {
		m.runNativeEtcdBackup(ctx, job)
		return
	}

	destDir := filepath.Join(m.workDir, job.ID)
	if err := os.MkdirAll(destDir, 0700); err != nil {
		m.setError(job, fmt.Sprintf("create dir: %v", err))
		return
	}

	// Convert detected map keys to strings for the collector
	detected := make(map[string]bool)
	m.mu.Lock()
	for k, v := range m.detected {
		detected[string(k)] = v
	}
	m.mu.Unlock()

	// For multi-select, run the collector once per selected type
	gatherTypes := []string{string(job.Type)}
	if job.Type == GatherMulti && len(opts.SelectedTypes) > 0 {
		gatherTypes = make([]string, len(opts.SelectedTypes))
		for i, t := range opts.SelectedTypes {
			gatherTypes[i] = string(t)
		}
	}

	for i, gt := range gatherTypes {
		if ctx.Err() != nil {
			m.setError(job, "Stopped by user")
			return
		}

		typeDestDir := destDir
		if len(gatherTypes) > 1 {
			typeDestDir = filepath.Join(destDir, filepath.Clean(gt))
			os.MkdirAll(typeDestDir, 0700)
			m.appendLog(job, fmt.Sprintf("=== Gather %d/%d: %s ===", i+1, len(gatherTypes), gt))
		}

		// In multi-select, skip the default definition for non-default types
		// since default is already collected as its own type
		skipDefault := len(gatherTypes) > 1 && gt != "default"

		runOpts := CollectorRunOpts{
			GatherType:  gt,
			Detected:    detected,
			DestDir:     typeDestDir,
			Since:       job.Since,
			SkipDefault: skipDefault,
			OnStep:      func(step, total int, label string) { m.setStep(job, step, total+1, label) },
			OnLog:       func(msg string) { m.appendLog(job, msg) },
		}
		if job.Type == GatherCustom && job.CustomOpts != nil {
			runOpts.CustomNamespaces = job.CustomOpts.Namespaces
			runOpts.CustomResourceTypes = job.CustomOpts.ResourceTypes
			runOpts.CustomIncludeLogs = job.CustomOpts.IncludeLogs
		}
		err := m.collector.Run(ctx, runOpts)
		if err != nil {
			if ctx.Err() != nil {
				m.setError(job, "Stopped by user")
				return
			}
			if len(gatherTypes) > 1 {
				// Multi: continue on failure like GatherAll
				m.appendLog(job, fmt.Sprintf("Warning: %s collection failed (continuing): %v", gt, err))
				continue
			}
			m.setError(job, fmt.Sprintf("collection failed: %v", err))
			return
		}
	}

	// Anonymization
	if job.Anonymize {
		m.appendLog(job, "=== Anonymizing data ===")

		nodeMapping := m.buildAndLogNodeMapping(job)

		redacted, err := FastAnonymize(destDir, m.workDir, m.clusterDomain, job.AnonOpts, nodeMapping)
		if err != nil {
			m.appendLog(job, fmt.Sprintf("Warning: anonymization error: %v", err))
		} else {
			m.appendLog(job, "Obfuscation complete.")
			if redacted > 0 {
				m.appendLog(job, fmt.Sprintf("Redacted %d secret value(s).", redacted))
			}
		}
	}

	// Create archive with standard must-gather directory structure
	tarName := job.ID
	if job.Anonymize {
		tarName += "-anonymized"
	}
	m.appendLog(job, "=== Creating tar.gz archive ===")

	// Restructure: rename destDir into must-gather.local.XXX/{clusterName}/
	innerName := m.clusterName
	if innerName == "" {
		innerName = "cluster"
	}
	wrapperBase := "must-gather.local." + job.ID
	wrapperDir := filepath.Join(m.workDir, wrapperBase)
	innerDir := filepath.Join(wrapperDir, innerName)
	os.MkdirAll(wrapperDir, 0700)
	os.Rename(destDir, innerDir)

	tarFile := filepath.Join(m.workDir, tarName+".tar.gz")
	if err := m.runCommand(ctx, job, "tar", "-czf", tarFile, "-C", m.workDir, wrapperBase); err != nil {
		m.setError(job, fmt.Sprintf("tar failed: %v", err))
		// Restore destDir on failure
		os.Rename(innerDir, destDir)
		os.Remove(wrapperDir)
		return
	}
	os.RemoveAll(wrapperDir)
	m.appendLog(job, "=== Creating tar.gz archive complete ===")

	m.mu.Lock()
	logSnapshot := job.LogOutput
	m.mu.Unlock()

	warning := gatherHadErrors(logSnapshot)

	m.mu.Lock()
	job.Status = "complete"
	job.FilePath = tarFile
	job.FileName = tarName + ".tar.gz"
	job.Warning = warning
	if warning != "" {
		job.StepLabel = "Completed with errors"
		job.LogOutput += "WARNING: " + warning + "\n"
		job.LogOutput += "=== Done! Archive ready for download (errors detected during gather). ===\n"
	} else {
		job.StepLabel = "Complete"
		job.LogOutput += "=== Done! Archive ready for download. ===\n"
	}
	m.mu.Unlock()
}

func (m *Manager) runNativeEtcdBackup(ctx context.Context, job *Job) {
	destDir := filepath.Join(m.workDir, job.ID)
	if err := os.MkdirAll(destDir, 0700); err != nil {
		m.setError(job, fmt.Sprintf("create dir: %v", err))
		return
	}

	err := m.collector.RunEtcdBackup(ctx, destDir,
		func(msg string) { m.appendLog(job, msg) },
		func(step, total int, label string) { m.setStep(job, step, total, label) },
	)
	if err != nil {
		if ctx.Err() != nil {
			m.setError(job, "Stopped by user")
			return
		}
		m.setError(job, fmt.Sprintf("etcd backup failed: %v", err))
		return
	}

	safeID := sanitizeID(job.ID)
	tarFile := filepath.Join(m.workDir, safeID+".tar.gz")

	// Check if the backup was written as a tar directly
	backupTar := filepath.Join(destDir, "etcd-backup.tar.gz")
	if info, err := os.Stat(backupTar); err == nil && info.Size() > 0 {
		// Move the tar to the expected location
		if err := os.Rename(backupTar, tarFile); err != nil {
			m.setError(job, fmt.Sprintf("move backup: %v", err))
			return
		}
	} else {
		m.setError(job, "backup archive is empty or not created")
		return
	}

	info, err := os.Stat(tarFile)
	if err != nil || info.Size() == 0 {
		m.setError(job, "backup archive is empty or not created")
		return
	}

	m.appendLog(job, fmt.Sprintf("Backup archive created: %s (%.1f MB)", filepath.Base(tarFile), float64(info.Size())/(1024*1024)))

	m.mu.Lock()
	job.Status = "complete"
	job.FilePath = tarFile
	job.FileName = "etcd-backup-" + job.ID + ".tar.gz"
	job.StepLabel = "Complete"
	job.LogOutput += "=== Done! Etcd backup ready for download. ===\n"
	m.mu.Unlock()
}

func (m *Manager) runEtcdBackup(ctx context.Context, job *Job) {
	destDir := filepath.Join(m.workDir, job.ID)
	if err := os.MkdirAll(destDir, 0700); err != nil {
		m.setError(job, fmt.Sprintf("create dir: %v", err))
		return
	}

	totalSteps := 3
	m.setStep(job, 1, totalSteps, "Finding master node")
	m.appendLog(job, "=== Step 1/3: Finding a master node ===")

	cmd := exec.Command("oc", "get", "nodes", "-l", "node-role.kubernetes.io/master=", "-o", "jsonpath={.items[0].metadata.name}")
	out, err := cmd.Output()
	if err != nil {
		m.setError(job, fmt.Sprintf("failed to find master node: %v", err))
		return
	}
	masterNode := strings.TrimSpace(string(out))
	if masterNode == "" {
		m.setError(job, "no master node found")
		return
	}
	if ok, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`, masterNode); !ok {
		m.setError(job, "invalid master node name")
		return
	}
	m.appendLog(job, fmt.Sprintf("Using master node: %s", masterNode))

	m.setStep(job, 2, totalSteps, "Running etcd backup on "+masterNode)
	m.appendLog(job, "=== Step 2/3: Running etcd backup ===")

	backupScript := `chroot /host /bin/bash -c '
		BACKUP_DIR=/home/core/etcd-backup-$(date +%Y%m%d-%H%M%S)
		mkdir -p ${BACKUP_DIR}
		/usr/local/bin/cluster-backup.sh ${BACKUP_DIR}
		echo "BACKUP_DIR=${BACKUP_DIR}"
		ls -la ${BACKUP_DIR}/
	'`

	err = m.runCommand(ctx, job, "oc", "debug", "node/"+masterNode, "--", "/bin/bash", "-c", backupScript)
	if err != nil {
		m.setError(job, fmt.Sprintf("etcd backup failed: %v", err))
		return
	}

	m.mu.Lock()
	logOutput := job.LogOutput
	m.mu.Unlock()

	var backupDir string
	for _, line := range strings.Split(logOutput, "\n") {
		if strings.HasPrefix(line, "BACKUP_DIR=") {
			backupDir = strings.TrimPrefix(line, "BACKUP_DIR=")
			break
		}
	}
	if backupDir == "" {
		m.setError(job, "could not determine backup directory from output")
		return
	}
	backupDir = strings.TrimSpace(backupDir)
	if !validBackupDir.MatchString(backupDir) {
		m.setError(job, "unexpected backup directory path")
		return
	}

	m.setStep(job, 3, totalSteps, "Copying backup files")
	m.appendLog(job, "=== Step 3/3: Copying backup files from node ===")

	safeID := sanitizeID(job.ID)
	tarFile := filepath.Join(m.workDir, safeID+".tar.gz")

	copyCmd := exec.CommandContext(ctx, "oc", "debug", "node/"+masterNode, "--",
		"/bin/bash", "-c", `chroot /host tar czf - -C "$1" .`, "--", backupDir)
	outFile, err := os.Create(tarFile)
	if err != nil {
		m.setError(job, fmt.Sprintf("create output file: %v", err))
		return
	}
	copyCmd.Stdout = outFile
	copyCmd.Stderr = os.Stderr

	if err := copyCmd.Run(); err != nil {
		outFile.Close()
		os.Remove(tarFile)
		m.setError(job, fmt.Sprintf("copy backup failed: %v", err))
		return
	}
	outFile.Close()

	info, err := os.Stat(tarFile)
	if err != nil || info.Size() == 0 {
		os.Remove(tarFile)
		m.setError(job, "backup archive is empty or not created")
		return
	}

	m.appendLog(job, fmt.Sprintf("Backup archive created: %s (%.1f MB)", filepath.Base(tarFile), float64(info.Size())/(1024*1024)))

	cleanupCmd := exec.Command("oc", "debug", "node/"+masterNode, "--",
		"/bin/bash", "-c", `chroot /host rm -rf "$1"`, "--", backupDir)
	_ = cleanupCmd.Run()

	m.mu.Lock()
	job.Status = "complete"
	job.FilePath = tarFile
	job.FileName = "etcd-backup-" + job.ID + ".tar.gz"
	job.Step = totalSteps
	job.StepLabel = "Complete"
	job.LogOutput += "=== Done! Etcd backup ready for download. ===\n"
	m.mu.Unlock()
}

// BuildNodeMapping fetches the cluster's node list and returns a map of
// real hostname -> role-based pseudonym (e.g. "master-1", "worker-2").
func BuildNodeMapping(client *k8s.Client) (map[string]string, error) {
	if client == nil {
		return nil, nil
	}
	data, err := client.Get("/api/v1/nodes")
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	type nodeInfo struct {
		name string
		role string
	}

	roleFor := func(labels map[string]interface{}) string {
		for _, r := range []string{"master", "control-plane"} {
			if _, ok := labels["node-role.kubernetes.io/"+r]; ok {
				return "master"
			}
		}
		if _, ok := labels["node-role.kubernetes.io/infra"]; ok {
			return "infra"
		}
		if _, ok := labels["node-role.kubernetes.io/worker"]; ok {
			return "worker"
		}
		var custom []string
		for k := range labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
				if role != "" {
					custom = append(custom, role)
				}
			}
		}
		if len(custom) > 0 {
			sort.Strings(custom)
			return custom[0]
		}
		return "worker"
	}

	safeHostname := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

	var nodes []nodeInfo
	for _, item := range k8s.JsonArray(data, "items") {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := k8s.JsonPath(m, "metadata", "name")
		if name == "" || !safeHostname.MatchString(name) {
			continue
		}
		labels := k8s.JsonMap(m, "metadata", "labels")
		role := "worker"
		if labels != nil {
			role = roleFor(labels)
		}
		nodes = append(nodes, nodeInfo{name: name, role: role})
	}

	roleNodes := map[string][]string{}
	for _, n := range nodes {
		roleNodes[n.role] = append(roleNodes[n.role], n.name)
	}
	for role := range roleNodes {
		sort.Strings(roleNodes[role])
	}

	mapping := make(map[string]string, len(nodes))
	for _, n := range nodes {
		sorted := roleNodes[n.role]
		idx := sort.SearchStrings(sorted, n.name)
		mapping[n.name] = fmt.Sprintf("%s-%d", n.role, idx+1)
	}

	return mapping, nil
}

// FastAnonymize applies regex-based obfuscation to all text files in a directory.
// It replaces IPs, MACs, domain names, and service DNS based on the provided options.
// nodeMapping provides optional hostname-to-pseudonym replacements applied first.
// Returns the number of secret values redacted (0 if secrets option not enabled).
func FastAnonymize(dir, workDir, clusterDomain string, opts AnonOptions, nodeMapping map[string]string) (int, error) {
	dir = filepath.Clean(dir)
	if !strings.HasPrefix(dir, workDir) {
		return 0, fmt.Errorf("directory outside work dir")
	}

	if len(nodeMapping) > 0 {
		if err := anonymizeNodeNames(dir, nodeMapping); err != nil {
			return 0, fmt.Errorf("node name anonymization: %w", err)
		}
	}

	findCmd := exec.Command("find", dir, "-type", "f",
		"(", "-name", "*.log", "-o", "-name", "*.yaml", "-o", "-name", "*.json", "-o", "-name", "*.txt", ")",
		"-print0")

	sedArgs := []string{"-0", "-P", "4", "-r", "sed", "-i", "-E"}
	if opts.IPs {
		sedArgs = append(sedArgs, "-e", `s/\b([0-9]{1,3}\.){3}[0-9]{1,3}\b/x.x.x.x/g`)
	}
	if opts.MACs {
		sedArgs = append(sedArgs, "-e", `s/\b([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}\b/xx:xx:xx:xx:xx:xx/g`)
	}
	if opts.Domains && clusterDomain != "" {
		domainMask := func(domain string) string {
			parts := strings.Split(domain, ".")
			masked := make([]string, len(parts))
			for i := range parts {
				masked[i] = "x"
			}
			return strings.Join(masked, ".")
		}
		escSed := func(s string) string {
			return strings.ReplaceAll(regexp.QuoteMeta(s), `/`, `\/`)
		}
		sedArgs = append(sedArgs, "-e", `s/`+escSed(clusterDomain)+`/`+domainMask(clusterDomain)+`/g`)
		if strings.HasPrefix(clusterDomain, "apps.") {
			clusterFQDN := clusterDomain[5:]
			sedArgs = append(sedArgs, "-e", `s/`+escSed(clusterFQDN)+`/`+domainMask(clusterFQDN)+`/g`)
			if idx := strings.Index(clusterFQDN, "."); idx > 0 {
				baseDomain := clusterFQDN[idx+1:]
				sedArgs = append(sedArgs, "-e", `s/`+escSed(baseDomain)+`/`+domainMask(baseDomain)+`/g`)
			}
		}
	}
	if opts.Services {
		sedArgs = append(sedArgs, "-e", `s/[a-zA-Z0-9._-]+\.svc\.cluster\.local/cleaned.svc.cluster.local/g`)
	}

	sedCmd := exec.Command("xargs", sedArgs...)
	sedCmd.Stdin, _ = findCmd.StdoutPipe()
	if err := findCmd.Start(); err != nil {
		return 0, err
	}
	if err := sedCmd.Run(); err != nil {
		return 0, err
	}
	if err := findCmd.Wait(); err != nil {
		return 0, err
	}

	if opts.Secrets {
		count, err := redactBase64Values(dir)
		if err != nil {
			return 0, fmt.Errorf("secret redaction: %w", err)
		}
		return count, nil
	}

	return 0, nil
}

// redactBase64Values walks YAML files and replaces base64-encoded values
// in data/stringData maps with a redaction marker. Returns the number of
// values redacted.
func redactBase64Values(dir string) (int, error) {
	total := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".json") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var obj map[interface{}]interface{}
		if err := yaml.Unmarshal(raw, &obj); err != nil {
			return nil
		}

		changed := 0
		for _, key := range []string{"data", "stringData"} {
			dm, ok := obj[key].(map[interface{}]interface{})
			if !ok {
				continue
			}
			for k, v := range dm {
				s, ok := v.(string)
				if !ok || len(s) == 0 {
					continue
				}
				if isBase64Encoded(s) {
					dm[k] = "<REDACTED>"
					changed++
				}
			}
		}

		if changed > 0 {
			total += changed
			out, err := yaml.Marshal(obj)
			if err != nil {
				return nil
			}
			return os.WriteFile(path, out, info.Mode())
		}
		return nil
	})
	return total, err
}

func isBase64Encoded(s string) bool {
	if len(s) < 4 {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return false
	}
	_ = decoded
	return true
}

func anonymizeNodeNames(dir string, mapping map[string]string) error {
	// Sort hostnames longest-first to avoid partial replacements
	hostnames := make([]string, 0, len(mapping))
	for h := range mapping {
		hostnames = append(hostnames, h)
	}
	sort.Slice(hostnames, func(i, j int) bool {
		return len(hostnames[i]) > len(hostnames[j])
	})

	// Rename directories and files containing node hostnames (depth-first)
	var allPaths []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		allPaths = append(allPaths, path)
		return nil
	})
	// Process deepest paths first so parent renames don't invalidate child paths
	sort.Slice(allPaths, func(i, j int) bool {
		return len(allPaths[i]) > len(allPaths[j])
	})
	for _, p := range allPaths {
		base := filepath.Base(p)
		newBase := base
		for _, h := range hostnames {
			newBase = strings.ReplaceAll(newBase, h, mapping[h])
		}
		if newBase != base {
			newPath := filepath.Join(filepath.Dir(p), newBase)
			os.Rename(p, newPath)
		}
	}

	// Build sed expressions for file contents
	escSed := func(s string) string {
		return strings.ReplaceAll(regexp.QuoteMeta(s), `/`, `\/`)
	}
	nodeSedArgs := []string{"-0", "-P", "4", "-r", "sed", "-i"}
	for _, h := range hostnames {
		nodeSedArgs = append(nodeSedArgs, "-e", `s/`+escSed(h)+`/`+mapping[h]+`/g`)
	}

	findCmd := exec.Command("find", dir, "-type", "f",
		"(", "-name", "*.log", "-o", "-name", "*.yaml", "-o", "-name", "*.json", "-o", "-name", "*.txt", ")",
		"-print0")
	sedCmd := exec.Command("xargs", nodeSedArgs...)
	sedCmd.Stdin, _ = findCmd.StdoutPipe()
	if err := findCmd.Start(); err != nil {
		return err
	}
	if err := sedCmd.Run(); err != nil {
		return err
	}
	return findCmd.Wait()
}

func (m *Manager) buildAndLogNodeMapping(job *Job) map[string]string {
	nodeMapping, err := BuildNodeMapping(m.k8sClient)
	if err != nil {
		m.appendLog(job, fmt.Sprintf("Warning: could not build node mapping: %v", err))
		return nil
	}
	if len(nodeMapping) > 0 {
		m.appendLog(job, fmt.Sprintf("Node name mapping (%d nodes):", len(nodeMapping)))
		names := make([]string, 0, len(nodeMapping))
		for n := range nodeMapping {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			m.appendLog(job, fmt.Sprintf("  %s -> %s", n, nodeMapping[n]))
		}
	}
	return nodeMapping
}

func (m *Manager) setError(job *Job, msg string) {
	m.mu.Lock()
	job.Status = "failed"
	job.Error = msg
	job.LogOutput += "ERROR: " + msg + "\n"
	m.mu.Unlock()
}

func (m *Manager) GetFilePath(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok && j.Status == "complete" {
		return j.FilePath
	}
	return ""
}

func (m *Manager) getEtcdPodName() (string, error) {
	cmd := exec.Command("oc", "get", "pods", "-n", "openshift-etcd",
		"-l", "app=etcd", "--field-selector=status.phase==Running",
		"-o", "jsonpath={.items[0].metadata.name}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find etcd pod: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no running etcd pod found")
	}
	return name, nil
}

// AllowedDiagObjects is the set of resource types permitted for drill-down diagnostics.
var AllowedDiagObjects = map[string]bool{
	"secrets": true, "configmaps": true, "events": true, "events.events.k8s.io": true,
	"pods": true, "deployments": true, "replicasets": true, "statefulsets": true,
	"daemonsets": true, "jobs": true, "cronjobs": true,
	"services": true, "endpointslices": true, "ingresses": true, "routes": true,
	"persistentvolumeclaims": true, "persistentvolumes": true,
	"serviceaccounts": true, "roles": true, "rolebindings": true,
	"clusterroles": true, "clusterrolebindings": true,
	"namespaces": true, "nodes": true,
	"virtualmachines": true, "virtualmachineinstances": true,
}

func (m *Manager) StartDiag(diagType, objectType string) string {
	// Map to safe prefix via switch — breaks taint from user input for CodeQL.
	var prefix string
	switch diagType {
	case "object-sizes":
		prefix = "diag-object-sizes"
	case "object-counts":
		prefix = "diag-object-counts"
	case "ns-breakdown":
		prefix = "diag-ns-breakdown"
	case "creation-timeline":
		prefix = "diag-creation-timeline"
	case "ns-object-counts":
		prefix = "diag-ns-object-counts"
	default:
		prefix = "diag-unknown"
	}
	id := fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
	dj := &DiagJob{
		ID:        id,
		Type:      diagType,
		Status:    "running",
		StartedAt: time.Now(),
	}

	m.diagMu.Lock()
	m.diagJobs[id] = dj
	m.diagMu.Unlock()

	metrics.EtcdDiagJobsTotal.Inc()
	go m.runDiag(dj, objectType)
	return id
}

func (m *Manager) GetDiagJob(id string) *DiagJob {
	m.diagMu.Lock()
	defer m.diagMu.Unlock()
	if dj, ok := m.diagJobs[id]; ok {
		cp := *dj
		return &cp
	}
	return nil
}

func (m *Manager) runDiag(dj *DiagJob, objectType string) {
	// Look up objectType from the allowlist to produce a known-safe value.
	// This breaks the CodeQL taint chain since safeObjectType is never assigned
	// from the user-provided objectType directly.
	var safeObjectType string
	for allowed := range AllowedDiagObjects {
		if allowed == objectType {
			safeObjectType = allowed
			break
		}
	}

	podName, err := m.getEtcdPodName()
	if err != nil {
		m.setDiagError(dj, err.Error())
		return
	}

	var script string
	switch dj.Type {
	case "object-sizes":
		script = `etcdctl get / --prefix --keys-only | grep -oE "^/[a-z|.]+/[a-z|.|8]*" | sort | uniq -c | sort -rn | while read KEY; do printf "$KEY\t" && etcdctl get ${KEY##* } --prefix --print-value-only | wc -c | numfmt --to=iec ; done | sort -k3 -hr | column -t`
	case "object-counts":
		script = `etcdctl get / --prefix --keys-only | sed '/^$/d' | cut -d/ -f3 | sort | uniq -c | sort -rn`
	case "ns-breakdown":
		script = `etcdctl get / --prefix --keys-only | grep -oE -e "^/kubernetes.io/secrets/[-a-z|.0-9]*/" -e "^/kubernetes.io/configmaps/[-a-z|.0-9]*/" -e "^/kubernetes.io/events/[-a-z|.0-9]*/" | sort -u | while read KEY; do printf "$KEY\t" && etcdctl get ${KEY##* } --prefix --print-value-only | wc -c | numfmt --to=iec ; done | sort -k2 -hr | head -50 | awk -F'/' 'BEGIN{print "NAMESPACE TYPE SIZE"}{print $4" "$3" "$5}' | column -t`
	case "creation-timeline":
		if safeObjectType == "" {
			m.setDiagError(dj, "invalid or missing object type")
			return
		}
	case "ns-object-counts":
		if safeObjectType == "" {
			m.setDiagError(dj, "invalid or missing object type")
			return
		}
	default:
		m.setDiagError(dj, "unknown diagnostic type: "+dj.Type)
		return
	}

	var cmd *exec.Cmd
	switch dj.Type {
	case "creation-timeline":
		cmd = exec.Command("bash", "-c",
			`OBJ="$1"; echo "=== By Month ===" && oc get "$OBJ" -A -o 'jsonpath={range .items[*]}{.metadata.creationTimestamp}{"\n"}{end}' | grep -oE "[0-9]{4}-[0-9]{2}" | sort | uniq -c && echo "" && echo "=== By Day ===" && oc get "$OBJ" -A -o 'jsonpath={range .items[*]}{.metadata.creationTimestamp}{"\n"}{end}' | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}" | sort | uniq -c && echo "" && echo "=== By Hour ===" && oc get "$OBJ" -A -o 'jsonpath={range .items[*]}{.metadata.creationTimestamp}{"\n"}{end}' | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}" | sort | uniq -c`,
			"--", safeObjectType)
	case "ns-object-counts":
		cmd = exec.Command("bash", "-c",
			`oc get "$1" -A --no-headers -o custom-columns=NS:.metadata.namespace 2>/dev/null | sort | uniq -c | sort -rn | head -50`,
			"--", safeObjectType)
	default:
		cmd = exec.Command("oc", "exec", "-n", "openshift-etcd", "-c", "etcdctl", podName, "--",
			"sh", "-c", script)
	}

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	m.diagMu.Lock()
	if err != nil {
		dj.Status = "failed"
		dj.Error = err.Error()
		if output != "" {
			dj.Output = output
		}
	} else {
		dj.Status = "complete"
		dj.Output = output
	}
	m.diagMu.Unlock()
}

func (m *Manager) setDiagError(dj *DiagJob, msg string) {
	m.diagMu.Lock()
	dj.Status = "failed"
	dj.Error = msg
	m.diagMu.Unlock()
}
