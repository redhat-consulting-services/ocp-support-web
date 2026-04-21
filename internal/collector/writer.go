package collector

import (
	"io"
	"os"
	"path/filepath"
)

// writeFile creates parent directories and writes data to the given path.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// writeStream creates parent directories and streams data to the given path.
func writeStream(path string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.Create(path)
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
