package acm

import (
	"archive/tar"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"github.com/redhat-consulting-services/ocp-support-web/internal/mustgather"
)

// ManagedCluster represents an ACM managed cluster.
type ManagedCluster struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "Available", "Unavailable", "Unknown"
	OCPVersion string `json:"ocpVersion,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	APIURL     string `json:"apiURL,omitempty"`
	AgentDeployed bool   `json:"agentDeployed"`
	AgentReady    bool   `json:"agentReady"`
	AgentVersion  string `json:"agentVersion,omitempty"`
}

// RemoteGatherJob tracks a must-gather deployed to a managed cluster.
type RemoteGatherJob struct {
	ID          string    `json:"id"`
	ClusterName string    `json:"clusterName"`
	GatherType  string    `json:"gatherType"`
	Status      string    `json:"status"` // "gathering", "downloading", "anonymizing", "complete", "failed"
	StartedAt   time.Time `json:"startedAt"`
	Error       string    `json:"error,omitempty"`
	LogOutput   string    `json:"logOutput,omitempty"`
	Anonymize   bool      `json:"anonymize"`
	AnonOpts    mustgather.AnonOptions `json:"anonOpts,omitempty"`
	FilePath    string    `json:"-"`
	FileName    string    `json:"fileName,omitempty"`
}

// cachedAccess stores a reusable k8s client and pod name for a managed cluster agent.
type cachedAccess struct {
	client  *k8s.Client
	podName string
	expires time.Time
}

// Client provides access to ACM multi-cluster APIs and manages remote agents.
type Client struct {
	k8s           *k8s.Client
	workDir       string
	clusterDomain string
	agentImage    string

	mu            sync.Mutex
	jobs          map[string]*RemoteGatherJob
	agents        map[string]bool // clusterName → ManifestWork deployed
	agentsSynced  bool            // true after first EnsureAgents reconciliation
	accessCache   map[string]*cachedAccess
}

const (
	agentNamespace    = "ocp-support-web-agent"
	manifestWorkName  = "ocp-support-agent"
	agentDeployName   = "ocp-support-agent"
	agentSAName       = "must-gather-sa"
	agentTokenSecret  = "must-gather-sa-token"
)

// NewClient creates an ACM client.
func NewClient(k8sClient *k8s.Client, workDir, clusterDomain string) *Client {
	agentImage := os.Getenv("AGENT_IMAGE")
	if agentImage == "" {
		agentImage = detectOwnImage(k8sClient)
		// The auto-detected image uses the internal registry URL which managed clusters can't reach.
		// Replace it with the external registry route so managed clusters can pull the image.
		if strings.Contains(agentImage, "image-registry.openshift-image-registry.svc") {
			externalRoute := detectRegistryRoute(k8sClient)
			if externalRoute != "" {
				agentImage = strings.Replace(agentImage, "image-registry.openshift-image-registry.svc:5000", externalRoute, 1)
			}
		}
	}
	if agentImage == "" {
		log.Printf("WARNING: Could not detect agent image. Set AGENT_IMAGE env var or ensure imagestream exists.")
	}
	log.Printf("ACM agent image: %s", agentImage)

	return &Client{
		k8s:           k8sClient,
		workDir:       workDir,
		clusterDomain: clusterDomain,
		agentImage:    agentImage,
		jobs:          make(map[string]*RemoteGatherJob),
		agents:        make(map[string]bool),
		accessCache:   make(map[string]*cachedAccess),
	}
}

// detectOwnImage reads this pod's container image and resolves it to a digest-pinned reference.
func detectOwnImage(k8sClient *k8s.Client) string {
	ns, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	namespace := strings.TrimSpace(string(ns))
	if namespace == "" {
		return ""
	}

	// Read the image reference from the imagestream tag (always has the digest)
	istagPath := fmt.Sprintf("/apis/image.openshift.io/v1/namespaces/%s/imagestreamtags/ocp-support-web:latest", namespace)
	data, err := k8sClient.Get(istagPath)
	if err != nil {
		log.Printf("Could not read imagestream tag: %v", err)
		return ""
	}

	// image.dockerImageReference has the full digest reference
	dockerRef := k8s.JsonPath(data, "image", "dockerImageReference")
	if dockerRef != "" {
		log.Printf("Auto-detected agent image from imagestream: %s", dockerRef)
		return dockerRef
	}
	return ""
}

// detectRegistryRoute reads the external route for the OpenShift internal image registry.
func detectRegistryRoute(k8sClient *k8s.Client) string {
	data, err := k8sClient.Get("/apis/route.openshift.io/v1/namespaces/openshift-image-registry/routes/default-route")
	if err != nil {
		log.Printf("Could not detect registry route: %v", err)
		return ""
	}
	host := k8s.JsonPath(data, "spec", "host")
	if host != "" {
		log.Printf("Detected external registry route: %s", host)
	}
	return host
}


// ListManagedClusters returns all ACM managed clusters with their status.
func (c *Client) ListManagedClusters() ([]ManagedCluster, error) {
	data, err := c.k8s.Get("/apis/cluster.open-cluster-management.io/v1/managedclusters")
	if err != nil {
		return nil, fmt.Errorf("list managed clusters: %w", err)
	}

	items := k8s.JsonArray(data, "items")
	var clusters []ManagedCluster
	for _, item := range items {
		mc, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := k8s.JsonPath(mc, "metadata", "name")
		if name == "local-cluster" {
			continue // skip the hub cluster itself
		}

		cluster := ManagedCluster{Name: name, Status: "Unknown"}

		// Extract status from conditions and clusterClaims
		if status, ok := mc["status"].(map[string]interface{}); ok {
			if conditions, ok := status["conditions"].([]interface{}); ok {
				for _, cond := range conditions {
					c, ok := cond.(map[string]interface{})
					if !ok {
						continue
					}
					if k8s.StringOrEmpty(c, "type") == "ManagedClusterConditionAvailable" {
						if k8s.StringOrEmpty(c, "status") == "True" {
							cluster.Status = "Available"
						} else {
							cluster.Status = "Unavailable"
						}
					}
				}
			}
			// Extract info from clusterClaims (most reliable source)
			if claims, ok := status["clusterClaims"].([]interface{}); ok {
				for _, claim := range claims {
					cm, ok := claim.(map[string]interface{})
					if !ok {
						continue
					}
					claimName := k8s.StringOrEmpty(cm, "name")
					value := k8s.StringOrEmpty(cm, "value")
					switch claimName {
					case "platform.open-cluster-management.io":
						if cluster.Platform == "" {
							cluster.Platform = value
						}
					case "product.open-cluster-management.io":
						cluster.Vendor = value
					case "version.openshift.io":
						cluster.OCPVersion = value
					}
				}
			}
		}

		// Extract platform and vendor from labels as fallback
		labels := k8s.JsonMap(mc, "metadata", "labels")
		if labels != nil {
			if cluster.Platform == "" {
				if p, ok := labels["platform.open-cluster-management.io"].(string); ok {
					cluster.Platform = p
				}
			}
			if cluster.Platform == "" {
				if p, ok := labels["cloud"].(string); ok {
					cluster.Platform = p
				}
			}
			if cluster.Vendor == "" {
				if v, ok := labels["vendor"].(string); ok {
					cluster.Vendor = v
				}
			}
			if cluster.OCPVersion == "" {
				if v, ok := labels["openshiftVersion"].(string); ok {
					cluster.OCPVersion = v
				}
			}
		}

		// Extract API URL from spec
		cluster.APIURL = k8s.JsonPath(mc, "spec", "managedClusterClientConfigs", "url")
		if cluster.APIURL == "" {
			if configs := k8s.JsonArray(mc, "spec", "managedClusterClientConfigs"); len(configs) > 0 {
				if cfg, ok := configs[0].(map[string]interface{}); ok {
					cluster.APIURL = k8s.StringOrEmpty(cfg, "url")
				}
			}
		}

		// Check if agent is deployed
		cluster.AgentDeployed, cluster.AgentReady, cluster.AgentVersion = c.agentStatus(name)

		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

// CountManagedClusters returns the number of managed clusters (excluding local-cluster).
func (c *Client) CountManagedClusters() int {
	clusters, err := c.ListManagedClusters()
	if err != nil {
		return 0
	}
	return len(clusters)
}

// EnsureAgents deploys the agent ManifestWork to all available managed clusters that don't have one.
func (c *Client) EnsureAgents(clusters []ManagedCluster) {
	c.mu.Lock()
	synced := c.agentsSynced
	c.mu.Unlock()

	for _, cluster := range clusters {
		if cluster.Status != "Available" {
			continue
		}
		// Skip if already deployed (unless this is the first sync — always reconcile once)
		if synced {
			c.mu.Lock()
			deployed := c.agents[cluster.Name]
			c.mu.Unlock()
			if deployed {
				continue
			}
		}

		go func(name string) {
			if err := c.DeployAgent(name); err != nil {
				log.Printf("Failed to deploy agent to %s: %v", name, err)
			}
		}(cluster.Name)
	}

	if !synced {
		c.mu.Lock()
		c.agentsSynced = true
		c.mu.Unlock()
	}
}

// clusterAgentImage returns the agent image for a specific cluster.
// If the ManagedCluster has an ocp-support-web/agent-image annotation, that value is used.
// Otherwise the global agentImage is returned.
func (c *Client) clusterAgentImage(clusterName string) string {
	mcPath := fmt.Sprintf("/apis/cluster.open-cluster-management.io/v1/managedclusters/%s", clusterName)
	data, err := c.k8s.Get(mcPath)
	if err == nil {
		annotations := k8s.JsonMap(data, "metadata", "annotations")
		if annotations != nil {
			if img, ok := annotations["ocp-support-web/agent-image"].(string); ok && img != "" {
				return img
			}
		}
	}
	return c.agentImage
}

// DeployAgent creates or updates the persistent agent ManifestWork on a managed cluster.
func (c *Client) DeployAgent(clusterName string) error {
	path := fmt.Sprintf("/apis/work.open-cluster-management.io/v1/namespaces/%s/manifestworks/%s", clusterName, manifestWorkName)
	existing, existsErr := c.k8s.Get(path)

	agentImage := c.clusterAgentImage(clusterName)

	manifests := []interface{}{
		// Namespace
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": agentNamespace,
				"labels": map[string]interface{}{
					"pod-security.kubernetes.io/enforce": "privileged",
					"pod-security.kubernetes.io/audit":   "privileged",
					"pod-security.kubernetes.io/warn":    "privileged",
				},
			},
		},
		// ServiceAccount
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name":      agentSAName,
				"namespace": agentNamespace,
			},
		},
		// ClusterRoleBinding
		map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "ocp-support-agent-admin",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "cluster-admin",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      agentSAName,
					"namespace": agentNamespace,
				},
			},
		},
		// SA token Secret (needed for hub to authenticate to managed cluster API)
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      agentTokenSecret,
				"namespace": agentNamespace,
				"annotations": map[string]interface{}{
					"kubernetes.io/service-account.name": agentSAName,
				},
			},
			"type": "kubernetes.io/service-account-token",
		},
		// Deployment
		map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      agentDeployName,
				"namespace": agentNamespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ocp-support-web",
					"app":                          "ocp-support-agent",
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "ocp-support-agent",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "ocp-support-agent",
						},
					},
					"spec": map[string]interface{}{
						"serviceAccountName": agentSAName,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "agent",
								"image":           agentImage,
								"imagePullPolicy": "Always",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "AGENT_MODE",
										"value": "true",
									},
								},
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 8080,
										"name":          "http",
										"protocol":      "TCP",
									},
								},
								"livenessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/healthz",
										"port": 8080,
									},
									"initialDelaySeconds": 10,
									"periodSeconds":       30,
								},
								"readinessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/healthz",
										"port": 8080,
									},
									"initialDelaySeconds": 5,
									"periodSeconds":       10,
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"cpu":    "10m",
										"memory": "32Mi",
									},
									"limits": map[string]interface{}{
										"memory": "256Mi",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// NetworkPolicy: deny all ingress except from kube-apiserver (pod proxy)
	manifests = append(manifests, map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]interface{}{
			"name":      "ocp-support-agent-deny-ingress",
			"namespace": agentNamespace,
		},
		"spec": map[string]interface{}{
			"podSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "ocp-support-agent",
				},
			},
			"policyTypes": []interface{}{"Ingress"},
			"ingress":     []interface{}{},
		},
	})

	manifestWork := map[string]interface{}{
		"apiVersion": "work.open-cluster-management.io/v1",
		"kind":       "ManifestWork",
		"metadata": map[string]interface{}{
			"name":      manifestWorkName,
			"namespace": clusterName,
			"labels": map[string]interface{}{
				"app.kubernetes.io/managed-by": "ocp-support-web",
			},
		},
		"spec": map[string]interface{}{
			"deleteOption": map[string]interface{}{
				"propagationPolicy": "Foreground",
			},
			"workload": map[string]interface{}{
				"manifests": manifests,
			},
			"manifestConfigs": []interface{}{
				map[string]interface{}{
					"resourceIdentifier": map[string]interface{}{
						"group":     "",
						"resource":  "secrets",
						"name":      agentTokenSecret,
						"namespace": agentNamespace,
					},
					"feedbackRules": []interface{}{
						map[string]interface{}{
							"type": "JSONPaths",
							"jsonPaths": []interface{}{
								map[string]interface{}{
									"name": "token",
									"path": ".data.token",
								},
							},
						},
					},
				},
			},
		},
	}

	if existsErr == nil {
		// Update existing ManifestWork — preserve resourceVersion
		rv := k8s.JsonPath(existing, "metadata", "resourceVersion")
		if rv != "" {
			manifestWork["metadata"].(map[string]interface{})["resourceVersion"] = rv
		}
		if _, err := c.k8s.Put(path, manifestWork); err != nil {
			return fmt.Errorf("update agent ManifestWork: %w", err)
		}
	} else {
		createPath := fmt.Sprintf("/apis/work.open-cluster-management.io/v1/namespaces/%s/manifestworks", clusterName)
		if _, err := c.k8s.Post(createPath, manifestWork); err != nil {
			return fmt.Errorf("create agent ManifestWork: %w", err)
		}
	}

	c.mu.Lock()
	c.agents[clusterName] = true
	c.mu.Unlock()

	log.Printf("Deployed agent to managed cluster %s (image: %s)", clusterName, agentImage)
	return nil
}

// agentStatus checks if the agent ManifestWork exists and is applied.
func (c *Client) agentStatus(clusterName string) (deployed bool, ready bool, version string) {
	path := fmt.Sprintf("/apis/work.open-cluster-management.io/v1/namespaces/%s/manifestworks/%s", clusterName, manifestWorkName)
	data, err := c.k8s.Get(path)
	if err != nil {
		return false, false, ""
	}

	c.mu.Lock()
	c.agents[clusterName] = true
	c.mu.Unlock()

	// Check Applied condition
	st := k8s.JsonMap(data, "status")
	if st == nil {
		return true, false, ""
	}
	conditions, _ := st["conditions"].([]interface{})
	for _, cond := range conditions {
		cm, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		if k8s.StringOrEmpty(cm, "type") == "Applied" && k8s.StringOrEmpty(cm, "status") == "True" {
			return true, true, ""
		}
	}
	return true, false, ""
}

// proxyAgentGetRaw performs a GET to the agent via pod proxy, retrying once on 404
// (which usually means the cached pod name is stale after a pod restart).
func (c *Client) proxyAgentGetRaw(clusterName, agentPath string) ([]byte, error) {
	client, podName, err := c.getAgentAccess(clusterName)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy%s", agentNamespace, podName, agentPath)
	raw, err := client.GetRaw(path)
	if err != nil && k8s.IsNotFound(err) {
		// Pod may have been replaced — invalidate cache and retry
		c.invalidateAgentAccess(clusterName)
		client, podName, err = c.getAgentAccess(clusterName)
		if err != nil {
			return nil, err
		}
		path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy%s", agentNamespace, podName, agentPath)
		return client.GetRaw(path)
	}
	return raw, err
}

// GetClusterAgentVersion reads the agent version from the remote agent's /version endpoint.
func (c *Client) GetClusterAgentVersion(clusterName string) (string, error) {
	raw, err := c.proxyAgentGetRaw(clusterName, "/version")
	if err != nil {
		return "", err
	}

	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	return result.Version, nil
}

// GetClusterNamespacesRaw reads namespaces from the agent as raw JSON bytes.
func (c *Client) GetClusterNamespacesRaw(clusterName string) ([]byte, error) {
	return c.proxyAgentGetRaw(clusterName, "/namespaces")
}

// GetClusterOperatorsRaw reads operators from the agent as raw JSON bytes.
func (c *Client) GetClusterOperatorsRaw(clusterName string) ([]byte, error) {
	return c.proxyAgentGetRaw(clusterName, "/operators")
}

// StartRemoteGather tells the agent on a managed cluster to start gathering.
func (c *Client) StartRemoteGather(clusterName string, gatherTypes []string, since string, anonymize bool, anonOpts mustgather.AnonOptions, namespaces, resourceTypes []string, includeLogs bool) (string, error) {
	client, podName, err := c.getAgentAccess(clusterName)
	if err != nil {
		return "", fmt.Errorf("get agent access: %w", err)
	}

	// Send gather request to agent via pod proxy
	gatherReq := map[string]interface{}{
		"gatherTypes": gatherTypes,
		"since":       since,
		"clusterName": clusterName,
	}
	if len(namespaces) > 0 {
		gatherReq["namespaces"] = namespaces
		gatherReq["resourceTypes"] = resourceTypes
		gatherReq["includeLogs"] = includeLogs
	}
	proxyPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy/gather", agentNamespace, podName)
	if _, err := client.Post(proxyPath, gatherReq); err != nil {
		if k8s.IsNotFound(err) {
			// Pod may have been replaced — invalidate cache and retry
			c.invalidateAgentAccess(clusterName)
			client, podName, err = c.getAgentAccess(clusterName)
			if err != nil {
				return "", fmt.Errorf("get agent access (retry): %w", err)
			}
			proxyPath = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy/gather", agentNamespace, podName)
			if _, err := client.Post(proxyPath, gatherReq); err != nil {
				return "", fmt.Errorf("start gather on agent: %w", err)
			}
		} else {
			return "", fmt.Errorf("start gather on agent: %w", err)
		}
	}

	id := fmt.Sprintf("remote-%s-%d", clusterName, time.Now().UnixMilli())

	// Build display label
	gatherLabel := "Default"
	if len(gatherTypes) > 1 || (len(gatherTypes) == 1 && gatherTypes[0] != "default") {
		gatherLabel = strings.Join(gatherTypes, ", ")
	}

	job := &RemoteGatherJob{
		ID:          id,
		ClusterName: clusterName,
		GatherType:  gatherLabel,
		Status:      "gathering",
		StartedAt:   time.Now(),
		Anonymize:   anonymize,
		AnonOpts:    anonOpts,
		LogOutput:   "Gather started on remote cluster " + clusterName + "...\n",
	}

	c.mu.Lock()
	c.jobs[id] = job
	c.mu.Unlock()

	go c.pollAgentAndRetrieve(clusterName, podName, client, id)

	return id, nil
}

// pollAgentAndRetrieve polls the agent status and downloads the archive when ready.
// Transient connection errors are tolerated — only consecutive failures count toward
// a deadline, so flaky hub-to-agent links (e.g. cross-region) don't kill the gather.
func (c *Client) pollAgentAndRetrieve(clusterName, podName string, client *k8s.Client, jobID string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	const gatherTimeout = 60 * time.Minute
	const maxConsecErrors = 60 // 60 × 5s = 5 minutes of continuous failure before giving up
	deadline := time.Now().Add(gatherTimeout)
	consecErrors := 0
	statusPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy/status", agentNamespace, podName)

	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				c.setJobStatus(jobID, "failed", "Timed out waiting for gather completion")
				return
			}

			raw, err := client.GetRaw(statusPath)
			if err != nil {
				consecErrors++
				if k8s.IsNotFound(err) {
					// Pod replaced — re-resolve and rebuild status path
					c.invalidateAgentAccess(clusterName)
					newClient, newPod, accessErr := c.getAgentAccess(clusterName)
					if accessErr == nil {
						client = newClient
						podName = newPod
						statusPath = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy/status", agentNamespace, newPod)
						consecErrors = 0
						c.appendLog(jobID, "Agent pod changed, reconnected.\n")
					}
				}
				if consecErrors >= maxConsecErrors {
					c.setJobStatus(jobID, "failed", "Lost connection to agent (5 minutes of consecutive errors)")
					return
				}
				if consecErrors == 1 {
					c.appendLog(jobID, "Connection interrupted, retrying...\n")
				}
				log.Printf("Agent poll error for %s (%d consecutive): %v", jobID, consecErrors, err)
				continue
			}

			// Successful poll — reset error counter and extend deadline
			if consecErrors > 0 {
				c.appendLog(jobID, "Connection restored.\n")
			}
			consecErrors = 0
			deadline = time.Now().Add(gatherTimeout)

			var agentStatus struct {
				Status    string `json:"status"`
				Error     string `json:"error"`
				LogOutput string `json:"logOutput"`
				FileName  string `json:"fileName"`
			}
			if err := json.Unmarshal(raw, &agentStatus); err != nil {
				continue
			}

			c.mu.Lock()
			j := c.jobs[jobID]
			if j != nil && agentStatus.LogOutput != "" {
				j.LogOutput = "Gather started on remote cluster " + clusterName + "...\n" + agentStatus.LogOutput
			}
			c.mu.Unlock()

			switch agentStatus.Status {
			case "error":
				c.setJobStatus(jobID, "failed", agentStatus.Error)
				return
			case "serving":
				c.setJobStatus(jobID, "downloading", "")
				c.appendLog(jobID, "Archive ready on remote cluster, downloading...\n")

				if err := c.retrieveArchive(clusterName, podName, client, jobID); err != nil {
					c.setJobStatus(jobID, "failed", fmt.Sprintf("download failed: %v", err))
					return
				}
				return
			}
		}
	}
}

// retrieveArchive downloads the tar.gz from the agent via pod proxy and optionally anonymizes it.
func (c *Client) retrieveArchive(clusterName, podName string, client *k8s.Client, jobID string) error {
	ts := time.Now().Format("20060102-150405")
	fileName := fmt.Sprintf("must-gather-%s-%s.tar.gz", clusterName, ts)
	filePath := filepath.Join(c.workDir, fileName)

	// Stream the archive from the agent with a long timeout
	downloadPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:8080/proxy/download", agentNamespace, podName)
	stream, err := c.longTimeoutStream(client, downloadPath)
	if err != nil {
		return fmt.Errorf("stream archive: %w", err)
	}
	defer stream.Close()

	os.MkdirAll(c.workDir, 0700)
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	n, err := io.Copy(f, stream)
	f.Close()
	if err != nil {
		os.Remove(filePath)
		return fmt.Errorf("download: %w", err)
	}

	c.appendLog(jobID, fmt.Sprintf("Downloaded %.1f MB\n", float64(n)/1024/1024))

	// Check if anonymization is requested
	c.mu.Lock()
	j := c.jobs[jobID]
	anonymize := j != nil && j.Anonymize
	anonOpts := mustgather.AnonOptions{}
	if j != nil {
		anonOpts = j.AnonOpts
	}
	c.mu.Unlock()

	if anonymize && (anonOpts.IPs || anonOpts.MACs || anonOpts.Domains || anonOpts.Services) {
		c.setJobStatus(jobID, "anonymizing", "")
		c.appendLog(jobID, "Applying anonymization...\n")

		if nodeMapping, err := mustgather.BuildNodeMapping(client); err != nil {
			c.appendLog(jobID, fmt.Sprintf("Warning: could not build node mapping: %v\n", err))
		} else if len(nodeMapping) > 0 {
			c.appendLog(jobID, fmt.Sprintf("Node name mapping (%d nodes):\n", len(nodeMapping)))
			names := make([]string, 0, len(nodeMapping))
			for n := range nodeMapping {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				c.appendLog(jobID, fmt.Sprintf("  %s -> %s\n", n, nodeMapping[n]))
			}
		}

		redacted, err := c.anonymizeArchive(filePath, anonOpts, client)
		if err != nil {
			c.appendLog(jobID, fmt.Sprintf("Warning: anonymization failed: %v\n", err))
		} else {
			if redacted > 0 {
				c.appendLog(jobID, fmt.Sprintf("Redacted %d secret value(s).\n", redacted))
			}
			// Rename to indicate anonymized
			anonName := strings.TrimSuffix(fileName, ".tar.gz") + "-anonymized.tar.gz"
			anonPath := filepath.Join(c.workDir, anonName)
			os.Rename(filePath, anonPath)
			filePath = anonPath
			fileName = anonName
			c.appendLog(jobID, "Anonymization complete\n")
		}
	}

	c.mu.Lock()
	if j, ok := c.jobs[jobID]; ok {
		j.Status = "complete"
		j.FilePath = filePath
		j.FileName = fileName
		j.LogOutput += fmt.Sprintf("Complete: %s\n", fileName)
	}
	c.mu.Unlock()

	return nil
}

// anonymizeArchive extracts a tar.gz, runs FastAnonymize, and re-creates the archive.
// managedClient is the k8s client for the managed cluster whose nodes should be mapped.
func (c *Client) anonymizeArchive(archivePath string, opts mustgather.AnonOptions, managedClient *k8s.Client) (int, error) {
	extractDir := archivePath + "-extract"
	if err := extractTarGz(archivePath, extractDir); err != nil {
		return 0, fmt.Errorf("extract: %w", err)
	}
	defer os.RemoveAll(extractDir)

	nodeMapping, _ := mustgather.BuildNodeMapping(managedClient)
	redacted, err := mustgather.FastAnonymize(extractDir, c.workDir, c.clusterDomain, opts, nodeMapping)
	if err != nil {
		return 0, fmt.Errorf("anonymize: %w", err)
	}

	os.Remove(archivePath)
	return redacted, createTarGz(archivePath, extractDir)
}

// longTimeoutStream creates a long-timeout HTTP GET to stream large files from the agent.
// Inherits TLS configuration from the provided k8s client.
func (c *Client) longTimeoutStream(client *k8s.Client, path string) (io.ReadCloser, error) {
	var tlsCfg *tls.Config
	if t, ok := client.HTTPClient.Transport.(*http.Transport); ok && t.TLSClientConfig != nil {
		tlsCfg = t.TLSClientConfig.Clone()
	}
	httpClient := &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	req, err := http.NewRequest("GET", client.APIURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+client.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

// invalidateAgentAccess removes the cached access for a managed cluster,
// forcing the next getAgentAccess call to re-resolve the pod name.
func (c *Client) invalidateAgentAccess(clusterName string) {
	c.mu.Lock()
	delete(c.accessCache, clusterName)
	c.mu.Unlock()
}

// getAgentAccess returns a k8s.Client for the managed cluster and the agent pod name.
// Results are cached for 10 minutes to avoid repeated lookups on every request.
func (c *Client) getAgentAccess(clusterName string) (*k8s.Client, string, error) {
	c.mu.Lock()
	cached := c.accessCache[clusterName]
	c.mu.Unlock()

	if cached != nil && time.Now().Before(cached.expires) {
		return cached.client, cached.podName, nil
	}

	// Get managed cluster API URL
	mcData, err := c.k8s.Get(fmt.Sprintf("/apis/cluster.open-cluster-management.io/v1/managedclusters/%s", clusterName))
	if err != nil {
		return nil, "", fmt.Errorf("get managed cluster: %w", err)
	}

	apiURL := ""
	if configs := k8s.JsonArray(mcData, "spec", "managedClusterClientConfigs"); len(configs) > 0 {
		if cfg, ok := configs[0].(map[string]interface{}); ok {
			apiURL = k8s.StringOrEmpty(cfg, "url")
		}
	}
	if apiURL == "" {
		apiURL = k8s.JsonPath(mcData, "spec", "managedClusterClientConfigs", "url")
	}
	if apiURL == "" {
		return nil, "", fmt.Errorf("no API URL for managed cluster %s", clusterName)
	}

	// Get SA token via ManifestWork statusFeedback
	token, err := c.getAgentToken(clusterName)
	if err != nil {
		return nil, "", fmt.Errorf("get agent token: %w", err)
	}

	client := k8s.NewClient(apiURL, token, true)

	// Find agent pod name
	podName, err := c.findAgentPod(client)
	if err != nil {
		return nil, "", fmt.Errorf("find agent pod: %w", err)
	}

	// Cache for 10 minutes
	c.mu.Lock()
	c.accessCache[clusterName] = &cachedAccess{
		client:  client,
		podName: podName,
		expires: time.Now().Add(10 * time.Minute),
	}
	c.mu.Unlock()

	return client, podName, nil
}

// getAgentToken reads the agent SA token from ManifestWork statusFeedback.
func (c *Client) getAgentToken(clusterName string) (string, error) {
	path := fmt.Sprintf("/apis/work.open-cluster-management.io/v1/namespaces/%s/manifestworks/%s", clusterName, manifestWorkName)

	// Poll for statusFeedback to be populated (may take a few seconds after ManifestWork is applied)
	for i := 0; i < 15; i++ {
		data, err := c.k8s.Get(path)
		if err != nil {
			return "", fmt.Errorf("get ManifestWork: %w", err)
		}

		// Look through resourceStatus.manifests for the Secret's statusFeedback
		manifests := k8s.JsonArray(data, "status", "resourceStatus", "manifests")
		for _, m := range manifests {
			manifest, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			meta := k8s.JsonMap(manifest, "resourceMeta")
			if meta == nil || k8s.StringOrEmpty(meta, "kind") != "Secret" || k8s.StringOrEmpty(meta, "name") != agentTokenSecret {
				continue
			}
			values := k8s.JsonArray(manifest, "statusFeedback", "values")
			for _, v := range values {
				val, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				if k8s.StringOrEmpty(val, "name") != "token" {
					continue
				}
				fv := k8s.JsonMap(val, "fieldValue")
				if fv == nil {
					continue
				}
				tokenB64 := k8s.StringOrEmpty(fv, "string")
				if tokenB64 == "" {
					continue
				}
				tokenBytes, err := base64.StdEncoding.DecodeString(tokenB64)
				if err != nil {
					return tokenB64, nil
				}
				return string(tokenBytes), nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("timed out waiting for agent token from ManifestWork statusFeedback")
}

// findAgentPod finds the running agent pod on the managed cluster.
func (c *Client) findAgentPod(client *k8s.Client) (string, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods?labelSelector=app=ocp-support-agent", agentNamespace)
	data, err := client.Get(path)
	if err != nil {
		return "", fmt.Errorf("list agent pods: %w", err)
	}

	for _, item := range k8s.JsonArray(data, "items") {
		pod, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		phase := k8s.JsonPath(pod, "status", "phase")
		if phase == "Running" {
			return k8s.JsonPath(pod, "metadata", "name"), nil
		}
	}
	return "", fmt.Errorf("no running agent pod found")
}

func (c *Client) setJobStatus(jobID, status, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j, ok := c.jobs[jobID]; ok {
		j.Status = status
		if errMsg != "" {
			j.Error = errMsg
			j.LogOutput += "ERROR: " + errMsg + "\n"
		}
	}
}

func (c *Client) appendLog(jobID, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j, ok := c.jobs[jobID]; ok {
		j.LogOutput += msg
	}
}

// GetJob returns a remote gather job by ID.
func (c *Client) GetJob(id string) *RemoteGatherJob {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j, ok := c.jobs[id]; ok {
		cpy := *j
		return &cpy
	}
	return nil
}

// GetFilePath returns the file path for a completed remote gather job.
func (c *Client) GetFilePath(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if j, ok := c.jobs[id]; ok {
		return j.FilePath
	}
	return ""
}

// ListJobs returns all remote gather jobs.
func (c *Client) ListJobs() []*RemoteGatherJob {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []*RemoteGatherJob
	for _, j := range c.jobs {
		cpy := *j
		result = append(result, &cpy)
	}
	return result
}

// DeleteJob deletes a remote gather job and its local file (agent stays deployed).
func (c *Client) DeleteJob(id string) error {
	c.mu.Lock()
	j, ok := c.jobs[id]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("job not found: %s", id)
	}
	filePath := j.FilePath
	delete(c.jobs, id)
	c.mu.Unlock()

	// Delete local file if it exists
	if filePath != "" {
		os.Remove(filePath)
	}

	return nil
}

// RemoveAgent removes the agent ManifestWork from a managed cluster.
func (c *Client) RemoveAgent(clusterName string) error {
	path := fmt.Sprintf("/apis/work.open-cluster-management.io/v1/namespaces/%s/manifestworks/%s", clusterName, manifestWorkName)
	if err := c.k8s.Delete(path); err != nil && !k8s.IsNotFound(err) {
		return fmt.Errorf("delete agent ManifestWork: %w", err)
	}
	c.mu.Lock()
	delete(c.agents, clusterName)
	delete(c.accessCache, clusterName)
	c.mu.Unlock()
	return nil
}

// RedeployAgent removes and redeploys the agent on a managed cluster (cleanup + upgrade).
func (c *Client) RedeployAgent(clusterName string) error {
	if err := c.RemoveAgent(clusterName); err != nil {
		return err
	}
	// Wait for the ManifestWork deletion to propagate
	path := fmt.Sprintf("/apis/work.open-cluster-management.io/v1/namespaces/%s/manifestworks/%s", clusterName, manifestWorkName)
	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)
		if _, err := c.k8s.Get(path); k8s.IsNotFound(err) {
			break
		}
	}
	return c.DeployAgent(clusterName)
}

// mustGatherImages maps gather type names to their official Red Hat must-gather container images.
var mustGatherImages = map[string]string{
	"default":       "registry.redhat.io/openshift4/ose-must-gather-rhel9:latest",
	"virtualization": "registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:latest",
	"odf":           "registry.redhat.io/odf4/ose-must-gather-rhel9:latest",
	"acm":           "registry.redhat.io/rhacm2/acm-must-gather-rhel9:latest",
	"logging":       "registry.redhat.io/openshift-logging/cluster-logging-rhel9-operator:latest",
	"service-mesh":  "registry.redhat.io/openshift-service-mesh/istio-must-gather-rhel8:latest",
	"serverless":    "registry.redhat.io/openshift-serverless-1/svls-must-gather-rhel8:latest",
	"mce":           "registry.redhat.io/multicluster-engine/must-gather-rhel9:latest",
	"gitops":        "registry.redhat.io/openshift-gitops-1/must-gather-rhel8:latest",
	"compliance":    "registry.redhat.io/compliance/openshift-compliance-must-gather-rhel8:latest",
	"mtc":           "registry.redhat.io/rhmtc/openshift-migration-must-gather-rhel8:latest",
	"netobserv":     "registry.redhat.io/netobserv/network-observability-must-gather-rhel9:latest",
	"local-storage": "registry.redhat.io/openshift4/ose-local-storage-mustgather-rhel9:latest",
	"lvms":          "registry.redhat.io/lvms4/lvms-must-gather-rhel9:latest",
}

// GatherImageInfo returns the available must-gather images for the UI.
type GatherImageInfo struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Image string `json:"image"`
}

// ListGatherImages returns all available must-gather image types.
func ListGatherImages() []GatherImageInfo {
	return []GatherImageInfo{
		{Type: "default", Label: "Default", Image: mustGatherImages["default"]},
		{Type: "virtualization", Label: "Virtualization", Image: mustGatherImages["virtualization"]},
		{Type: "odf", Label: "ODF (Storage)", Image: mustGatherImages["odf"]},
		{Type: "acm", Label: "ACM", Image: mustGatherImages["acm"]},
		{Type: "logging", Label: "Logging", Image: mustGatherImages["logging"]},
		{Type: "service-mesh", Label: "Service Mesh", Image: mustGatherImages["service-mesh"]},
		{Type: "serverless", Label: "Serverless", Image: mustGatherImages["serverless"]},
		{Type: "mce", Label: "MCE", Image: mustGatherImages["mce"]},
		{Type: "gitops", Label: "GitOps", Image: mustGatherImages["gitops"]},
		{Type: "compliance", Label: "Compliance", Image: mustGatherImages["compliance"]},
		{Type: "mtc", Label: "MTC", Image: mustGatherImages["mtc"]},
		{Type: "netobserv", Label: "Network Observability", Image: mustGatherImages["netobserv"]},
		{Type: "local-storage", Label: "Local Storage", Image: mustGatherImages["local-storage"]},
		{Type: "lvms", Label: "LVMS", Image: mustGatherImages["lvms"]},
	}
}

const (
	maxExtractFileSize  = 500 << 20  // 500 MB per file
	maxExtractTotalSize = 10 << 30   // 10 GB total
	maxExtractFiles     = 100_000
)

// extractTarGz extracts a tar.gz archive to a destination directory.
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	var totalBytes int64
	var fileCount int

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0700)
		case tar.TypeReg:
			fileCount++
			if fileCount > maxExtractFiles {
				return fmt.Errorf("archive exceeds maximum file count (%d)", maxExtractFiles)
			}
			os.MkdirAll(filepath.Dir(target), 0700)
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			n, err := io.Copy(outFile, io.LimitReader(tr, maxExtractFileSize+1))
			outFile.Close()
			if err != nil {
				return err
			}
			if n > maxExtractFileSize {
				return fmt.Errorf("file %s exceeds maximum size (%d MB)", header.Name, maxExtractFileSize>>20)
			}
			totalBytes += n
			if totalBytes > maxExtractTotalSize {
				return fmt.Errorf("archive exceeds maximum total extraction size (%d GB)", maxExtractTotalSize>>30)
			}
		}
	}
	return nil
}

// createTarGz creates a gzipped tar archive of the given directory.
func createTarGz(archivePath, sourceDir string) error {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}
