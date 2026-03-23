package mustgather

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/metrics"
)

const gatherTimeout = 30 * time.Minute

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
	GatherAll            GatherType = "all"
	GatherEtcdBackup     GatherType = "etcd-backup"
)

type Job struct {
	ID         string     `json:"id"`
	Type       GatherType `json:"type"`
	Status     string     `json:"status"` // running, complete, failed
	StartedAt  time.Time  `json:"startedAt"`
	Error      string     `json:"error,omitempty"`
	Warning    string     `json:"warning,omitempty"`
	FilePath   string     `json:"-"`
	FileName   string     `json:"fileName,omitempty"`
	Anonymize  bool       `json:"anonymize"`
	Since      string     `json:"since,omitempty"`
	LogOutput  string     `json:"logOutput,omitempty"`
	Step       int        `json:"step"`
	TotalSteps int        `json:"totalSteps"`
	StepLabel  string     `json:"stepLabel,omitempty"`
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
	DefaultMustGather   string
	CNVMustGather       string
	ODFMustGather       string
	ACMMustGather       string
	LoggingMustGather   string
	ServiceMeshMustGather string
	ComplianceMustGather  string
	MTCMustGather       string
	GitOpsMustGather    string
	ServerlessMustGather  string
}

type Manager struct {
	workDir  string
	images   ImageConfig
	mu       sync.Mutex
	jobs     map[string]*Job
	diagMu   sync.Mutex
	diagJobs map[string]*DiagJob
}

func NewManager(workDir string, images ImageConfig) (*Manager, error) {
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return nil, err
	}
	return &Manager{
		workDir:  workDir,
		images:   images,
		jobs:     make(map[string]*Job),
		diagJobs: make(map[string]*DiagJob),
	}, nil
}

// SetACMImage sets the ACM must-gather image if not already configured.
func (m *Manager) SetACMImage(image string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.images.ACMMustGather == "" {
		m.images.ACMMustGather = image
	}
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

func (m *Manager) StartGather(gatherType GatherType, anonymize bool, since string) string {
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
	case GatherAudit:
		prefix = "audit"
	case GatherAll:
		prefix = "all"
	case GatherEtcdBackup:
		prefix = "etcd-backup"
	default:
		prefix = "unknown"
	}
	id := fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
	job := &Job{
		ID:        id,
		Type:      gatherType,
		Status:    "running",
		StartedAt: time.Now(),
		Anonymize: anonymize,
		Since:     since,
	}

	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()

	metrics.MustGatherJobsTotal.WithLabelValues(string(gatherType)).Inc()
	metrics.MustGatherJobsActive.Inc()
	go m.runGather(job)
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

func (m *Manager) runCommand(job *Job, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), gatherTimeout)
	defer cancel()

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

func (m *Manager) runGather(job *Job) {
	defer metrics.MustGatherJobsActive.Dec()

	if job.Type == GatherEtcdBackup {
		m.runEtcdBackup(job)
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

	defaultArgs := []string{"adm", "must-gather", "--dest-dir=" + destDir}
	if m.images.DefaultMustGather != "" {
		defaultArgs = append(defaultArgs, "--image="+m.images.DefaultMustGather)
	}
	if sinceArg != "" {
		defaultArgs = append(defaultArgs, sinceArg)
	}
	auditArgs := []string{"adm", "must-gather", "--dest-dir=" + destDir}
	if m.images.DefaultMustGather != "" {
		auditArgs = append(auditArgs, "--image="+m.images.DefaultMustGather)
	}
	if sinceArg != "" {
		auditArgs = append(auditArgs, sinceArg)
	}
	auditArgs = append(auditArgs, "--", "/usr/bin/gather_audit_logs")

	imageArgs := func(image string) []string {
		args := []string{"adm", "must-gather", "--dest-dir=" + destDir, "--image=" + image}
		if sinceArg != "" {
			args = append(args, sinceArg)
		}
		return args
	}

	switch job.Type {
	case GatherDefault:
		steps = append(steps, gatherStep{"Default must-gather", defaultArgs})
	case GatherVirtualization:
		steps = append(steps, gatherStep{"Virtualization must-gather", imageArgs(m.images.CNVMustGather)})
	case GatherODF:
		steps = append(steps, gatherStep{"ODF must-gather", imageArgs(m.images.ODFMustGather)})
	case GatherACM:
		steps = append(steps, gatherStep{"ACM must-gather", imageArgs(m.images.ACMMustGather)})
	case GatherLogging:
		steps = append(steps, gatherStep{"Logging must-gather", imageArgs(m.images.LoggingMustGather)})
	case GatherServiceMesh:
		steps = append(steps, gatherStep{"Service Mesh must-gather", imageArgs(m.images.ServiceMeshMustGather)})
	case GatherCompliance:
		steps = append(steps, gatherStep{"Compliance must-gather", imageArgs(m.images.ComplianceMustGather)})
	case GatherMTC:
		steps = append(steps, gatherStep{"MTC must-gather", imageArgs(m.images.MTCMustGather)})
	case GatherGitOps:
		steps = append(steps, gatherStep{"GitOps must-gather", imageArgs(m.images.GitOpsMustGather)})
	case GatherServerless:
		steps = append(steps, gatherStep{"Serverless must-gather", imageArgs(m.images.ServerlessMustGather)})
	case GatherAudit:
		steps = append(steps, gatherStep{"Audit logs", auditArgs})
	case GatherAll:
		steps = append(steps,
			gatherStep{"Default must-gather", defaultArgs},
			gatherStep{"Virtualization must-gather", imageArgs(m.images.CNVMustGather)},
			gatherStep{"ODF must-gather", imageArgs(m.images.ODFMustGather)},
			gatherStep{"Audit logs", auditArgs},
		)
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

		err := m.runCommand(job, "oc", s.args...)
		if err != nil {
			if job.Type == GatherAll && i > 0 {
				m.appendLog(job, fmt.Sprintf("Warning: %s failed (continuing): %v", s.label, err))
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

		anonDir := destDir + "-anonymized"
		err := m.runCommand(job, "must-gather-clean", "-i", destDir, "-o", anonDir)
		if err != nil {
			m.appendLog(job, fmt.Sprintf("must-gather-clean not available (%v). Falling back to IP obfuscation...", err))
			if err := m.fallbackAnonymize(destDir); err != nil {
				m.appendLog(job, fmt.Sprintf("Warning: fallback anonymization error: %v", err))
			} else {
				m.appendLog(job, "Fallback IP anonymization complete.")
			}
		} else {
			m.appendLog(job, "=== Anonymization complete ===")
			finalDir = anonDir
		}
	}

	tarStepNum := totalSteps
	m.setStep(job, tarStepNum, totalSteps, "Creating archive")
	tarName := job.ID
	if job.Anonymize {
		tarName += "-anonymized"
	}
	m.appendLog(job, fmt.Sprintf("=== Step %d/%d: Creating tar.gz archive ===", tarStepNum, totalSteps))

	tarFile := filepath.Join(m.workDir, tarName+".tar.gz")
	err := m.runCommand(job, "tar", "-czf", tarFile, "-C", filepath.Dir(finalDir), filepath.Base(finalDir))
	if err != nil {
		m.setError(job, fmt.Sprintf("tar failed: %v", err))
		return
	}

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

func (m *Manager) runEtcdBackup(job *Job) {
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

	err = m.runCommand(job, "oc", "debug", "node/"+masterNode, "--", "/bin/bash", "-c", backupScript)
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

	copyCmd := exec.Command("oc", "debug", "node/"+masterNode, "--",
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

func (m *Manager) fallbackAnonymize(dir string) error {
	dir = filepath.Clean(dir)
	if !strings.HasPrefix(dir, m.workDir) {
		return fmt.Errorf("directory outside work dir")
	}
	findCmd := exec.Command("find", dir, "-type", "f",
		"(", "-name", "*.log", "-o", "-name", "*.yaml", "-o", "-name", "*.json", "-o", "-name", "*.txt", ")",
		"-exec", "sed", "-i", "-E", `s/\b([0-9]{1,3}\.){3}[0-9]{1,3}\b/x.x.x.x/g`, "{}", "+")
	return findCmd.Run()
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
	"services": true, "endpoints": true, "ingresses": true, "routes": true,
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
	// Validate objectType against allowlist locally (breaks taint chain for CodeQL).
	var safeObjectType string
	if AllowedDiagObjects[objectType] {
		safeObjectType = objectType
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
