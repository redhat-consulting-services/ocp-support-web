package collector

import (
	"bytes"
	"context"
	"fmt"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// collectPodExecs runs exec commands in pods and writes output to destDir.
func (e *Engine) collectPodExecs(ctx context.Context, def GatherDefinition, destDir string, log func(string)) []error {
	var errs []error

	for _, spec := range def.PodExecs {
		if ctx.Err() != nil {
			return append(errs, ctx.Err())
		}

		// Find the target pod
		path := fmt.Sprintf("/api/v1/namespaces/%s/pods", spec.Namespace)
		if spec.PodSelector != "" {
			path += "?labelSelector=" + spec.PodSelector
		}

		items, err := e.k8sClient.GetList(path)
		if err != nil {
			if k8s.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("find pod for exec %s: %w", spec.Name, err))
			continue
		}

		if len(items) == 0 {
			log(fmt.Sprintf("  Skipping exec %s: no matching pods in %s", spec.Name, spec.Namespace))
			continue
		}

		// Use the first matching pod
		podName := k8s.JsonPath(items[0], "metadata", "name")
		container := spec.Container
		if container == "" {
			// Default to first container
			containers := k8s.JsonArray(items[0], "spec", "containers")
			if len(containers) > 0 {
				if cm, ok := containers[0].(map[string]interface{}); ok {
					container = k8s.StringOrEmpty(cm, "name")
				}
			}
		}

		log(fmt.Sprintf("  Exec %s in %s/%s (%s)", spec.Name, spec.Namespace, podName, container))

		stdout, stderr, err := e.execInPod(ctx, spec.Namespace, podName, container, spec.Command)
		if err != nil {
			log(fmt.Sprintf("  Warning: exec %s failed: %v", spec.Name, err))
			if len(stderr) > 0 {
				log(fmt.Sprintf("  stderr: %s", string(stderr)))
			}
			errs = append(errs, fmt.Errorf("exec %s: %w", spec.Name, err))
			continue
		}

		outputFile := spec.OutputFile
		if outputFile == "" {
			outputFile = spec.Name + ".txt"
		}
		filePath := execOutputPath(destDir, spec.Namespace, podName, outputFile)
		if err := writeFile(destDir, filePath, stdout); err != nil {
			errs = append(errs, fmt.Errorf("write exec output %s: %w", spec.Name, err))
		}
	}

	return errs
}

// execInPod executes a command in a pod container using SPDY via client-go.
func (e *Engine) execInPod(ctx context.Context, namespace, pod, container string, command []string) ([]byte, []byte, error) {
	cfg := &rest.Config{
		Host:        e.k8sClient.APIURL,
		BearerToken: e.k8sClient.Token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: e.k8sClient.HTTPClient.Transport != nil,
		},
	}

	restClient, err := rest.RESTClientFor(&rest.Config{
		Host:        cfg.Host,
		BearerToken: cfg.BearerToken,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: cfg.TLSClientConfig.Insecure,
		},
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &corev1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create REST client: %w", err)
	}

	req := restClient.Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		Param("container", container).
		Param("stdout", "true").
		Param("stderr", "true")

	for _, c := range command {
		req = req.Param("command", c)
	}

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return nil, nil, fmt.Errorf("create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("exec stream: %w", err)
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}
