package collector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

// collectNodeCommands runs commands on nodes via debug pods and writes output to destDir.
func (e *Engine) collectNodeCommands(ctx context.Context, def GatherDefinition, destDir, since string, log func(string)) []error {
	var errs []error

	// Group commands by node selector to minimize debug pods
	type nodeGroup struct {
		selector string
		commands []NodeCommandSpec
	}
	groups := map[string]*nodeGroup{}
	for _, cmd := range def.NodeCommands {
		g, ok := groups[cmd.NodeSelector]
		if !ok {
			g = &nodeGroup{selector: cmd.NodeSelector}
			groups[cmd.NodeSelector] = g
		}
		g.commands = append(g.commands, cmd)
	}

	for _, group := range groups {
		if ctx.Err() != nil {
			return append(errs, ctx.Err())
		}

		// Find matching nodes
		path := "/api/v1/nodes"
		if group.selector != "" {
			path += "?labelSelector=" + group.selector
		}
		items, err := e.k8sClient.GetList(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("list nodes for selector %s: %w", group.selector, err))
			continue
		}

		for _, nodeItem := range items {
			if ctx.Err() != nil {
				return append(errs, ctx.Err())
			}

			nodeName := k8s.JsonPath(nodeItem, "metadata", "name")
			if nodeName == "" {
				continue
			}

			log(fmt.Sprintf("  Creating debug pod on %s (namespace: %s)", nodeName, e.namespace))
			podName, err := e.createDebugPod(ctx, nodeName)
			if err != nil {
				log(fmt.Sprintf("  Warning: failed to create debug pod on %s: %v", nodeName, err))
				errs = append(errs, fmt.Errorf("create debug pod on %s: %w", nodeName, err))
				continue
			}

			if err := e.waitForPod(ctx, e.namespace, podName, 2*time.Minute); err != nil {
				log(fmt.Sprintf("  Warning: debug pod on %s failed to start: %v", nodeName, err))
				e.deletePod(ctx, e.namespace, podName)
				errs = append(errs, fmt.Errorf("wait for debug pod on %s: %w", nodeName, err))
				continue
			}

			for _, cmd := range group.commands {
				if ctx.Err() != nil {
					e.deletePod(ctx, e.namespace, podName)
					return append(errs, ctx.Err())
				}

				log(fmt.Sprintf("  Running %s on %s", cmd.Name, nodeName))
				command := applyJournalSince(cmd.Command, since)
				stdout, stderr, err := e.execInPod(ctx, e.namespace, podName, "debug", command)
				if err != nil {
					log(fmt.Sprintf("  Warning: %s on %s failed: %v", cmd.Name, nodeName, err))
					if len(stderr) > 0 {
						log(fmt.Sprintf("  stderr: %s", string(stderr)))
					}
					errs = append(errs, fmt.Errorf("%s on %s: %w", cmd.Name, nodeName, err))
					continue
				}

				filePath := nodeOutputPath(destDir, nodeName, cmd.OutputFile)
				if err := writeFile(filePath, stdout); err != nil {
					errs = append(errs, fmt.Errorf("write %s output for %s: %w", cmd.Name, nodeName, err))
				}
			}

			e.deletePod(ctx, e.namespace, podName)
		}
	}

	return errs
}

// createDebugPod creates a privileged debug pod on a specific node.
func (e *Engine) createDebugPod(ctx context.Context, nodeName string) (string, error) {
	podName := fmt.Sprintf("ocp-support-debug-%s-%d", sanitizeNodeName(nodeName), time.Now().UnixMilli())

	pod := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      podName,
			"namespace": e.namespace,
			"labels": map[string]interface{}{
				"app": "ocp-support-debug",
			},
		},
		"spec": map[string]interface{}{
			"nodeName":      nodeName,
			"hostPID":       true,
			"hostNetwork":   true,
			"restartPolicy": "Never",
			"containers": []map[string]interface{}{
				{
					"name":    "debug",
					"image":   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
					"command": []string{"sleep", "600"},
					"securityContext": map[string]interface{}{
						"privileged": true,
					},
					"volumeMounts": []map[string]interface{}{
						{
							"name":      "host",
							"mountPath": "/host",
						},
					},
				},
			},
			"volumes": []map[string]interface{}{
				{
					"name": "host",
					"hostPath": map[string]interface{}{
						"path": "/",
					},
				},
			},
		},
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods", e.namespace)
	_, err := e.k8sClient.Post(path, pod)
	if err != nil {
		return "", err
	}

	return podName, nil
}

// waitForPod polls until the pod is Running or fails.
func (e *Engine) waitForPod(ctx context.Context, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", namespace, name)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		data, err := e.k8sClient.Get(path)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		phase := k8s.JsonPath(data, "status", "phase")
		switch phase {
		case "Running":
			return nil
		case "Failed", "Succeeded":
			return fmt.Errorf("pod %s in phase %s", name, phase)
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for pod %s to be Running", name)
}

// deletePod deletes a pod.
func (e *Engine) deletePod(ctx context.Context, namespace, name string) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", namespace, name)
	_ = e.k8sClient.Delete(path)
}

// applyJournalSince modifies journalctl commands to use --since when a time filter is set.
// For journalctl commands, it replaces "-n XXXX" with "--since 'X hours ago'" when since is non-empty.
func applyJournalSince(command []string, since string) []string {
	if since == "" || len(command) < 2 {
		return command
	}
	// Only apply to journalctl commands
	isJournalctl := false
	for _, arg := range command {
		if strings.HasSuffix(arg, "journalctl") || arg == "journalctl" {
			isJournalctl = true
			break
		}
	}
	if !isJournalctl {
		return command
	}

	hours, err := strconv.ParseInt(strings.TrimSuffix(since, "h"), 10, 64)
	if err != nil || hours <= 0 {
		return command
	}

	// Replace -n XXXX with --since
	result := make([]string, 0, len(command))
	skip := false
	for i, arg := range command {
		if skip {
			skip = false
			continue
		}
		if arg == "-n" && i+1 < len(command) {
			// Skip -n and its value, we'll add --since instead
			skip = true
			continue
		}
		result = append(result, arg)
	}
	result = append(result, "--since", fmt.Sprintf("%d hours ago", hours))
	return result
}

// sanitizeNodeName returns a safe string for use in pod names.
func sanitizeNodeName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		} else {
			result = append(result, '-')
		}
	}
	if len(result) > 40 {
		result = result[:40]
	}
	return string(result)
}
