# OCP Support Web

A web application that provides common OpenShift support tools through a browser interface. Designed to run behind an OpenShift OAuth proxy, giving cluster-admin users one-click access to cluster diagnostics, etcd backup, etcd diagnostics, cluster status, and ACM multi-cluster support.

**This is a community project. It is not supported by Red Hat.**

## Features

### Native Cluster Diagnostics
Collect cluster diagnostics using native Go API calls — no external must-gather images required. Supports 20 operator-specific gather profiles:

- **Default** — general cluster diagnostics, pod logs, etcd status, OVN, and node diagnostics
- **Virtualization**, **ODF**, **ACM**, **Logging**, **Service Mesh**, **Compliance**, **MTC**, **GitOps**, **Serverless**, **MCE**, **Network Observability**, **Local Storage**, **Sandboxed Containers**, **Node Health Check**, **NUMA Resources**, **PTP**, **Secrets Store CSI**, **LVMS**
- **Audit Logs** — API server audit logging records
- **Gather All** — all installed types in one run

Only installed operators are shown. Optional data anonymization (IPs, MACs, domain names) before sharing with support. Archives download directly through the browser.

### Standalone Must-Gather Image
The container image also works as a standalone must-gather image:
```bash
oc adm must-gather --image=quay.io/redhat-consulting-services/ocp-support-web:v3.0.0
```
Auto-detects installed operators and collects diagnostics for all of them.

### ACM Multi-Cluster Support
Deploy agents to managed clusters via the web UI and gather diagnostics from remote clusters through ACM.

### Etcd Backup
Create etcd snapshots from a master node with step-by-step progress tracking. The backup archive (etcd snapshot + static pod resources) is available for browser download.

### Etcd Diagnostics
Analyze etcd database contents without downloading anything:
- **Object Type Counts** — how many objects of each type are stored
- **Object Sizes** — total size per resource type
- **Namespace Size Breakdown** — which namespaces use the most etcd space (secrets, configmaps, events)
- **Creation Timeline** — when objects of a specific type were created (by month, day, hour)
- **Count per Namespace** — namespace distribution for a specific resource type

Results can be copied to clipboard or saved as text files.

### Cluster Status
At-a-glance cluster health dashboard:
- Cluster version and update status
- Control plane component health
- Cluster operator status (degraded/unavailable detection)
- ODF storage health (Ceph status)
- Etcd member health and leader info
- Node utilization (CPU, memory, pods per node)
- Top consumers (pods and VMs by CPU/memory — VMs only shown if CNV is installed)
- Storage classes with default marking, provisioner, and reclaim policy
- NMState network interfaces with per-node active/missing status (only shown if NMState is installed)

## ConfigMap-Driven Gather Configuration

Gather definitions can be customized via Kubernetes ConfigMaps. When deployed via the operator, a `gather-common` ConfigMap is created automatically. You can edit it to add custom resources, commands, and log specifications.

ConfigMap entries are **merged with the built-in defaults** — you only need to specify what you want to add, not the full set.

### ConfigMap Format

Each gather type maps to a ConfigMap named `gather-{type}` (e.g., `gather-virtualization`, `gather-odf`). The default/common gather uses `gather-common`.

The ConfigMap must contain a `data.spec` key with YAML content:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gather-common
  namespace: ocp-support-web
data:
  spec: |
    displayName: "Custom cluster diagnostics"

    clusterResources:
      - group: "example.io"
        version: "v1"
        resource: "myresources"

    namespacedResources:
      - group: ""
        version: "v1"
        resource: "configmaps"
        namespaces: ["my-namespace", "other-ns-*"]
        labelSelector: "app=myapp"

    podLogs:
      - namespaces: ["my-namespace"]
        labelSelector: "app=myapp"
        maxLines: 5000
      - namespaces: ["my-namespace"]
        previous: true
        maxLines: 2000
```

### Spec Fields

| Field | Description |
|-------|-------------|
| `displayName` | Human-readable name (overrides built-in if set) |
| `clusterResources` | Cluster-scoped Kubernetes resources to collect |
| `namespacedResources` | Namespaced resources with glob pattern support for namespace selection (e.g., `openshift-*`) |
| `podLogs` | Pod log collection specs with namespace patterns, label selectors, and line limits |

### Merge Behavior

ConfigMap entries are appended to the built-in definition for that gather type:
- If `gather-common` exists, its resources are added to the built-in default definition
- If `gather-virtualization` exists, its resources are added to the built-in virtualization definition
- If no ConfigMap exists, the built-in definition is used as-is
- Changes take effect on the next gather request — no application restart needed

### Creating Custom Gather Types

You can also create entirely new gather types by creating a ConfigMap with no built-in counterpart. For example, `gather-myapp` would define a custom gather profile that only exists as a ConfigMap.

## Architecture

```
main.go                         Entry point, wires components (web server, agent, or must-gather mode)
internal/config/                Configuration from environment variables
internal/handler/               HTTP handlers and route registration
internal/collector/             Native gather engine (config.go, engine.go, resources.go, exec.go, logs.go, nodeops.go)
internal/gather/                Standalone must-gather mode (oc adm must-gather --image=...)
internal/acm/                   ACM multi-cluster management and agent deployment
internal/agent/                 Remote agent for managed clusters
internal/k8s/                   Extended Kubernetes client for native API calls
internal/status/                Cluster health queries via OpenShift API
internal/monitoring/            Thanos/Prometheus queries for etcd health
internal/metrics/               Prometheus metrics and HTTP middleware
web/templates/                  HTML templates (support.html, status.html, acm.html)
web/static/                     JavaScript, CSS, SVG assets
```

The application is a single Go binary using stdlib `net/http`. Frontend assets are embedded via `go:embed`. No build step is required for the frontend — it uses plain HTML/JS with PatternFly 5 CSS from CDN.

The container image serves three modes:
- **Web server** (default) — the full web UI with all features
- **Agent** (`AGENT_MODE=true`) — lightweight agent for ACM managed clusters
- **Must-gather** (`oc adm must-gather --image=...`) — standalone gather with auto operator detection

## Deployment

The recommended deployment method is via the [OCP Support Web Operator](https://github.com/redhat-consulting-services/ocp-support-web-operator), which handles the full lifecycle including OAuth proxy, RBAC, routes, ConfigMaps, and metrics.

### Container Image

```bash
podman build -t quay.io/youruser/ocp-support-web:latest -f Containerfile .
podman push quay.io/youruser/ocp-support-web:latest
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LISTEN_ADDR` | HTTP listen address | `:8080` |
| `MUST_GATHER_DIR` | Working directory for gather archives | `/tmp/ocp-support-web/gather` |
| `NATIVE_GATHER` | Use native Go collector instead of oc CLI | `true` |
| `OPENSHIFT_API_URL` | OpenShift API URL | Auto-detected in-cluster |
| `OPENSHIFT_TOKEN` | Bearer token for API access | SA token from mounted secret |
| `CLUSTER_DOMAIN` | Cluster apps domain (for Thanos queries) | Not set |
| `INSECURE_SKIP_TLS` | Skip TLS verification for API calls | `true` |
| `AGENT_IMAGE` | Container image for ACM remote agents | Auto-detected from pod spec |
| `AGENT_MODE` | Run in agent mode for managed clusters | `false` |
| `MUST_GATHER` | Run in standalone must-gather mode | `false` |

When running in-cluster, `OPENSHIFT_API_URL` and `OPENSHIFT_TOKEN` are auto-detected from the pod's service account.

## Metrics

The application exposes Prometheus metrics on port 8081 (separate from the main server to avoid the OAuth proxy):

- `ocp_support_web_http_requests_total` — HTTP request count by method, path, and status
- `ocp_support_web_http_request_duration_seconds` — HTTP request latency histogram
- `ocp_support_web_mustgather_jobs_active` — currently running gather jobs
- `ocp_support_web_mustgather_jobs_total` — total gather jobs by type
- `ocp_support_web_etcd_diag_jobs_total` — total etcd diagnostic jobs

## Disconnected / Air-Gapped Environments

The application uses a single container image for all modes (web UI, ACM agents, standalone must-gather). No external image pulls are required at runtime — all gather operations use native Go API calls instead of launching external must-gather images.

### Single Registry (All Clusters Share One Registry)

If the hub and all managed clusters can pull from the same internal registry:

1. Mirror the application image to your internal registry
2. Set `spec.image` in the operator CR to the mirrored location:
   ```yaml
   spec:
     image: registry.internal.example.com/ocp-support-web:v3.0.0
   ```
3. The operator passes this image to the application as `AGENT_IMAGE`, so ACM agents deployed to managed clusters will also use the mirrored image

### Multiple Registries (Each Cluster Has Its Own Registry)

When managed clusters are separately disconnected and each connects to a different registry, the agent image reference must be set per cluster. This is done by annotating each `ManagedCluster` resource on the hub.

#### Step 1: Mirror the Image to Each Cluster's Registry

Mirror the application image to every disconnected cluster's local registry:

```bash
# For each cluster, mirror to its registry
skopeo copy \
  docker://quay.io/redhat-consulting-services/ocp-support-web:v3.0.0 \
  docker://registry.cluster-a.example.com/ocp-support-web:v3.0.0

skopeo copy \
  docker://quay.io/redhat-consulting-services/ocp-support-web:v3.0.0 \
  docker://registry.cluster-b.example.com/ocp-support-web:v3.0.0
```

#### Step 2: Annotate Each ManagedCluster

On the hub cluster, annotate each `ManagedCluster` resource with its local registry path:

```bash
oc annotate managedcluster cluster-a \
  ocp-support-web/agent-image=registry.cluster-a.example.com/ocp-support-web:v3.0.0

oc annotate managedcluster cluster-b \
  ocp-support-web/agent-image=registry.cluster-b.example.com/ocp-support-web:v3.0.0
```

#### Step 3: Deploy or Redeploy Agents

If agents are already deployed, redeploy them from the ACM page in the web UI so they pick up the new image reference. New agents will automatically use the annotated image.

#### How It Works

When deploying an agent, the application checks the `ManagedCluster` resource for the `ocp-support-web/agent-image` annotation. If present, that image is used in the agent's Deployment spec. If absent, it falls back to the global `AGENT_IMAGE` or auto-detected image.

| Cluster | Annotation | Image Used |
|---------|-----------|------------|
| cluster-a | `ocp-support-web/agent-image=registry.cluster-a.example.com/...` | Annotation value |
| cluster-b | `ocp-support-web/agent-image=registry.cluster-b.example.com/...` | Annotation value |
| cluster-c | *(none)* | Global `AGENT_IMAGE` or auto-detected |

#### Updating the Image Version

When upgrading to a new version, update the annotation on each managed cluster and redeploy:

```bash
# Update annotations
oc annotate managedcluster cluster-a \
  ocp-support-web/agent-image=registry.cluster-a.example.com/ocp-support-web:v3.1.0 --overwrite

oc annotate managedcluster cluster-b \
  ocp-support-web/agent-image=registry.cluster-b.example.com/ocp-support-web:v3.1.0 --overwrite
```

Then redeploy agents from the web UI or restart the hub application to trigger reconciliation.

### Standalone Must-Gather

For standalone must-gather usage in disconnected environments:
```bash
oc adm must-gather --image=registry.internal.example.com/ocp-support-web:v3.0.0
```

## Authentication

When deployed via the operator, the application sits behind an OpenShift OAuth proxy. Access is restricted to configured OpenShift groups (defaults to `cluster-admins`). The authenticated username is displayed in the masthead with a logout option.
