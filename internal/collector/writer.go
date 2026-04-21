package collector

import (
	"io"
	"os"
	"path/filepath"
)

// writeFile creates parent directories and writes data.
// All path components must be pre-sanitized with filepath.Base by the path helpers.
func writeFile(path string, data []byte) error {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0700); err != nil {
		return err
	}
	return os.WriteFile(clean, data, 0600)
}

// writeStream creates parent directories and streams data.
// All path components must be pre-sanitized with filepath.Base by the path helpers.
func writeStream(path string, r io.Reader) error {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0700); err != nil {
		return err
	}
	f, err := os.Create(clean)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// clusterResourcePath returns the file path for a cluster-scoped resource.
func clusterResourcePath(destDir, group, resource, name string) string {
	groupDir := filepath.Base(group)
	if group == "" {
		groupDir = "core"
	}
	return filepath.Join(destDir, "cluster-scoped-resources", groupDir, filepath.Base(resource), filepath.Base(name)+".yaml")
}

// namespacedResourcePath returns the file path for a namespaced resource.
func namespacedResourcePath(destDir, namespace, group, resource, name string) string {
	groupDir := filepath.Base(group)
	if group == "" {
		groupDir = "core"
	}
	return filepath.Join(destDir, "namespaces", filepath.Base(namespace), groupDir, filepath.Base(resource), filepath.Base(name)+".yaml")
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
	return filepath.Join(destDir, "namespaces", filepath.Base(namespace), "pods", filepath.Base(pod), filepath.Base(container), filepath.Base(container), "logs", logFile)
}

// podYAMLPath returns the file path for a pod's YAML resource.
// Traditional must-gather stores pod YAML under pods/{pod}/{pod}.yaml
// in addition to the standard {group}/{resource}/{name}.yaml location.
func podYAMLPath(destDir, namespace, pod string) string {
	return filepath.Join(destDir, "namespaces", filepath.Base(namespace), "pods", filepath.Base(pod), filepath.Base(pod)+".yaml")
}

// execOutputPath returns the file path for a pod exec output.
func execOutputPath(destDir, namespace, pod, outputFile string) string {
	return filepath.Join(destDir, "exec-outputs", filepath.Base(namespace), filepath.Base(pod), filepath.Base(outputFile))
}

// nodeOutputPath returns the file path for a node command output.
func nodeOutputPath(destDir, nodeName, fileName string) string {
	return filepath.Join(destDir, "host-collected", filepath.Base(nodeName), filepath.Base(fileName))
}
