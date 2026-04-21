package collector

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

// collectPodLogs fetches pod logs based on the definition and writes them to destDir.
func (e *Engine) collectPodLogs(ctx context.Context, def GatherDefinition, destDir, since string, log func(string)) []error {
	var errs []error

	sinceSeconds := parseSinceDuration(since)

	for _, spec := range def.PodLogs {
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

			// List pods in namespace
			path := "/api/v1/namespaces/" + ns + "/pods"
			if spec.LabelSelector != "" {
				path += "?labelSelector=" + spec.LabelSelector
			}

			items, err := e.k8sClient.GetList(path)
			if err != nil {
				if k8s.IsNotFound(err) {
					continue
				}
				errs = append(errs, fmt.Errorf("list pods in %s: %w", ns, err))
				continue
			}

			for _, pod := range items {
				podName := k8s.JsonPath(pod, "metadata", "name")
				if podName == "" {
					continue
				}

				// Get container names from the pod spec
				containers := k8s.JsonArray(pod, "spec", "containers")
				for _, c := range containers {
					cm, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					containerName := k8s.StringOrEmpty(cm, "name")
					if containerName == "" {
						continue
					}

					errs = append(errs, e.fetchPodLog(ctx, destDir, ns, podName, containerName, true, sinceSeconds, spec.MaxLines)...)

					if spec.Previous {
						errs = append(errs, e.fetchPodLog(ctx, destDir, ns, podName, containerName, false, sinceSeconds, spec.MaxLines)...)
					}
				}

				// Also collect init container logs
				initContainers := k8s.JsonArray(pod, "spec", "initContainers")
				for _, c := range initContainers {
					cm, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					containerName := k8s.StringOrEmpty(cm, "name")
					if containerName == "" {
						continue
					}
					errs = append(errs, e.fetchPodLog(ctx, destDir, ns, podName, containerName, true, sinceSeconds, spec.MaxLines)...)
				}
			}

			log(fmt.Sprintf("  %s: collected logs from %d pods", ns, len(items)))
		}
	}
	return errs
}

// fetchPodLog fetches a single container's log and writes it to disk.
func (e *Engine) fetchPodLog(ctx context.Context, destDir, namespace, pod, container string, current bool, sinceSeconds int64, maxLines int) []error {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log?container=%s", namespace, pod, container)

	if !current {
		path += "&previous=true"
	}
	if sinceSeconds > 0 {
		path += "&sinceSeconds=" + strconv.FormatInt(sinceSeconds, 10)
	}
	if maxLines > 0 {
		path += "&tailLines=" + strconv.Itoa(maxLines)
	}

	stream, err := e.k8sClient.GetStream(path)
	if err != nil {
		if k8s.IsNotFound(err) {
			return nil // Pod/container may have been deleted
		}
		// Previous logs may not exist — not an error
		if !current {
			return nil
		}
		return []error{fmt.Errorf("log %s/%s/%s: %w", namespace, pod, container, err)}
	}
	defer stream.Close()

	filePath := podLogPath(destDir, namespace, pod, container, current)
	if err := writeStream(filePath, stream); err != nil {
		return []error{fmt.Errorf("write log %s/%s/%s: %w", namespace, pod, container, err)}
	}

	return nil
}

// parseSinceDuration converts a "Xh" duration string to seconds.
func parseSinceDuration(since string) int64 {
	if since == "" {
		return 0
	}
	// Expected format: "48h", "24h", etc.
	if len(since) < 2 || since[len(since)-1] != 'h' {
		return 0
	}
	hours, err := strconv.ParseInt(since[:len(since)-1], 10, 64)
	if err != nil {
		return 0
	}
	return hours * 3600
}
