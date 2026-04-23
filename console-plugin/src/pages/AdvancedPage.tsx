import React, { useEffect, useState, useCallback } from 'react';
import {
  Card, CardTitle, CardBody,
  Button,
  Flex,
  Checkbox,
  FormGroup,
  FormSelect, FormSelectOption,
  ToggleGroup, ToggleGroupItem,
  Label,
  Spinner,
} from '@patternfly/react-core';
import SupportPageLayout from '../components/SupportPageLayout';
import JobCard from '../components/JobCard';
import { api, ManagedCluster, GatherJob } from '../api/client';

const RESOURCE_TYPES = [
  { value: 'pods', label: 'Pods', default: true },
  { value: 'services', label: 'Services', default: true },
  { value: 'configmaps', label: 'ConfigMaps', default: true },
  { value: 'events', label: 'Events', default: true },
  { value: 'deployments', label: 'Deployments', default: true },
  { value: 'statefulsets', label: 'StatefulSets' },
  { value: 'daemonsets', label: 'DaemonSets' },
  { value: 'jobs', label: 'Jobs' },
  { value: 'cronjobs', label: 'CronJobs' },
  { value: 'replicasets', label: 'ReplicaSets' },
  { value: 'persistentvolumeclaims', label: 'PVCs' },
  { value: 'serviceaccounts', label: 'Service Accounts' },
  { value: 'roles', label: 'Roles' },
  { value: 'rolebindings', label: 'RoleBindings' },
  { value: 'routes', label: 'Routes' },
  { value: 'ingresses', label: 'Ingresses' },
  { value: 'endpointslices', label: 'EndpointSlices' },
  { value: 'secrets', label: 'Secrets (metadata)' },
  { value: 'networkpolicies', label: 'NetworkPolicies' },
  { value: 'horizontalpodautoscalers', label: 'HPAs' },
];

const LOCAL_CLUSTER = '__local__';

const AdvancedPage: React.FC = () => {
  const [acmClusters, setAcmClusters] = useState<ManagedCluster[]>([]);
  const [selectedCluster, setSelectedCluster] = useState(LOCAL_CLUSTER);
  const [allNamespaces, setAllNamespaces] = useState<string[]>([]);
  const [nsLoading, setNsLoading] = useState(false);
  const [selectedNS, setSelectedNS] = useState<string[]>([]);
  const [selectedRT, setSelectedRT] = useState<Set<string>>(new Set(RESOURCE_TYPES.filter((r) => r.default).map((r) => r.value)));
  const [includeLogs, setIncludeLogs] = useState(true);
  const [anonymize, setAnonymize] = useState(false);
  const [anonIPs, setAnonIPs] = useState(true);
  const [anonMACs, setAnonMACs] = useState(true);
  const [anonDomains, setAnonDomains] = useState(true);
  const [anonServices, setAnonServices] = useState(true);
  const [anonSecrets, setAnonSecrets] = useState(false);
  const [sinceEnabled, setSinceEnabled] = useState(false);
  const [since, setSince] = useState('24h');
  const [localJobs, setLocalJobs] = useState<GatherJob[]>([]);
  const [remoteJobs, setRemoteJobs] = useState<GatherJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploadEnabled, setUploadEnabled] = useState(false);

  const isRemote = selectedCluster !== LOCAL_CLUSTER;
  const jobs = isRemote ? remoteJobs : localJobs;

  useEffect(() => {
    Promise.all([
      api.getNamespaces().then(setAllNamespaces),
      api.listGatherJobs().then(setLocalJobs),
      api.getUploadConfig().then((d) => setUploadEnabled(d.configured)).catch(() => null),
      api.getCapabilities().then((caps) => {
        if (caps.acm) {
          api.getACMClusters().then((clusters) => {
            setAcmClusters(Array.isArray(clusters) ? clusters.filter((c) => c.status === 'Available' && c.agentReady) : []);
          }).catch(() => null);
        }
      }).catch(() => null),
      api.listRemoteGatherJobs().then((d) => setRemoteJobs(Array.isArray(d) ? d : [])).catch(() => null),
    ])
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const pollJobs = useCallback(() => {
    api.listGatherJobs().then(setLocalJobs).catch(() => {});
    api.listRemoteGatherJobs().then((d) => setRemoteJobs(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  useEffect(() => {
    const interval = setInterval(pollJobs, 5000);
    return () => clearInterval(interval);
  }, [pollJobs]);

  const loadNamespacesForCluster = async (cluster: string) => {
    setSelectedNS([]);
    setNsLoading(true);
    try {
      if (cluster === LOCAL_CLUSTER) {
        const ns = await api.getNamespaces();
        setAllNamespaces(ns);
      } else {
        const ns = await api.getClusterNamespaces(cluster);
        setAllNamespaces(Array.isArray(ns) ? ns : []);
      }
    } catch {
      setAllNamespaces([]);
    } finally {
      setNsLoading(false);
    }
  };

  const onClusterChange = (_e: unknown, cluster: string) => {
    setSelectedCluster(cluster);
    loadNamespacesForCluster(cluster);
  };

  const addNamespace = (ns: string) => {
    if (ns && !selectedNS.includes(ns)) {
      setSelectedNS((prev) => [...prev, ns]);
    }
  };

  const removeNamespace = (ns: string) => {
    setSelectedNS((prev) => prev.filter((n) => n !== ns));
  };

  const toggleRT = (value: string) => {
    setSelectedRT((prev) => {
      const next = new Set(prev);
      next.has(value) ? next.delete(value) : next.add(value);
      return next;
    });
  };

  const startGather = async () => {
    const body: Record<string, unknown> = {
      namespaces: selectedNS,
      resourceTypes: Array.from(selectedRT),
      includeLogs,
    };
    if (anonymize) {
      body.anonymize = true;
      body.anonOpts = { ips: anonIPs, macs: anonMACs, domains: anonDomains, services: anonServices, secrets: anonSecrets };
    }
    if (sinceEnabled) body.since = since;

    try {
      if (isRemote) {
        body.clusterName = selectedCluster;
        body.gatherTypes = ['custom'];
        await api.startRemoteGather(body);
      } else {
        body.types = ['custom'];
        await api.startGather(body);
      }
      pollJobs();
    } catch (e) {
      console.error('Failed to start custom gather', e);
    }
  };

  if (loading) {
    return (
      <SupportPageLayout title="Custom Namespace Gather">
        <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
      </SupportPageLayout>
    );
  }

  return (
    <SupportPageLayout title="Custom Namespace Gather" description="Collect diagnostic data from specific namespaces. Select which namespaces and resource types to include.">
      {acmClusters.length > 0 && (
        <Card isCompact className="pf-v5-u-mb-lg">
          <CardTitle>Target Cluster</CardTitle>
          <CardBody>
            <FormSelect
              value={selectedCluster}
              onChange={onClusterChange}
              style={{ maxWidth: 400 }}
            >
              <FormSelectOption value={LOCAL_CLUSTER} label="Local Cluster (hub)" />
              {acmClusters.map((c) => (
                <FormSelectOption key={c.name} value={c.name} label={`${c.name}${c.ocpVersion ? ` (OCP ${c.ocpVersion})` : ''}`} />
              ))}
            </FormSelect>
            {isRemote && (
              <p className="pf-v5-u-font-size-sm pf-v5-u-color-200 pf-v5-u-mt-sm">
                Gathering from managed cluster: <strong>{selectedCluster}</strong>
              </p>
            )}
          </CardBody>
        </Card>
      )}

      <Card isCompact className="pf-v5-u-mb-lg">
        <CardTitle>Select Namespaces</CardTitle>
        <CardBody>
          {nsLoading ? (
            <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="md" /></Flex>
          ) : (
            <>
              <FormSelect
                value=""
                onChange={(_e, v) => addNamespace(v)}
                style={{ maxWidth: 400 }}
              >
                <FormSelectOption value="" label="-- Select a namespace --" isPlaceholder />
                {allNamespaces.filter((n) => !selectedNS.includes(n)).map((n) => (
                  <FormSelectOption key={n} value={n} label={n} />
                ))}
              </FormSelect>
              <Flex gap={{ default: 'gapSm' }} flexWrap={{ default: 'wrap' }} className="pf-v5-u-mt-sm">
                {selectedNS.map((ns) => (
                  <Label key={ns} onClose={() => removeNamespace(ns)}>{ns}</Label>
                ))}
              </Flex>
              {selectedNS.length > 0 && (
                <p className="pf-v5-u-font-size-sm pf-v5-u-color-200 pf-v5-u-mt-sm">
                  {selectedNS.length} namespace{selectedNS.length !== 1 ? 's' : ''} selected
                </p>
              )}
            </>
          )}
        </CardBody>
      </Card>

      <Card isCompact className="pf-v5-u-mb-lg">
        <CardTitle>Resource Types</CardTitle>
        <CardBody>
          <Flex gap={{ default: 'gapMd' }} flexWrap={{ default: 'wrap' }}>
            {RESOURCE_TYPES.map((rt) => (
              <Checkbox
                key={rt.value}
                id={`rt-${rt.value}`}
                label={rt.label}
                isChecked={selectedRT.has(rt.value)}
                onChange={() => toggleRT(rt.value)}
              />
            ))}
          </Flex>
          <Flex gap={{ default: 'gapSm' }} className="pf-v5-u-mt-sm">
            <Button variant="link" size="sm" onClick={() => setSelectedRT(new Set(RESOURCE_TYPES.map((r) => r.value)))}>Select all</Button>
            <Button variant="link" size="sm" onClick={() => setSelectedRT(new Set())}>Deselect all</Button>
          </Flex>
        </CardBody>
      </Card>

      <Card isCompact className="pf-v5-u-mb-lg">
        <CardTitle>Options</CardTitle>
        <CardBody>
          <Flex alignItems={{ default: 'alignItemsCenter' }} gap={{ default: 'gapLg' }} flexWrap={{ default: 'wrap' }}>
            <FormGroup label="Pod Logs" isInline>
              <ToggleGroup>
                <ToggleGroupItem text="Off" buttonId="logs-off" isSelected={!includeLogs} onChange={() => setIncludeLogs(false)} />
                <ToggleGroupItem text="On" buttonId="logs-on" isSelected={includeLogs} onChange={() => setIncludeLogs(true)} />
              </ToggleGroup>
            </FormGroup>
            <FormGroup label="Anonymize" isInline>
              <ToggleGroup>
                <ToggleGroupItem text="Off" buttonId="anon-off" isSelected={!anonymize} onChange={() => setAnonymize(false)} />
                <ToggleGroupItem text="On" buttonId="anon-on" isSelected={anonymize} onChange={() => setAnonymize(true)} />
              </ToggleGroup>
              {anonymize && (
                <Flex direction={{ default: 'column' }} gap={{ default: 'gapXs' }} className="pf-v5-u-mt-sm">
                  <Checkbox id="adv-anon-ips" label="IP addresses" isChecked={anonIPs} onChange={(_e, v) => setAnonIPs(v)} />
                  <Checkbox id="adv-anon-macs" label="MAC addresses" isChecked={anonMACs} onChange={(_e, v) => setAnonMACs(v)} />
                  <Checkbox id="adv-anon-domains" label="Domain names" isChecked={anonDomains} onChange={(_e, v) => setAnonDomains(v)} />
                  <Checkbox id="adv-anon-services" label="Service names" isChecked={anonServices} onChange={(_e, v) => setAnonServices(v)} />
                  <Checkbox id="adv-anon-secrets" label="Secrets" isChecked={anonSecrets} onChange={(_e, v) => setAnonSecrets(v)} />
                </Flex>
              )}
            </FormGroup>
            <FormGroup label="Time frame" isInline>
              <ToggleGroup>
                <ToggleGroupItem text="Off" buttonId="since-off" isSelected={!sinceEnabled} onChange={() => setSinceEnabled(false)} />
                <ToggleGroupItem text="On" buttonId="since-on" isSelected={sinceEnabled} onChange={() => setSinceEnabled(true)} />
              </ToggleGroup>
              {sinceEnabled && (
                <FormSelect value={since} onChange={(_e, v) => setSince(v)} className="pf-v5-u-ml-sm" style={{ width: 'auto' }}>
                  <FormSelectOption value="6h" label="Last 6 hours" />
                  <FormSelectOption value="12h" label="Last 12 hours" />
                  <FormSelectOption value="24h" label="Last 24 hours" />
                  <FormSelectOption value="48h" label="Last 48 hours" />
                </FormSelect>
              )}
            </FormGroup>
          </Flex>
          <Flex justifyContent={{ default: 'justifyContentFlexEnd' }} className="pf-v5-u-mt-md">
            <Button variant="primary" isDisabled={selectedNS.length === 0} onClick={startGather}>
              {isRemote ? `Start Custom Gather on ${selectedCluster}` : 'Start Custom Gather'}
            </Button>
          </Flex>
        </CardBody>
      </Card>

      <Card isCompact>
        <CardTitle>Jobs</CardTitle>
        <CardBody>
          {jobs.length === 0 ? (
            <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No jobs yet.</p>
          ) : (
            jobs.map((job) => (
              <JobCard
                key={job.id}
                job={job}
                uploadEnabled={uploadEnabled}
                downloadURL={isRemote ? api.getRemoteGatherDownloadURL(job.id) : api.getGatherDownloadURL(job.id)}
                onDelete={async () => {
                  if (isRemote) {
                    await api.deleteRemoteGather(job.id);
                    setRemoteJobs((prev) => prev.filter((j) => j.id !== job.id));
                  } else {
                    await api.deleteGather(job.id);
                    setLocalJobs((prev) => prev.filter((j) => j.id !== job.id));
                  }
                }}
              />
            ))
          )}
        </CardBody>
      </Card>
    </SupportPageLayout>
  );
};

export default AdvancedPage;
