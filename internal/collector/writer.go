package collector

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// safePath validates that the resolved path stays within baseDir.
func safePath(baseDir, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base: %w", err)
	}
	if !strings.HasPrefix(abs, base+string(filepath.Separator)) && abs != base {
		return "", fmt.Errorf("path %q escapes base directory", path)
	}
	return abs, nil
}

// writeFile creates parent directories and writes data to the given path.
// The path must resolve within baseDir.
func writeFile(baseDir, path string, data []byte) error {
	safe, err := safePath(baseDir, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(safe), 0700); err != nil {
		return err
	}
	return os.WriteFile(safe, data, 0600)
}

// writeStream creates parent directories and streams data to the given path.
// The path must resolve within baseDir.
func writeStream(baseDir, path string, r io.Reader) error {
	safe, err := safePath(baseDir, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(safe), 0700); err != nil {
		return err
	}
	f, err := os.Create(safe)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// clusterResourcePath returns the file path for a cluster-scoped resource.
func clusterResourcePath(destDir, group, resource, name string) string {
	groupDir := group
	if groupDir == "" {
		groupDir = "core"
	}
	return filepath.Join(destDir, "cluster-scoped-resources", groupDir, resource, name+".yaml")
}

// namespacedResourcePath returns the file path for a namespaced resource.
func namespacedResourcePath(destDir, namespace, group, resource, name string) string {
	groupDir := group
	if groupDir == "" {
		groupDir = "core"
	}
	return filepath.Join(destDir, "namespaces", namespace, groupDir, resource, name+".yaml")
}

// podLogPath returns the file path for a pod log.
// Matches the traditional `oc adm inspect` format:
//
//	namespaces/{ns}/pods/{pod}/{container}/{container}/logs/{current|previous}.log
func podLogPath(destDir, namespace, pod, container string, current bool) string {
	logFile := "current.log"
	if !current {
		logFile = "previous.log"
	}
	// Traditional must-gather duplicates the container name in the path
	return filepath.Join(destDir, "namespaces", namespace, "pods", pod, container, container, "logs", logFile)
}

// podYAMLPath returns the file path for a pod's YAML resource.
// Traditional must-gather stores pod YAML under pods/{pod}/{pod}.yaml
// in addition to the standard {group}/{resource}/{name}.yaml location.
func podYAMLPath(destDir, namespace, pod string) string {
	return filepath.Join(destDir, "namespaces", namespace, "pods", pod, pod+".yaml")
}

// execOutputPath returns the file path for a pod exec output.
func execOutputPath(destDir, namespace, pod, outputFile string) string {
	return filepath.Join(destDir, "exec-outputs", namespace, pod, outputFile)
}

// nodeOutputPath returns the file path for a node command output.
func nodeOutputPath(destDir, nodeName, fileName string) string {
	return filepath.Join(destDir, "host-collected", nodeName, fileName)
}
