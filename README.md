# OCP Support Web

A web application that provides common OpenShift support tools through a browser interface. Designed to run behind an OpenShift OAuth proxy, giving cluster-admin users one-click access to must-gather, etcd backup, etcd diagnostics, and cluster status.

**This is a community project. It is not supported by Red Hat.**

## Features

### Must-Gather
Run must-gather collections from the browser with type selection:
- **Default** — general cluster diagnostics and logs
- **Virtualization** — OpenShift Virtualization / KubeVirt (only shown if CNV is installed)
- **ODF** — OpenShift Data Foundation / Ceph (only shown if ODF is installed)
- **Audit Logs** — API server audit logging records
- **Gather All** — all installed types in one run

Optional data anonymization (IP obfuscation) before sharing with support. Archives download directly through the browser.

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

## Architecture

```
main.go                         Entry point, wires components
internal/config/                Configuration from environment variables
internal/handler/               HTTP handlers and route registration
internal/mustgather/            Must-gather job manager (runs oc adm must-gather)
internal/status/                Cluster health queries via OpenShift API
internal/monitoring/            Thanos/Prometheus queries for etcd health
internal/metrics/               Prometheus metrics and HTTP middleware
web/templates/                  HTML templates (support.html, status.html)
web/static/                     JavaScript, CSS, SVG assets
```

The application is a single Go binary using stdlib `net/http`. Frontend assets are embedded via `go:embed`. No build step is required for the frontend — it uses plain HTML/JS with PatternFly 5 CSS from CDN.

## Deployment

The recommended deployment method is via the [OCP Support Web Operator](https://github.com/redhat-consulting-services/ocp-support-web-operator), which handles the full lifecycle including OAuth proxy, RBAC, routes, and metrics.

### Container Image

```bash
podman build -t quay.io/youruser/ocp-support-web:v0.1.0 -f Containerfile .
podman push quay.io/youruser/ocp-support-web:v0.1.0
```

The runtime image is based on `ose-cli-rhel9` which provides the `oc` binary needed for must-gather and etcd operations.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LISTEN_ADDR` | HTTP listen address | `:8080` |
| `MUST_GATHER_DIR` | Working directory for must-gather archives | `/tmp/ocp-support-web/gather` |
| `OPENSHIFT_API_URL` | OpenShift API URL | Auto-detected in-cluster |
| `OPENSHIFT_TOKEN` | Bearer token for API access | SA token from mounted secret |
| `CLUSTER_DOMAIN` | Cluster apps domain (for Thanos queries) | Not set |
| `INSECURE_SKIP_TLS` | Skip TLS verification for API calls | `true` |
| `MUST_GATHER_IMAGE_DEFAULT` | Default must-gather image | Cluster release payload |
| `MUST_GATHER_IMAGE_CNV` | CNV must-gather image | `registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0` |
| `MUST_GATHER_IMAGE_ODF` | ODF must-gather image | `registry.redhat.io/odf4/ocs-must-gather-rhel9:latest` |

When running in-cluster, `OPENSHIFT_API_URL` and `OPENSHIFT_TOKEN` are auto-detected from the pod's service account.

## Metrics

The application exposes Prometheus metrics on port 8081 (separate from the main server to avoid the OAuth proxy):

- `ocp_support_web_http_requests_total` — HTTP request count by method, path, and status
- `ocp_support_web_http_request_duration_seconds` — HTTP request latency histogram
- `ocp_support_web_mustgather_jobs_active` — currently running must-gather jobs
- `ocp_support_web_mustgather_jobs_total` — total must-gather jobs by type
- `ocp_support_web_etcd_diag_jobs_total` — total etcd diagnostic jobs

## Authentication

When deployed via the operator, the application sits behind an OpenShift OAuth proxy. Only users with `create` permission on `namespaces` (cluster-admin privilege) can access the UI. The authenticated username is displayed in the masthead.

---

*Assisted by: Claude*
