import React, { useEffect, useState, useCallback } from 'react';
import {
  Card, CardTitle, CardBody,
  Button,
  Flex, FlexItem,
  FormSelect, FormSelectOption,
  TextInput,
  Spinner,
  Modal, ModalVariant,
  ExpandableSection,
  CodeBlock, CodeBlockCode,
} from '@patternfly/react-core';
import SupportPageLayout from '../components/SupportPageLayout';
import { api } from '../api/client';

interface ResourceItem {
  name: string;
  created: string;
}

interface NamespaceResource {
  group: string;
  version: string;
  resource: string;
  kind: string;
  items: ResourceItem[];
}

function formatAge(timestamp: string): string {
  if (!timestamp) return '';
  const d = new Date(timestamp);
  const now = new Date();
  const diff = Math.floor((now.getTime() - d.getTime()) / 1000);
  if (diff < 0) return '0s';
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
  return `${Math.floor(diff / 86400)}d`;
}

const ResourcesPage: React.FC = () => {
  const [allNamespaces, setAllNamespaces] = useState<string[]>([]);
  const [selectedNS, setSelectedNS] = useState('');
  const [resources, setResources] = useState<NamespaceResource[]>([]);
  const [filter, setFilter] = useState('');
  const [scanning, setScanning] = useState(false);
  const [loading, setLoading] = useState(true);

  const [yamlModal, setYamlModal] = useState<{ title: string; content: string } | null>(null);
  const [yamlLoading, setYamlLoading] = useState(false);

  useEffect(() => {
    api.getNamespaces()
      .then(setAllNamespaces)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const scanNamespace = useCallback(async (ns: string) => {
    setSelectedNS(ns);
    if (!ns) {
      setResources([]);
      return;
    }
    setScanning(true);
    try {
      const res = await api.getNamespaceResources(ns);
      setResources(res as NamespaceResource[]);
    } catch {
      setResources([]);
    } finally {
      setScanning(false);
    }
  }, []);

  const viewResource = async (group: string, version: string, resource: string, name: string) => {
    setYamlModal({ title: `${resource}/${name}`, content: '' });
    setYamlLoading(true);
    try {
      const yaml = await api.getResource(selectedNS, group, version, resource, name);
      setYamlModal({ title: `${resource}/${name}`, content: yaml });
    } catch {
      setYamlModal({ title: `${resource}/${name}`, content: 'Failed to load resource.' });
    } finally {
      setYamlLoading(false);
    }
  };

  const filtered = filter
    ? resources.filter((r) =>
        r.resource.includes(filter.toLowerCase()) ||
        r.kind.toLowerCase().includes(filter.toLowerCase()) ||
        (r.group && r.group.includes(filter.toLowerCase()))
      )
    : resources;

  const grouped = filtered.reduce<Record<string, NamespaceResource[]>>((acc, r) => {
    const key = r.group || '';
    if (!acc[key]) acc[key] = [];
    acc[key].push(r);
    return acc;
  }, {});

  const groupKeys = Object.keys(grouped).sort((a, b) => {
    if (a === '') return -1;
    if (b === '') return 1;
    return a.localeCompare(b);
  });

  if (loading) {
    return (
      <SupportPageLayout title="Resource Browser">
        <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
      </SupportPageLayout>
    );
  }

  return (
    <SupportPageLayout title="Resource Browser" description="Browse API resources by namespace. Select a namespace and click a resource type to list objects.">
      <Card isCompact className="pf-v5-u-mb-lg">
        <CardBody>
          <Flex gap={{ default: 'gapMd' }} alignItems={{ default: 'alignItemsFlexEnd' }}>
            <FlexItem style={{ minWidth: 300 }}>
              <FormSelect
                value={selectedNS}
                onChange={(_e, v) => scanNamespace(v)}
              >
                <FormSelectOption value="" label="-- Select a namespace --" isPlaceholder />
                {allNamespaces.map((n) => (
                  <FormSelectOption key={n} value={n} label={n} />
                ))}
              </FormSelect>
            </FlexItem>
            {resources.length > 0 && (
              <FlexItem>
                <TextInput
                  type="text"
                  placeholder="Filter resources..."
                  value={filter}
                  onChange={(_e, v) => setFilter(v)}
                  style={{ maxWidth: 250 }}
                />
              </FlexItem>
            )}
          </Flex>
        </CardBody>
      </Card>

      {scanning && (
        <Flex justifyContent={{ default: 'justifyContentCenter' }} className="pf-v5-u-py-xl">
          <Spinner size="lg" />
          <p className="pf-v5-u-ml-sm">Scanning resources in {selectedNS}...</p>
        </Flex>
      )}

      {!scanning && selectedNS && resources.length === 0 && (
        <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No resources found in {selectedNS}.</p>
      )}

      {!scanning && selectedNS && filtered.length === 0 && resources.length > 0 && (
        <p className="pf-v5-u-color-200 pf-v5-u-text-align-center">No matching resources found.</p>
      )}

      {!scanning && groupKeys.map((groupKey) => (
        <div key={groupKey} className="pf-v5-u-mb-lg">
          <h3 className="pf-v5-u-mb-sm pf-v5-u-font-weight-bold">
            {groupKey || 'core'} ({grouped[groupKey][0].version})
          </h3>
          {grouped[groupKey].map((r) => (
            <ExpandableSection
              key={`${r.group}-${r.resource}`}
              toggleText={`${r.kind} (${r.items.length})`}
              className="pf-v5-u-mb-sm"
            >
              <Card isCompact>
                <CardBody>
                  <table className="pf-v5-c-table pf-m-compact" style={{ width: '100%' }}>
                    <thead>
                      <tr>
                        <th>Name</th>
                        <th>Age</th>
                        <th></th>
                      </tr>
                    </thead>
                    <tbody>
                      {r.items.map((item) => (
                        <tr key={item.name}>
                          <td><strong>{item.name}</strong></td>
                          <td>{formatAge(item.created)}</td>
                          <td>
                            <Button
                              variant="link"
                              size="sm"
                              onClick={() => viewResource(r.group, r.version, r.resource, item.name)}
                            >
                              View YAML
                            </Button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </CardBody>
              </Card>
            </ExpandableSection>
          ))}
        </div>
      ))}

      {!scanning && !selectedNS && (
        <p className="pf-v5-u-color-200 pf-v5-u-text-align-center pf-v5-u-py-xl">
          Select a namespace to browse resources.
        </p>
      )}

      {yamlModal && (
        <Modal
          variant={ModalVariant.large}
          title={yamlModal.title}
          isOpen
          onClose={() => setYamlModal(null)}
          actions={[
            <Button key="copy" variant="secondary" onClick={() => navigator.clipboard.writeText(yamlModal.content)}>Copy to clipboard</Button>,
            <Button key="close" variant="link" onClick={() => setYamlModal(null)}>Close</Button>,
          ]}
        >
          {yamlLoading ? (
            <Flex justifyContent={{ default: 'justifyContentCenter' }}><Spinner size="lg" /></Flex>
          ) : (
            <CodeBlock><CodeBlockCode>{yamlModal.content}</CodeBlockCode></CodeBlock>
          )}
        </Modal>
      )}
    </SupportPageLayout>
  );
};

export default ResourcesPage;
