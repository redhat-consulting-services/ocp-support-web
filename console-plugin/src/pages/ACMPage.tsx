import React, { useEffect, useState, useCallback, useRef } from 'react';
import {
  Card, CardTitle, CardBody,
  Button,
  Flex, FlexItem,
  Label,
  Checkbox,
  FormGroup,
  FormSelect, FormSelectOption,
  ToggleGroup, ToggleGroupItem,
  Modal, ModalVariant,
  Alert,
  Spinner,
} from '@patternfly/react-core';
import { EllipsisVIcon } from '@patternfly/react-icons';
import SupportPageLayout from '../components/SupportPageLayout';
import JobCard from '../components/JobCard';
import { api, ManagedCluster, GatherJob } from '../api/client';

interface ClusterOperator {
  type: string;
  label: string;
  version?: string;
}

const ACMPage: React.FC = () => {
  const [clusters, setClusters] = useState<ManagedCluster[]>([]);
  const [jobs, setJobs] = useState<GatherJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [agentVersions, setAgentVersions] = useState<Record<string, string>>({});
  const [menuOpen, setMenuOpen] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  const [modalCluster, setModalCluster] = useState<string | null>(null);
  const [modalOperators, setModalOperators] = useState<ClusterOperator[]>([]);
  const [modalSelected, setModalSelected] = useState<Set<string>>(new Set());
  const [modalSince, setModalSince] = useState('');
  const [modalAnonymize, setModalAnonymize] = useState(false);
  const [modalAnonIPs, setModalAnonIPs] = useState(true);
  const [modalAnonMACs, setModalAnonMACs] = useState(true);
  const [modalAnonDomains, setModalAnonDomains] = useState(true);
  const [modalAnonServices, setModalAnonServices] = useState(true);
  const [modalAnonSecrets, setModalAnonSecrets] = useState(false);
  const [modalLoading, setModalLoading] = useState(false);
  const [uploadEnabled, setUploadEnabled] = useState(false);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(null);
      }
    };
    document.addEventListener('click', handleClickOutside);
    return () => document.removeEventListener('click', handleClickOutside);
  }, []);

  const loadClusters = useCallback(async () => {
    try {
      const data = await api.getACMClusters();
      setClusters(Array.isArray(data) ? data : []);
      setError(null);
      (Array.isArray(data) ? data : []).filter((c) => c.agentReady).forEach(async (c) => {
        try {
          const ver = await api.getAgentVersion(c.name);
          setAgentVersions((prev) => ({ ...prev, [c.name]: ver.version }));
        } catch { /* ignore */ }
      });
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);

  const pollJobs = useCallback(() => {
    api.listRemoteGatherJobs().then((data) => setJobs(Array.isArray(data) ? data : [])).catch(() => {});
  }, []);

  useEffect(() => {
    Promise.all([
      loadClusters(),
      pollJobs(),
      api.getUploadConfig().then((d) => setUploadEnabled(d.configured)).catch(() => null),
    ]).finally(() => setLoading(false));
  }, [loadClusters, pollJobs]);

  useEffect(() => {
    const interval = setInterval(pollJobs, 5000);
    return () => clearInterval(interval);
  }, [pollJobs]);

  useEffect(() => {
    const hasDeploying = clusters.some((c) => c.agentDeployed && !c.agentReady && c.status === 'Available');
    if (!hasDeploying) return;
    const interval = setInterval(loadClusters, 10000);
    return () => clearInterval(interval);
  }, [clusters, loadClusters]);

  const openGatherModal = async (clusterName: string) => {
    setModalCluster(clusterName);
    setModalSelected(new Set());
    setModalSince('');
    setModalAnonymize(false);
    setModalLoading(true);
    try {
      const ops = await api.getClusterOperators(clusterName) as ClusterOperator[];
      setModalOperators(Array.isArray(ops) ? ops : []);
    } catch {
      setModalOperators([]);
    } finally {
      setModalLoading(false);
    }
  };

  const startRemoteGather = async () => {
    if (!modalCluster) return;
    const types = ['default', ...Array.from(modalSelected)];
    try {
      const req: Record<string, unknown> = {
        clusterName: modalCluster,
        gatherTypes: types,
        since: modalSince,
        anonymize: modalAnonymize,
      };
      if (modalAnonymize) {
        req.anonOpts = {
          ips: modalAnonIPs,
          macs: modalAnonMACs,
          domains: modalAnonDomains,
          services: modalAnonServices,
          secrets: modalAnonSecrets,
        };
      }
      await api.startRemoteGather(req);
      pollJobs();
    } catch (e) {
      console.error('Failed to start remote gather', e);
    }
    setModalCluster(null);
  };

  const installAgent = async (clusterName: string) => {
    try {
      await api.deployAgent(clusterName);
      loadClusters();
    } catch (e) {
      console.error('Failed to deploy agent', e);
    }
  };

  const removeAgentAction = async (clusterName: string) => {
    try {
      await api.removeAgent(clusterName);
      loadClusters();
    } catch (e) {
      console.error('Failed to remove agent', e);
    }
    setMenuOpen(null);
  };

  const reinstallAgentAction = async (clusterName: string) => {
    try {
      await api.redeployAgent(clusterName);
      loadClusters();
    } catch (e) {
      console.error('Failed to reinstall agent', e);
    }
    setMenuOpen(null);
  };

  const deleteJob = async (jobId: string) => {
    try {
      await api.deleteRemoteGather(jobId);
      setJobs((prev) => prev.filter((j) => j.id !== jobId));
    } catch (e) {
      console.error('Failed to delete job', e);
    }
  };

  const statusColor = (s: string) => s === 'Available' ? 'green' : s === 'Unavailable' ? 'red' : 'grey';

  if (loading) {
    return (
      <SupportPageLayout title="ACM - Multi-Cluster Must-Gather">
        <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
      </SupportPageLayout>
    );
  }

  return (
    <SupportPageLayout title="ACM - Multi-Cluster Must-Gather" description="Run must-gather on managed clusters via the support agent. Archives are downloaded to the hub for review.">
      {error && <Alert variant="danger" isInline title={error} className="pf-v5-u-mb-md" />}

      <Card isCompact className="pf-v5-u-mb-lg">
        <CardTitle>Managed Clusters</CardTitle>
        <CardBody>
          {clusters.length === 0 ? (
            <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No managed clusters found.</p>
          ) : (
            <table className="pf-v5-c-table pf-m-compact" style={{ width: '100%' }}>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th>OCP Version</th>
                  <th>Platform</th>
                  <th>Agent</th>
                  <th>Agent Version</th>
                  <th>Gather</th>
                  <th style={{ width: '48px' }}></th>
                </tr>
              </thead>
              <tbody>
                {clusters.map((c) => (
                  <tr key={c.name}>
                    <td><strong>{c.name}</strong></td>
                    <td><Label color={statusColor(c.status)}>{c.status}</Label></td>
                    <td>{c.ocpVersion || '-'}</td>
                    <td>{c.platform || '-'}{c.vendor && c.vendor !== 'OpenShift' && c.platform ? ` (${c.vendor})` : ''}</td>
                    <td>
                      {c.agentReady ? (
                        <Label color="green" isCompact>Ready</Label>
                      ) : c.agentDeployed && c.status === 'Available' ? (
                        <Label color="blue" isCompact>Deploying</Label>
                      ) : c.status === 'Available' ? (
                        <Button variant="link" size="sm" onClick={() => installAgent(c.name)}>Install Agent</Button>
                      ) : (
                        <span>-</span>
                      )}
                    </td>
                    <td className="pf-v5-u-font-size-sm">{agentVersions[c.name] || '-'}</td>
                    <td>
                      {c.status === 'Available' && c.agentReady ? (
                        <Button variant="primary" size="sm" onClick={() => openGatherModal(c.name)}>Start Gather</Button>
                      ) : c.agentDeployed && c.status === 'Available' ? (
                        <span className="pf-v5-u-font-size-xs pf-v5-u-color-200">Agent deploying...</span>
                      ) : c.status === 'Available' ? (
                        <span className="pf-v5-u-font-size-xs pf-v5-u-color-200">Agent not installed</span>
                      ) : (
                        <span className="pf-v5-u-font-size-xs pf-v5-u-color-200">Cluster unavailable</span>
                      )}
                    </td>
                    <td>
                      {c.status === 'Available' && c.agentDeployed && (
                        <div ref={menuOpen === c.name ? menuRef : undefined} style={{ position: 'relative' }}>
                          <Button
                            variant="plain"
                            aria-label="Actions"
                            onClick={(e) => { e.stopPropagation(); setMenuOpen(menuOpen === c.name ? null : c.name); }}
                          >
                            <EllipsisVIcon />
                          </Button>
                          {menuOpen === c.name && (
                            <div style={{
                              position: 'absolute', right: 0, top: '100%', zIndex: 100,
                              background: 'var(--pf-v5-global--BackgroundColor--100, #fff)',
                              boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
                              borderRadius: 4, minWidth: 160, padding: '4px 0',
                            }}>
                              <button
                                className="pf-v5-c-menu__item"
                                style={{ display: 'block', width: '100%', textAlign: 'left', padding: '8px 16px', border: 'none', background: 'none', cursor: 'pointer' }}
                                onClick={() => reinstallAgentAction(c.name)}
                              >
                                Reinstall Agent
                              </button>
                              <button
                                className="pf-v5-c-menu__item"
                                style={{ display: 'block', width: '100%', textAlign: 'left', padding: '8px 16px', border: 'none', background: 'none', cursor: 'pointer' }}
                                onClick={() => removeAgentAction(c.name)}
                              >
                                Remove Agent
                              </button>
                            </div>
                          )}
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardBody>
      </Card>

      <Card isCompact>
        <CardTitle>Remote Gather Jobs</CardTitle>
        <CardBody>
          {jobs.length === 0 ? (
            <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No remote gather jobs yet.</p>
          ) : (
            jobs.map((job) => (
              <JobCard
                key={job.id}
                job={job}
                uploadEnabled={uploadEnabled}
                downloadURL={api.getRemoteGatherDownloadURL(job.id)}
                onDelete={() => deleteJob(job.id)}
              />
            ))
          )}
        </CardBody>
      </Card>

      {modalCluster && (
        <Modal
          variant={ModalVariant.medium}
          title={`Gather from ${modalCluster}`}
          isOpen
          onClose={() => setModalCluster(null)}
          actions={[
            <Button key="start" variant="primary" onClick={startRemoteGather} isDisabled={modalLoading}>Start Gather</Button>,
            <Button key="cancel" variant="link" onClick={() => setModalCluster(null)}>Cancel</Button>,
          ]}
        >
          {modalLoading ? (
            <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
          ) : (
            <>
              <FormGroup label="Operator Gathers" className="pf-v5-u-mb-md">
                {modalOperators.length === 0 ? (
                  <p className="pf-v5-u-font-size-sm pf-v5-u-color-200">No additional operators detected. Default gather will run.</p>
                ) : (
                  <Flex gap={{ default: 'gapSm' }} flexWrap={{ default: 'wrap' }}>
                    {modalOperators.map((op) => (
                      <Checkbox
                        key={op.type}
                        id={`modal-op-${op.type}`}
                        label={`${op.label}${op.version ? ` (v${op.version})` : ''}`}
                        isChecked={modalSelected.has(op.type)}
                        onChange={() => {
                          setModalSelected((prev) => {
                            const next = new Set(prev);
                            next.has(op.type) ? next.delete(op.type) : next.add(op.type);
                            return next;
                          });
                        }}
                      />
                    ))}
                  </Flex>
                )}
              </FormGroup>
              <FormGroup label="Log Time Frame" className="pf-v5-u-mb-md">
                <FormSelect value={modalSince} onChange={(_e, v) => setModalSince(v)} style={{ maxWidth: 200 }}>
                  <FormSelectOption value="" label="All logs" />
                  <FormSelectOption value="1h" label="Last 1 hour" />
                  <FormSelectOption value="6h" label="Last 6 hours" />
                  <FormSelectOption value="12h" label="Last 12 hours" />
                  <FormSelectOption value="24h" label="Last 24 hours" />
                  <FormSelectOption value="48h" label="Last 48 hours" />
                  <FormSelectOption value="7d" label="Last 7 days" />
                </FormSelect>
              </FormGroup>
              <FormGroup label="Anonymize" isInline>
                <ToggleGroup>
                  <ToggleGroupItem text="Off" buttonId="acm-anon-off" isSelected={!modalAnonymize} onChange={() => setModalAnonymize(false)} />
                  <ToggleGroupItem text="On" buttonId="acm-anon-on" isSelected={modalAnonymize} onChange={() => setModalAnonymize(true)} />
                </ToggleGroup>
                {modalAnonymize && (
                  <Flex direction={{ default: 'column' }} gap={{ default: 'gapXs' }} className="pf-v5-u-mt-sm">
                    <Checkbox id="acm-anon-ips" label="IP addresses" isChecked={modalAnonIPs} onChange={(_e, v) => setModalAnonIPs(v)} />
                    <Checkbox id="acm-anon-macs" label="MAC addresses" isChecked={modalAnonMACs} onChange={(_e, v) => setModalAnonMACs(v)} />
                    <Checkbox id="acm-anon-domains" label="Domain names" isChecked={modalAnonDomains} onChange={(_e, v) => setModalAnonDomains(v)} />
                    <Checkbox id="acm-anon-services" label="Service names" isChecked={modalAnonServices} onChange={(_e, v) => setModalAnonServices(v)} />
                    <Checkbox id="acm-anon-secrets" label="Secrets" isChecked={modalAnonSecrets} onChange={(_e, v) => setModalAnonSecrets(v)} />
                  </Flex>
                )}
              </FormGroup>
            </>
          )}
        </Modal>
      )}
    </SupportPageLayout>
  );
};

export default ACMPage;
