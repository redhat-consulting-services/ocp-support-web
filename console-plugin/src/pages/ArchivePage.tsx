import React, { useEffect, useState, useCallback } from 'react';
import {
  Card, CardTitle, CardBody,
  Button,
  Flex, FlexItem,
  FormSelect, FormSelectOption,
  Label,
  Spinner,
  Alert,
} from '@patternfly/react-core';
import SupportPageLayout from '../components/SupportPageLayout';
import { api, ArchiveEntry } from '../api/client';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function formatDate(ts: string): string {
  if (!ts) return '';
  return new Date(ts).toLocaleString();
}

const ArchivePage: React.FC = () => {
  const [archives, setArchives] = useState<ArchiveEntry[]>([]);
  const [clusterFilter, setClusterFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);

  const loadArchives = useCallback(async (cluster?: string) => {
    try {
      const data = await api.listArchives(cluster || undefined);
      setArchives(data);
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);

  useEffect(() => {
    loadArchives().finally(() => setLoading(false));
  }, [loadArchives]);

  const handleClusterFilter = (cluster: string) => {
    setClusterFilter(cluster);
    loadArchives(cluster);
  };

  const handleDelete = async (key: string) => {
    setDeleting(key);
    try {
      await api.deleteArchive(key);
      setArchives((prev) => prev.filter((a) => a.key !== key));
    } catch (e) {
      setError(`Delete failed: ${(e as Error).message}`);
    } finally {
      setDeleting(null);
    }
  };

  const clusters = [...new Set(archives.map((a) => a.clusterName).filter(Boolean))].sort();

  if (loading) {
    return (
      <SupportPageLayout title="Archive">
        <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
      </SupportPageLayout>
    );
  }

  return (
    <SupportPageLayout title="Archive" description="Previously gathered must-gather archives stored in S3.">
      {error && <Alert variant="danger" isInline title={error} className="pf-v5-u-mb-md" />}

      {clusters.length > 1 && (
        <Card isCompact className="pf-v5-u-mb-lg">
          <CardBody>
            <FormSelect value={clusterFilter} onChange={(_e, v) => handleClusterFilter(v)} style={{ maxWidth: 300 }}>
              <FormSelectOption value="" label="All clusters" />
              {clusters.map((c) => (
                <FormSelectOption key={c} value={c} label={c} />
              ))}
            </FormSelect>
          </CardBody>
        </Card>
      )}

      <Card isCompact>
        <CardTitle>Archives</CardTitle>
        <CardBody>
          {archives.length === 0 ? (
            <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No archives found.</p>
          ) : (
            <table className="pf-v5-c-table pf-m-compact" style={{ width: '100%' }}>
              <thead>
                <tr>
                  <th>File</th>
                  <th>Cluster</th>
                  <th>Type</th>
                  <th>Date</th>
                  <th>Size</th>
                  <th>Source</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {archives.map((a) => (
                  <tr key={a.key}>
                    <td><strong>{a.fileName}</strong>{a.description && <><br /><span className="pf-v5-u-font-size-xs pf-v5-u-color-200">{a.description}</span></>}</td>
                    <td>{a.clusterName || '-'}</td>
                    <td><Label isCompact>{a.gatherType || 'default'}</Label></td>
                    <td className="pf-v5-u-font-size-sm">{formatDate(a.timestamp)}</td>
                    <td className="pf-v5-u-font-size-sm">{formatBytes(a.sizeBytes)}</td>
                    <td><Label isCompact color={a.source === 'remote' ? 'blue' : 'grey'}>{a.source || 'local'}</Label></td>
                    <td>
                      <Flex gap={{ default: 'gapSm' }}>
                        <Button variant="primary" size="sm" component="a" href={api.getArchiveDownloadURL(a.key)}>Download</Button>
                        <Button
                          variant="danger"
                          size="sm"
                          isLoading={deleting === a.key}
                          isDisabled={deleting === a.key}
                          onClick={() => handleDelete(a.key)}
                        >
                          Delete
                        </Button>
                      </Flex>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardBody>
      </Card>
    </SupportPageLayout>
  );
};

export default ArchivePage;
