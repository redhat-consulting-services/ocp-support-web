import React, { useEffect, useState, useCallback } from 'react';
import {
  Card, CardTitle, CardBody,
  Button,
  Flex, FlexItem,
  ToggleGroup, ToggleGroupItem,
  Checkbox,
  FormGroup,
  FormSelect, FormSelectOption,
  Divider,
  ClipboardCopy,
  Spinner,
} from '@patternfly/react-core';
import SupportPageLayout from '../components/SupportPageLayout';
import JobCard from '../components/JobCard';
import { api, Capabilities, GatherJob } from '../api/client';

interface GatherType {
  type: string;
  title: string;
  desc: string;
  detected: boolean;
}

const GATHER_TYPES: Omit<GatherType, 'detected'>[] = [
  { type: 'virtualization', title: 'Virtualization', desc: 'OpenShift Virtualization / KubeVirt' },
  { type: 'odf', title: 'ODF (Storage)', desc: 'OpenShift Data Foundation / Ceph' },
  { type: 'acm', title: 'ACM', desc: 'Advanced Cluster Management' },
  { type: 'logging', title: 'Logging', desc: 'OpenShift Logging / Cluster Logging' },
  { type: 'service-mesh', title: 'Service Mesh', desc: 'OpenShift Service Mesh / Istio' },
  { type: 'compliance', title: 'Compliance', desc: 'Compliance Operator scans' },
  { type: 'mtc', title: 'MTC', desc: 'Migration Toolkit for Containers' },
  { type: 'gitops', title: 'GitOps', desc: 'OpenShift GitOps / Argo CD' },
  { type: 'serverless', title: 'Serverless', desc: 'OpenShift Serverless / Knative' },
  { type: 'mce', title: 'MCE', desc: 'Multicluster Engine / Hosted Control Planes' },
  { type: 'netobserv', title: 'Network Observability', desc: 'Network Observability Operator' },
  { type: 'local-storage', title: 'Local Storage', desc: 'Local Storage Operator' },
  { type: 'sandboxed', title: 'Sandboxed Containers', desc: 'OpenShift Sandboxed Containers' },
  { type: 'nhc', title: 'Node Health Check', desc: 'Workload Availability Operators' },
  { type: 'numa', title: 'NUMA Resources', desc: 'NUMA Resources Operator' },
  { type: 'ptp', title: 'PTP', desc: 'PTP Operator' },
  { type: 'secrets-store', title: 'Secrets Store CSI', desc: 'Secrets Store CSI Driver Operator' },
  { type: 'lvms', title: 'LVMS', desc: 'LVM Storage Operator' },
  { type: 'audit', title: 'Audit Logs', desc: 'API server audit logging records' },
];

const GatherPage: React.FC = () => {
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [clusterID, setClusterID] = useState<string>('');
  const [selectedTypes, setSelectedTypes] = useState<Set<string>>(new Set());
  const [anonymize, setAnonymize] = useState(false);
  const [anonIPs, setAnonIPs] = useState(true);
  const [anonMACs, setAnonMACs] = useState(true);
  const [anonDomains, setAnonDomains] = useState(true);
  const [anonServices, setAnonServices] = useState(true);
  const [anonSecrets, setAnonSecrets] = useState(false);
  const [sinceEnabled, setSinceEnabled] = useState(false);
  const [since, setSince] = useState('24h');
  const [jobs, setJobs] = useState<GatherJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploadEnabled, setUploadEnabled] = useState(false);

  useEffect(() => {
    Promise.all([
      api.getCapabilities().then(setCaps).catch(() => null),
      api.getClusterID().then((d) => setClusterID(d.id)).catch(() => null),
      api.listGatherJobs().then((d) => setJobs(Array.isArray(d) ? d : [])).catch(() => null),
      api.getUploadConfig().then((d) => setUploadEnabled(d.configured)).catch(() => null),
    ]).finally(() => setLoading(false));
  }, []);

  const pollJobs = useCallback(() => {
    api.listGatherJobs().then((d) => setJobs(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  useEffect(() => {
    const interval = setInterval(pollJobs, 5000);
    return () => clearInterval(interval);
  }, [pollJobs]);

  const toggleType = (type: string) => {
    setSelectedTypes((prev) => {
      const next = new Set(prev);
      if (type === 'all') {
        if (next.has('all')) {
          next.clear();
        } else {
          next.clear();
          next.add('all');
        }
      } else {
        next.delete('all');
        next.has(type) ? next.delete(type) : next.add(type);
      }
      return next;
    });
  };

  const startGather = async () => {
    const types = selectedTypes.has('all') ? ['all'] : selectedTypes.size > 0 ? Array.from(selectedTypes) : ['default'];
    const body: Record<string, unknown> = { types };
    if (anonymize) {
      body.anonymize = true;
      body.anonOpts = {
        ips: anonIPs,
        macs: anonMACs,
        domains: anonDomains,
        services: anonServices,
        secrets: anonSecrets,
      };
    }
    if (sinceEnabled) body.since = since;

    try {
      await api.startGather(body);
      pollJobs();
    } catch (e) {
      console.error('Failed to start gather', e);
    }
  };

  const isDetected = (type: string): boolean => {
    if (!caps) return false;
    const capMap: Record<string, string> = {
      virtualization: 'cnv', odf: 'odf', acm: 'acm',
    };
    const key = capMap[type];
    return key ? Boolean((caps as Record<string, unknown>)[key]) : Boolean((caps as Record<string, unknown>)[type]);
  };

  if (loading) {
    return (
      <SupportPageLayout title="Support - Must-Gather" description="Collect diagnostic data from the cluster for troubleshooting.">
        <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
      </SupportPageLayout>
    );
  }

  return (
    <SupportPageLayout title="Support - Must-Gather" description="Collect diagnostic data from the cluster for troubleshooting.">
      {clusterID && (
        <Card isCompact className="pf-v5-u-mb-lg">
          <CardTitle>Cluster Information</CardTitle>
          <CardBody>
            <Flex alignItems={{ default: 'alignItemsCenter' }} gap={{ default: 'gapSm' }}>
              <FlexItem><strong>Cluster UUID:</strong></FlexItem>
              <FlexItem><ClipboardCopy isReadOnly variant="inline-compact">{clusterID}</ClipboardCopy></FlexItem>
            </Flex>
          </CardBody>
        </Card>
      )}

      <Card isCompact className="pf-v5-u-mb-lg">
        <CardTitle>Run Must-Gather</CardTitle>
        <CardBody>
          <p className="pf-v5-u-mb-md"><strong>Default Must-Gather</strong> always runs. Select additional gathers below:</p>
          <Flex gap={{ default: 'gapSm' }} flexWrap={{ default: 'wrap' }} className="pf-v5-u-mb-lg">
            {GATHER_TYPES.filter((gt) => gt.type === 'audit' || gt.type === 'all' || isDetected(gt.type)).map((gt) => (
              <Button
                key={gt.type}
                variant={selectedTypes.has(gt.type) ? 'primary' : 'secondary'}
                onClick={() => toggleType(gt.type)}
                size="sm"
              >
                {gt.title}
              </Button>
            ))}
            <Button
              variant={selectedTypes.has('all') ? 'primary' : 'secondary'}
              onClick={() => toggleType('all')}
              size="sm"
            >
              Gather All
            </Button>
          </Flex>

          <Divider className="pf-v5-u-mb-lg" />

          <Flex alignItems={{ default: 'alignItemsFlexStart' }} gap={{ default: 'gapLg' }} className="pf-v5-u-mb-md">
            <FlexItem>
              <FormGroup label="Anonymize" isInline>
                <ToggleGroup>
                  <ToggleGroupItem text="Off" buttonId="gather-anon-off" isSelected={!anonymize} onChange={() => setAnonymize(false)} />
                  <ToggleGroupItem text="On" buttonId="gather-anon-on" isSelected={anonymize} onChange={() => setAnonymize(true)} />
                </ToggleGroup>
              </FormGroup>
              {anonymize && (
                <Flex direction={{ default: 'column' }} gap={{ default: 'gapXs' }} className="pf-v5-u-mt-sm">
                  <Checkbox id="anon-ips" label="IP addresses" isChecked={anonIPs} onChange={(_e, v) => setAnonIPs(v)} />
                  <Checkbox id="anon-macs" label="MAC addresses" isChecked={anonMACs} onChange={(_e, v) => setAnonMACs(v)} />
                  <Checkbox id="anon-domains" label="Domain names" isChecked={anonDomains} onChange={(_e, v) => setAnonDomains(v)} />
                  <Checkbox id="anon-services" label="Service names" isChecked={anonServices} onChange={(_e, v) => setAnonServices(v)} />
                  <Checkbox id="anon-secrets" label="Secrets" isChecked={anonSecrets} onChange={(_e, v) => setAnonSecrets(v)} />
                </Flex>
              )}
            </FlexItem>
            <FlexItem>
              <FormGroup label="Time frame" isInline>
                <ToggleGroup>
                  <ToggleGroupItem text="Off" buttonId="gather-since-off" isSelected={!sinceEnabled} onChange={() => setSinceEnabled(false)} />
                  <ToggleGroupItem text="On" buttonId="gather-since-on" isSelected={sinceEnabled} onChange={() => setSinceEnabled(true)} />
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
            </FlexItem>
          </Flex>

          <Flex justifyContent={{ default: 'justifyContentFlexEnd' }}>
            <Button variant="primary" onClick={startGather}>Start Gathering</Button>
          </Flex>
        </CardBody>
      </Card>

      <Card isCompact>
        <CardTitle>Jobs</CardTitle>
        <CardBody>
          {jobs.length === 0 ? (
            <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No must-gather jobs yet. Click Start Gathering above.</p>
          ) : (
            jobs.map((job) => (
              <JobCard
                key={job.id}
                job={job}
                uploadEnabled={uploadEnabled}
                downloadURL={api.getGatherDownloadURL(job.id)}
                onStop={async () => { await api.stopGather(job.id); pollJobs(); }}
                onDelete={async () => { await api.deleteGather(job.id); setJobs((prev) => prev.filter((j) => j.id !== job.id)); }}
              />
            ))
          )}
        </CardBody>
      </Card>
    </SupportPageLayout>
  );
};

export default GatherPage;
