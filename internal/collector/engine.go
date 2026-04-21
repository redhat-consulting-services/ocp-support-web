package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

// Engine orchestrates native Go resource collection based on ConfigMap definitions.
type Engine struct {
	k8sClient *k8s.Client
	namespace string // namespace where gather ConfigMaps live
}

// NewEngine creates a collection engine.
func NewEngine(k8sClient *k8s.Client, namespace string) *Engine {
	return &Engine{
		k8sClient: k8sClient,
		namespace: namespace,
	}
}

// RunOpts configures a collection run.
type RunOpts struct {
	GatherType  string
	Detected    map[string]bool
	DestDir     string
	Since       string
	SkipDefault bool // when true, don't include the default definition (used in multi-select)
	OnStep      func(step, total int, label string)
	OnLog       func(msg string)

	// Custom gather options — used when GatherType is "custom"
	CustomNamespaces    []string
	CustomResourceTypes []string
	CustomIncludeLogs   bool
}

// Run executes the collection based on ConfigMap definitions.
func (e *Engine) Run(ctx context.Context, opts RunOpts) error {
	logFn := opts.OnLog
	if logFn == nil {
		logFn = func(string) {}
	}
	stepFn := opts.OnStep
	if stepFn == nil {
		stepFn = func(int, int, string) {}
	}

	// Write timestamp file
	tsFile := filepath.Join(opts.DestDir, "timestamp")
	_ = writeFile(opts.DestDir, tsFile, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"))

	var defs []GatherDefinition

	if opts.GatherType == "custom" && len(opts.CustomNamespaces) > 0 {
		logFn(fmt.Sprintf("Building custom gather definition for %d namespaces, %d resource types, logs=%v",
			len(opts.CustomNamespaces), len(opts.CustomResourceTypes), opts.CustomIncludeLogs))
		defs = []GatherDefinition{buildCustomDefinition(opts.CustomNamespaces, opts.CustomResourceTypes, opts.CustomIncludeLogs)}
	} else {
		logFn(fmt.Sprintf("Loading gather definitions for type=%s (customNS=%d)...", opts.GatherType, len(opts.CustomNamespaces)))
		var err error
		defs, err = loadDefinitions(e.k8sClient, e.namespace, opts.GatherType, opts.Detected, opts.SkipDefault)
		if err != nil {
			return fmt.Errorf("load definitions: %w", err)
		}
	}

	if len(defs) == 0 {
		return fmt.Errorf("no gather definitions found")
	}

	// Calculate total steps: one per definition for resources, one for logs, one for execs, one for node commands
	// Simplified: one step per definition
	totalSteps := len(defs)
	var allErrors []error

	for i, def := range defs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		stepNum := i + 1
		label := def.DisplayName
		if label == "" {
			label = fmt.Sprintf("Definition %d", stepNum)
		}

		stepFn(stepNum, totalSteps, label)
		logFn(fmt.Sprintf("=== Step %d/%d: %s ===", stepNum, totalSteps, label))

		// Collect cluster-scoped resources
		if len(def.ClusterResources) > 0 {
			logFn("Collecting cluster-scoped resources...")
			errs := e.collectClusterResources(ctx, def, opts.DestDir, logFn)
			allErrors = append(allErrors, errs...)
		}

		// Collect namespaced resources
		if len(def.NamespacedResources) > 0 {
			logFn("Collecting namespaced resources...")
			errs := e.collectNamespacedResources(ctx, def, opts.DestDir, logFn)
			allErrors = append(allErrors, errs...)
		}

		// Collect pod logs
		if len(def.PodLogs) > 0 {
			logFn("Collecting pod logs...")
			errs := e.collectPodLogs(ctx, def, opts.DestDir, opts.Since, logFn)
			allErrors = append(allErrors, errs...)
		}

		// Collect pod exec outputs
		if len(def.PodExecs) > 0 {
			// Ensure ceph tools pod is running if any exec targets rook-ceph-tools
			e.ensureCephToolsIfNeeded(ctx, def.PodExecs, logFn)

			logFn("Collecting pod exec outputs...")
			errs := e.collectPodExecs(ctx, def, opts.DestDir, logFn)
			allErrors = append(allErrors, errs...)
		}

		// Collect node command outputs
		if len(def.NodeCommands) > 0 {
			logFn("Collecting node command outputs...")
			errs := e.collectNodeCommands(ctx, def, opts.DestDir, opts.Since, logFn)
			allErrors = append(allErrors, errs...)
		}

		logFn(fmt.Sprintf("=== %s complete ===", label))
	}

	// Write error log if any errors accumulated
	appendErrorLog(opts.DestDir, allErrors)

	return nil
}

// resourceTypeSpec maps user-facing resource type names to their API group/version/resource.
var resourceTypeSpec = map[string]ResourceSpec{
	"pods":                     {Group: "", Version: "v1", Resource: "pods"},
	"services":                 {Group: "", Version: "v1", Resource: "services"},
	"configmaps":               {Group: "", Version: "v1", Resource: "configmaps"},
	"events":                   {Group: "", Version: "v1", Resource: "events"},
	"persistentvolumeclaims":   {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
	"serviceaccounts":          {Group: "", Version: "v1", Resource: "serviceaccounts"},
	"secrets":                  {Group: "", Version: "v1", Resource: "secrets"},
	"endpoints":                {Group: "", Version: "v1", Resource: "endpoints"},
	"deployments":              {Group: "apps", Version: "v1", Resource: "deployments"},
	"statefulsets":             {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"daemonsets":               {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"replicasets":              {Group: "apps", Version: "v1", Resource: "replicasets"},
	"jobs":                     {Group: "batch", Version: "v1", Resource: "jobs"},
	"cronjobs":                 {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"roles":                    {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
	"rolebindings":             {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	"routes":                   {Group: "route.openshift.io", Version: "v1", Resource: "routes"},
	"ingresses":                {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"endpointslices":           {Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
	"networkpolicies":          {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
	"horizontalpodautoscalers": {Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
}

// buildCustomDefinition creates a GatherDefinition from custom gather options.
func buildCustomDefinition(namespaces, resourceTypes []string, includeLogs bool) GatherDefinition {
	def := GatherDefinition{
		DisplayName: "Custom namespace gather",
	}

	for _, rt := range resourceTypes {
		if spec, ok := resourceTypeSpec[rt]; ok {
			spec.Namespaces = namespaces
			def.NamespacedResources = append(def.NamespacedResources, spec)
		}
	}

	// If no resource types selected, collect common defaults
	if len(def.NamespacedResources) == 0 {
		for _, rt := range []string{"pods", "services", "configmaps", "events", "deployments"} {
			spec := resourceTypeSpec[rt]
			spec.Namespaces = namespaces
			def.NamespacedResources = append(def.NamespacedResources, spec)
		}
	}

	if includeLogs {
		def.PodLogs = append(def.PodLogs, PodLogSpec{
			Namespaces: namespaces,
		})
	}

	return def
}

// RunEtcdBackup performs an etcd backup using a debug pod on a master node.
func (e *Engine) RunEtcdBackup(ctx context.Context, destDir string, logFn func(string), stepFn func(int, int, string)) error {
	totalSteps := 3

	// Step 1: Find master node
	stepFn(1, totalSteps, "Finding master node")
	logFn("=== Step 1/3: Finding a master node ===")

	masterNode, err := e.findMasterNode(ctx)
	if err != nil {
		return fmt.Errorf("find master node: %w", err)
	}
	logFn(fmt.Sprintf("Using master node: %s", masterNode))

	// Step 2: Run backup
	stepFn(2, totalSteps, "Running etcd backup on "+masterNode)
	logFn("=== Step 2/3: Running etcd backup ===")

	backupScript := `chroot /host /bin/bash -c '
		BACKUP_DIR=/home/core/etcd-backup-$(date +%Y%m%d-%H%M%S)
		mkdir -p ${BACKUP_DIR}
		/usr/local/bin/cluster-backup.sh ${BACKUP_DIR}
		echo "BACKUP_DIR=${BACKUP_DIR}"
		ls -la ${BACKUP_DIR}/
	'`

	podName, err := e.createDebugPod(ctx, masterNode)
	if err != nil {
		return fmt.Errorf("create debug pod: %w", err)
	}
	defer e.deletePod(ctx, e.namespace, podName)

	if err := e.waitForPod(ctx, e.namespace, podName, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for debug pod: %w", err)
	}

	stdout, stderr, err := e.execInPod(ctx, e.namespace, podName, "debug", []string{"/bin/bash", "-c", backupScript})
	if err != nil {
		logFn(fmt.Sprintf("stderr: %s", string(stderr)))
		return fmt.Errorf("etcd backup: %w", err)
	}
	logFn(string(stdout))

	// Parse backup dir from output
	var backupDir string
	for _, line := range splitLines(string(stdout)) {
		if len(line) > 11 && line[:11] == "BACKUP_DIR=" {
			backupDir = line[11:]
			break
		}
	}
	if backupDir == "" {
		return fmt.Errorf("could not determine backup directory from output")
	}

	// Step 3: Copy backup files
	stepFn(3, totalSteps, "Copying backup files")
	logFn("=== Step 3/3: Copying backup files from node ===")

	tarCmd := []string{"/bin/bash", "-c", fmt.Sprintf("chroot /host tar czf - -C %q .", backupDir)}
	tarData, _, err := e.execInPod(ctx, e.namespace, podName, "debug", tarCmd)
	if err != nil {
		return fmt.Errorf("copy backup: %w", err)
	}

	tarFile := filepath.Join(destDir, "etcd-backup.tar.gz")
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return err
	}
	if err := writeFile(destDir, tarFile, tarData); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}

	// Cleanup backup dir on node
	cleanupCmd := []string{"/bin/bash", "-c", fmt.Sprintf("chroot /host rm -rf %q", backupDir)}
	_, _, _ = e.execInPod(ctx, e.namespace, podName, "debug", cleanupCmd)

	logFn(fmt.Sprintf("Backup archive created: %s", filepath.Base(tarFile)))
	return nil
}

// findMasterNode returns the name of the first master node.
func (e *Engine) findMasterNode(ctx context.Context) (string, error) {
	items, err := e.k8sClient.GetList("/api/v1/nodes?labelSelector=node-role.kubernetes.io/master=")
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no master nodes found")
	}
	return k8s.JsonPath(items[0], "metadata", "name"), nil
}

// ensureCephToolsIfNeeded checks if any PodExec targets the rook-ceph-tools pod.
// If so, it patches the StorageCluster to enable ceph tools and waits for the pod.
func (e *Engine) ensureCephToolsIfNeeded(ctx context.Context, execs []PodExecSpec, logFn func(string)) {
	needsTools := false
	for _, spec := range execs {
		if spec.PodSelector == "app=rook-ceph-tools" {
			needsTools = true
			break
		}
	}
	if !needsTools {
		return
	}

	// Check if tools pod already exists
	items, err := e.k8sClient.GetList("/api/v1/namespaces/openshift-storage/pods?labelSelector=app=rook-ceph-tools")
	if err == nil && len(items) > 0 {
		phase := k8s.JsonPath(items[0], "status", "phase")
		if phase == "Running" {
			return
		}
	}

	// Patch StorageCluster to enable ceph tools
	logFn("Enabling ceph tools pod on StorageCluster...")
	scPath := "/apis/ocs.openshift.io/v1/namespaces/openshift-storage/storageclusters/ocs-storagecluster"
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"enableCephTools": true,
		},
	}
	if _, err := e.k8sClient.Patch(scPath, patch); err != nil {
		logFn(fmt.Sprintf("Warning: failed to enable ceph tools: %v", err))
		return
	}

	// Wait for the tools pod to become ready (up to 2 minutes)
	logFn("Waiting for ceph tools pod to start...")
	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			logFn("Warning: timed out waiting for ceph tools pod")
			return
		case <-ticker.C:
			items, err := e.k8sClient.GetList("/api/v1/namespaces/openshift-storage/pods?labelSelector=app=rook-ceph-tools")
			if err == nil && len(items) > 0 {
				phase := k8s.JsonPath(items[0], "status", "phase")
				if phase == "Running" {
					logFn("Ceph tools pod is running")
					return
				}
			}
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
