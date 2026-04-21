package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"go.yaml.in/yaml/v2"
)

// collectClusterResources fetches cluster-scoped resources and writes them to destDir.
func (e *Engine) collectClusterResources(ctx context.Context, def GatherDefinition, destDir string, log func(string)) []error {
	var errs []error
	for _, spec := range def.ClusterResources {
		if ctx.Err() != nil {
			return append(errs, ctx.Err())
		}

		path := spec.APIPath("")
		if spec.LabelSelector != "" {
			path += "?labelSelector=" + spec.LabelSelector
		}

		items, err := e.k8sClient.GetList(path)
		if err != nil {
			if k8s.IsNotFound(err) {
				continue // CRD doesn't exist on this cluster
			}
			if k8s.IsForbidden(err) {
				log(fmt.Sprintf("  Warning: no permission for %s/%s", spec.Group, spec.Resource))
				errs = append(errs, err)
				continue
			}
			log(fmt.Sprintf("  Warning: failed to list %s: %v", spec.Resource, err))
			errs = append(errs, err)
			continue
		}

		log(fmt.Sprintf("  %s: %d items", spec.Resource, len(items)))

		for _, item := range items {
			name := k8s.JsonPath(item, "metadata", "name")
			if name == "" {
				continue
			}

			yamlData, err := toYAML(item)
			if err != nil {
				errs = append(errs, fmt.Errorf("marshal %s/%s: %w", spec.Resource, name, err))
				continue
			}

			filePath := clusterResourcePath(destDir, spec.GroupDir(), spec.Resource, name)
			if err := writeFile(destDir, filePath, yamlData); err != nil {
				errs = append(errs, fmt.Errorf("write %s/%s: %w", spec.Resource, name, err))
			}
		}
	}
	return errs
}

// collectNamespacedResources fetches namespaced resources and writes them to destDir.
func (e *Engine) collectNamespacedResources(ctx context.Context, def GatherDefinition, destDir string, log func(string)) []error {
	var errs []error
	for _, spec := range def.NamespacedResources {
		if ctx.Err() != nil {
			return append(errs, ctx.Err())
		}

		namespaces, err := e.resolveNamespaces(ctx, spec.Namespaces)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		for _, ns := range namespaces {
			if ctx.Err() != nil {
				return append(errs, ctx.Err())
			}

			path := spec.APIPath(ns)
			if spec.LabelSelector != "" {
				path += "?labelSelector=" + spec.LabelSelector
			}

			items, err := e.k8sClient.GetList(path)
			if err != nil {
				if k8s.IsNotFound(err) {
					continue
				}
				if k8s.IsForbidden(err) {
					log(fmt.Sprintf("  Warning: no permission for %s/%s in %s", spec.Group, spec.Resource, ns))
					errs = append(errs, err)
					continue
				}
				errs = append(errs, fmt.Errorf("list %s in %s: %w", spec.Resource, ns, err))
				continue
			}

			if len(items) == 0 {
				continue
			}
			log(fmt.Sprintf("  %s/%s: %d items", ns, spec.Resource, len(items)))

			for _, item := range items {
				name := k8s.JsonPath(item, "metadata", "name")
				if name == "" {
					continue
				}

				yamlData, err := toYAML(item)
				if err != nil {
					errs = append(errs, fmt.Errorf("marshal %s/%s/%s: %w", ns, spec.Resource, name, err))
					continue
				}

				filePath := namespacedResourcePath(destDir, ns, spec.GroupDir(), spec.Resource, name)
				if err := writeFile(destDir, filePath, yamlData); err != nil {
					errs = append(errs, fmt.Errorf("write %s/%s/%s: %w", ns, spec.Resource, name, err))
				}

				// For pods, also write under pods/{pod}/{pod}.yaml
				// to match the traditional oc adm inspect layout
				if spec.Resource == "pods" {
					podPath := podYAMLPath(destDir, ns, name)
					_ = writeFile(destDir, podPath, yamlData)
				}
			}
		}
	}
	return errs
}

// resolveNamespaces expands namespace patterns (including globs like "openshift-*") to actual namespace names.
func (e *Engine) resolveNamespaces(ctx context.Context, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	// Check if any patterns contain wildcards
	hasGlob := false
	for _, p := range patterns {
		if strings.Contains(p, "*") || strings.Contains(p, "?") {
			hasGlob = true
			break
		}
	}

	if !hasGlob {
		return patterns, nil
	}

	// Fetch all namespaces and match against patterns
	items, err := e.k8sClient.GetList("/api/v1/namespaces")
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	var result []string
	for _, item := range items {
		name := k8s.JsonPath(item, "metadata", "name")
		if name == "" {
			continue
		}
		for _, pattern := range patterns {
			if matched, _ := filepath.Match(pattern, name); matched {
				result = append(result, name)
				break
			}
		}
	}

	return result, nil
}

// toYAML converts a JSON-decoded map to YAML bytes.
func toYAML(data map[string]interface{}) ([]byte, error) {
	// Clean up managed fields to reduce noise
	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		delete(metadata, "managedFields")
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	return yamlBytes, nil
}

// appendErrorLog writes accumulated errors to gather-errors.log.
func appendErrorLog(destDir string, errs []error) {
	if len(errs) == 0 {
		return
	}
	var lines []string
	for _, err := range errs {
		lines = append(lines, err.Error())
	}
	content := strings.Join(lines, "\n") + "\n"
	path := filepath.Join(destDir, "gather-errors.log")
	_ = writeFile(destDir, path, []byte(content))
}

// toJSON converts a Go value to indented JSON bytes.
func toJSON(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
