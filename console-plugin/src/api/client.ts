import { consoleFetch } from '@openshift-console/dynamic-plugin-sdk';

const BACKEND_PROXY = '/api/proxy/plugin/ocp-support-web/backend';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${BACKEND_PROXY}${path}`;
  const response = await consoleFetch(url, init);
  if (!response.ok) {
    const text = await response.text().catch(() => '');
    throw new Error(`${response.status}: ${text || response.statusText}`);
  }
  return response.json() as Promise<T>;
}

async function requestText(path: string, init?: RequestInit): Promise<string> {
  const url = `${BACKEND_PROXY}${path}`;
  const response = await consoleFetch(url, init);
  if (!response.ok) {
    const text = await response.text().catch(() => '');
    throw new Error(`${response.status}: ${text || response.statusText}`);
  }
  return response.text();
}

export interface Capabilities {
  cnv: boolean;
  odf: boolean;
  acm: boolean;
  s3Storage: boolean;
  managedClusterCount?: number;
  [key: string]: unknown;
}

export interface GatherJob {
  id: string;
  type: string;
  status: string;
  step: number;
  totalSteps: number;
  stepLabel: string;
  logOutput?: string;
  fileName?: string;
  error?: string;
  startedAt?: string;
  anonymize?: boolean;
}

export interface TopPod {
  name: string;
  namespace: string;
  cpuUsage: number;
  memUsage: number;
  cpuStr: string;
  memStr: string;
}

export interface EtcdHealth {
  healthy: boolean;
  members: Array<{
    name: string;
    pod: string;
    isLeader: boolean;
    revision: number;
    dbSizeMB: number;
  }>;
}

export interface ClusterHealth {
  version: string;
  status: string;
  etcdEncryption?: string;
  controlPlane: Array<{ name: string; available: boolean; degraded: boolean; message?: string }>;
  operators: Array<{ name: string; available: boolean; degraded: boolean; message?: string }>;
  odf?: { installed: boolean; name?: string; phase?: string };
}

export interface NodeInfo {
  name: string;
  status: string;
  roles: string[];
  cpuAllocatable: number;
  cpuUsage: number;
  cpuRequests: number;
  cpuOvercommitPct: number;
  memAllocatable: number;
  memUsage: number;
  memRequests: number;
  memOvercommitPct: number;
  pods: number;
}

export interface ArchiveEntry {
  key: string;
  clusterName: string;
  gatherType: string;
  description: string;
  timestamp: string;
  sizeBytes: number;
  fileName: string;
  source: string;
}

export interface ManagedCluster {
  name: string;
  status: string;
  ocpVersion?: string;
  platform?: string;
  vendor?: string;
  agentDeployed: boolean;
  agentReady: boolean;
}

export const api = {
  getCapabilities: () => request<Capabilities>('/api/support/capabilities'),
  getClusterID: () => request<{ id: string }>('/api/support/cluster-id'),
  getNamespaces: () => request<string[]>('/api/support/namespaces'),
  getNodes: () => request<Array<{ name: string }>>('/api/support/nodes'),

  startGather: (body: Record<string, unknown>) =>
    request<{ jobId: string }>('/api/support/gather', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),
  getGatherStatus: (jobId: string) => request<GatherJob>(`/api/support/gather/${encodeURIComponent(jobId)}`),
  stopGather: (jobId: string) => request<void>(`/api/support/gather/${encodeURIComponent(jobId)}/stop`, { method: 'POST' }),
  deleteGather: (jobId: string) => request<void>(`/api/support/gather/${encodeURIComponent(jobId)}`, { method: 'DELETE' }),
  listGatherJobs: () => request<GatherJob[]>('/api/support/jobs'),
  getGatherDownloadURL: (jobId: string) => `${BACKEND_PROXY}/api/support/gather/${encodeURIComponent(jobId)}/download`,

  startDiag: (body: Record<string, unknown>) =>
    request<{ jobId: string }>('/api/support/etcd-diag', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),
  getDiagStatus: (jobId: string) => request<GatherJob>(`/api/support/etcd-diag/${encodeURIComponent(jobId)}`),

  getClusterHealth: () => request<ClusterHealth>('/api/status/cluster'),
  getNodeUtilization: () => request<NodeInfo[]>('/api/status/nodes'),
  getTopConsumers: () => request<{ pods: TopPod[] }>('/api/status/top'),
  getStorageClasses: () => request<unknown[]>('/api/status/storageclasses'),
  getNetworks: () => request<unknown[]>('/api/status/networks'),
  getEtcdHealth: () => request<EtcdHealth>('/api/status/etcd'),
  getGPUNodes: () => request<unknown[]>('/api/status/gpus'),

  getAPIResources: () => request<unknown[]>('/api/resources/apiresources'),
  getNamespaceResources: (ns: string) => request<unknown[]>(`/api/resources/ns?ns=${encodeURIComponent(ns)}`),
  getResource: (ns: string, group: string, version: string, resource: string, name: string) =>
    requestText(`/api/resources/get?ns=${encodeURIComponent(ns)}&group=${encodeURIComponent(group)}&version=${encodeURIComponent(version)}&resource=${encodeURIComponent(resource)}&name=${encodeURIComponent(name)}`),

  listArchives: (cluster?: string) => {
    const params = cluster ? `?cluster=${encodeURIComponent(cluster)}` : '';
    return request<ArchiveEntry[]>(`/api/archive/list${params}`);
  },
  getArchiveDownloadURL: (key: string) => `${BACKEND_PROXY}/api/archive/download?key=${encodeURIComponent(key)}`,
  deleteArchive: (key: string) => request<void>(`/api/archive/${encodeURIComponent(key)}`, { method: 'DELETE' }),

  getUploadConfig: () => request<{ configured: boolean }>('/api/support/upload/config'),
  uploadToRedHat: (jobId: string, body: { caseID: string; internalUser: boolean }) =>
    request<{ status: string }>(`/api/support/upload/${encodeURIComponent(jobId)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),

  getACMClusters: () => request<ManagedCluster[]>('/api/acm/clusters'),
  startRemoteGather: (body: Record<string, unknown>) =>
    request<{ jobId: string }>('/api/acm/gather', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),
  listRemoteGatherJobs: () => request<GatherJob[]>('/api/acm/gather/jobs'),
  getRemoteGatherStatus: (jobId: string) => request<GatherJob>(`/api/acm/gather/${encodeURIComponent(jobId)}`),
  getRemoteGatherDownloadURL: (jobId: string) => `${BACKEND_PROXY}/api/acm/gather/${encodeURIComponent(jobId)}/download`,
  deleteRemoteGather: (jobId: string) => request<void>(`/api/acm/gather/${encodeURIComponent(jobId)}`, { method: 'DELETE' }),
  deployAgent: (cluster: string) => request<void>(`/api/acm/clusters/${encodeURIComponent(cluster)}/agent`, { method: 'POST' }),
  removeAgent: (cluster: string) => request<void>(`/api/acm/clusters/${encodeURIComponent(cluster)}/agent`, { method: 'DELETE' }),
  redeployAgent: (cluster: string) => request<void>(`/api/acm/clusters/${encodeURIComponent(cluster)}/agent/redeploy`, { method: 'POST' }),
  getClusterNamespaces: (cluster: string) => request<string[]>(`/api/acm/clusters/${encodeURIComponent(cluster)}/namespaces`),
  getClusterOperators: (cluster: string) => request<unknown[]>(`/api/acm/clusters/${encodeURIComponent(cluster)}/operators`),
  getAgentVersion: (cluster: string) => request<{ version: string }>(`/api/acm/clusters/${encodeURIComponent(cluster)}/version`),
};
