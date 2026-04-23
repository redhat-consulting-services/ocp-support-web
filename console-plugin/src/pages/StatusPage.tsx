import React, { useEffect, useState } from 'react';
import {
  Card, CardTitle, CardBody,
  Grid, GridItem,
  Label,
  Flex, FlexItem,
  Progress,
  Spinner,
} from '@patternfly/react-core';
import SupportPageLayout from '../components/SupportPageLayout';
import { api, ClusterHealth, NodeInfo, TopPod, EtcdHealth } from '../api/client';

const StatusPage: React.FC = () => {
  const [health, setHealth] = useState<ClusterHealth | null>(null);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [topPods, setTopPods] = useState<TopPod[]>([]);
  const [etcd, setEtcd] = useState<EtcdHealth | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api.getClusterHealth().then(setHealth),
      api.getNodeUtilization().then(setNodes),
      api.getTopConsumers().then((d) => setTopPods(d.pods || [])).catch(() => null),
      api.getEtcdHealth().then(setEtcd).catch(() => null),
    ])
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <SupportPageLayout title="Cluster Status" description="Overview of cluster health, operators, node utilization, and top consumers.">
        <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
      </SupportPageLayout>
    );
  }

  if (error) {
    return (
      <SupportPageLayout title="Cluster Status">
        <Card><CardBody>Error loading status: {error}</CardBody></Card>
      </SupportPageLayout>
    );
  }

  const statusColor = (s: string) => s === 'Available' || s === 'Ready' ? 'green' : s === 'Degraded' ? 'red' : 'orange';

  return (
    <SupportPageLayout title="Cluster Status" description="Overview of cluster health, operators, node utilization, and top consumers.">
      <h2 className="pf-v5-u-mb-md">Cluster Health</h2>
      <Grid hasGutter className="pf-v5-u-mb-lg">
        {health && (
          <>
            <GridItem md={4}>
              <Card isCompact isFullHeight>
                <CardTitle>Cluster</CardTitle>
                <CardBody>
                  <Flex direction={{ default: 'column' }} gap={{ default: 'gapSm' }}>
                    <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }}>
                      <FlexItem><strong>Version</strong></FlexItem>
                      <FlexItem>{health.version}</FlexItem>
                    </Flex>
                    <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }}>
                      <FlexItem><strong>Status</strong></FlexItem>
                      <FlexItem><Label color={statusColor(health.status)}>{health.status}</Label></FlexItem>
                    </Flex>
                    {health.etcdEncryption && (
                      <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }}>
                        <FlexItem><strong>ETCD Encryption</strong></FlexItem>
                        <FlexItem>
                          <Label color={health.etcdEncryption === 'Encrypted' ? 'green' : 'orange'}>
                            {health.etcdEncryption}
                          </Label>
                        </FlexItem>
                      </Flex>
                    )}
                    {health.odf && health.odf.installed && (
                      <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }}>
                        <FlexItem><strong>ODF</strong></FlexItem>
                        <FlexItem>
                          <Label color={health.odf.phase === 'Ready' ? 'green' : 'orange'}>
                            {health.odf.phase || 'Unknown'}
                          </Label>
                        </FlexItem>
                      </Flex>
                    )}
                  </Flex>
                </CardBody>
              </Card>
            </GridItem>
            <GridItem md={4}>
              <Card isCompact isFullHeight>
                <CardTitle>Control Plane</CardTitle>
                <CardBody>
                  <Flex direction={{ default: 'column' }} gap={{ default: 'gapSm' }}>
                    {health.controlPlane.map((op) => (
                      <Flex key={op.name} justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
                        <FlexItem>{op.name}</FlexItem>
                        <FlexItem>
                          <Label color={op.degraded ? 'red' : op.available ? 'green' : 'orange'}>
                            {op.degraded ? 'Degraded' : op.available ? 'Available' : 'Unavailable'}
                          </Label>
                        </FlexItem>
                      </Flex>
                    ))}
                  </Flex>
                </CardBody>
              </Card>
            </GridItem>
            <GridItem md={4}>
              <Card isCompact isFullHeight>
                <CardTitle>Operators</CardTitle>
                <CardBody>
                  <Flex direction={{ default: 'column' }} gap={{ default: 'gapSm' }}>
                    {health.operators.map((op) => (
                      <Flex key={op.name} justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
                        <FlexItem className="pf-v5-u-font-size-sm">{op.name}</FlexItem>
                        <FlexItem>
                          <Label color={op.degraded ? 'red' : op.available ? 'green' : 'orange'}>
                            {op.degraded ? 'Degraded' : op.available ? 'Available' : 'Unavailable'}
                          </Label>
                        </FlexItem>
                      </Flex>
                    ))}
                  </Flex>
                </CardBody>
              </Card>
            </GridItem>
          </>
        )}
      </Grid>

      {etcd && (
        <>
          <h2 className="pf-v5-u-mb-md">ETCD Health</h2>
          <Card isCompact className="pf-v5-u-mb-lg">
            <CardBody>
              <Flex gap={{ default: 'gapSm' }} className="pf-v5-u-mb-md" alignItems={{ default: 'alignItemsCenter' }}>
                <FlexItem><strong>Status:</strong></FlexItem>
                <FlexItem>
                  <Label color={etcd.healthy ? 'green' : 'red'}>{etcd.healthy ? 'Healthy' : 'Unhealthy'}</Label>
                </FlexItem>
              </Flex>
              <table className="pf-v5-c-table pf-m-compact" style={{ width: '100%' }}>
                <thead>
                  <tr>
                    <th>Member</th>
                    <th>Leader</th>
                    <th>Revision</th>
                    <th>DB Size</th>
                  </tr>
                </thead>
                <tbody>
                  {etcd.members.map((m) => (
                    <tr key={m.name}>
                      <td><strong>{m.name}</strong></td>
                      <td>{m.isLeader ? <Label color="blue" isCompact>Leader</Label> : '-'}</td>
                      <td>{m.revision}</td>
                      <td>{m.dbSizeMB.toFixed(1)} MB</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </CardBody>
          </Card>
        </>
      )}

      <h2 className="pf-v5-u-mb-md">Node Utilization</h2>
      <Card isCompact className="pf-v5-u-mb-lg">
        <CardBody>
          {nodes.length === 0 ? (
            <p className="pf-v5-u-color-200">No node data</p>
          ) : (
            <table className="pf-v5-c-table pf-m-compact" style={{ width: '100%' }}>
              <thead>
                <tr>
                  <th style={{ width: '20%' }}>Node</th>
                  <th style={{ width: '8%' }}>Status</th>
                  <th style={{ width: '28%' }}>CPU Usage</th>
                  <th style={{ width: '28%' }}>Memory Usage</th>
                  <th style={{ width: '8%' }}>Pods</th>
                  <th style={{ width: '8%' }}>CPU Req%</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => {
                  const cpuPct = n.cpuAllocatable > 0 ? (n.cpuUsage / n.cpuAllocatable) * 100 : 0;
                  const memPct = n.memAllocatable > 0 ? (n.memUsage / n.memAllocatable) * 100 : 0;
                  return (
                    <tr key={n.name}>
                      <td><strong>{n.name.split('.')[0]}</strong><br /><span className="pf-v5-u-font-size-xs pf-v5-u-color-200">{n.roles.join(', ')}</span></td>
                      <td><Label color={statusColor(n.status)} isCompact>{n.status}</Label></td>
                      <td><Progress value={cpuPct} measureLocation="outside" size="sm" aria-label="CPU" /><span className="pf-v5-u-font-size-xs">{cpuPct.toFixed(0)}%</span></td>
                      <td><Progress value={memPct} measureLocation="outside" size="sm" aria-label="Memory" /><span className="pf-v5-u-font-size-xs">{memPct.toFixed(0)}%</span></td>
                      <td>{n.pods}</td>
                      <td>{n.cpuOvercommitPct.toFixed(0)}%</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </CardBody>
      </Card>

      {topPods.length > 0 && (
        <>
          <h2 className="pf-v5-u-mb-md">Top Pods (by resource usage)</h2>
          <Card isCompact className="pf-v5-u-mb-lg">
            <CardBody>
              <table className="pf-v5-c-table pf-m-compact" style={{ width: '100%' }}>
                <thead>
                  <tr>
                    <th>Pod</th>
                    <th>Namespace</th>
                    <th>CPU</th>
                    <th>Memory</th>
                  </tr>
                </thead>
                <tbody>
                  {topPods.slice(0, 15).map((p) => (
                    <tr key={`${p.namespace}/${p.name}`}>
                      <td><strong>{p.name}</strong></td>
                      <td className="pf-v5-u-font-size-sm">{p.namespace}</td>
                      <td>{p.cpuStr}</td>
                      <td>{p.memStr}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </CardBody>
          </Card>
        </>
      )}
    </SupportPageLayout>
  );
};

export default StatusPage;
