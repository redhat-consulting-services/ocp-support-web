import React, { useState } from 'react';
import {
  Card, CardBody,
  Button,
  Flex, FlexItem,
  Label,
  FormGroup,
  Progress, ProgressVariant,
  Alert,
  ExpandableSection,
  CodeBlock, CodeBlockCode,
  Modal, ModalVariant,
  TextInput,
} from '@patternfly/react-core';
import { api, GatherJob } from '../api/client';

interface JobCardProps {
  job: GatherJob;
  uploadEnabled: boolean;
  downloadURL: string;
  onStop?: () => void;
  onDelete?: () => void;
}

const JobCard: React.FC<JobCardProps> = ({ job, uploadEnabled, downloadURL, onStop, onDelete }) => {
  const isComplete = job.status === 'complete';
  const isFailed = job.status === 'failed';
  const isRunning = !isComplete && !isFailed;
  const isDone = isComplete || isFailed;
  const hasDownload = isComplete && !!job.fileName;
  const variant = isFailed ? ProgressVariant.danger : isComplete ? ProgressVariant.success : undefined;

  const progress = isComplete ? 100 : isFailed ? 0
    : job.totalSteps > 0 ? Math.round(job.step * 100 / job.totalSteps) : 0;

  const logLines = job.logOutput ? job.logOutput.split('\n') : [];

  const [uploadModal, setUploadModal] = useState(false);
  const [caseID, setCaseID] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadResult, setUploadResult] = useState<{ type: 'success' | 'danger'; msg: string } | null>(null);

  const handleUpload = async () => {
    setUploading(true);
    setUploadResult(null);
    try {
      await api.uploadToRedHat(job.id, { caseID, internalUser: false });
      setUploadResult({ type: 'success', msg: `Upload started for case ${caseID}` });
      setTimeout(() => setUploadModal(false), 2000);
    } catch (e) {
      setUploadResult({ type: 'danger', msg: `Upload failed: ${(e as Error).message}` });
    } finally {
      setUploading(false);
    }
  };

  return (
    <>
      <Card isCompact className="pf-v5-u-mb-sm">
        <CardBody>
          <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
            <FlexItem>
              <strong>{job.type || 'Must-Gather'}</strong>
              <Label className="pf-v5-u-ml-sm" color={isComplete ? 'green' : isFailed ? 'red' : 'blue'}>
                {job.status}
              </Label>
            </FlexItem>
            <FlexItem>
              <Flex gap={{ default: 'gapSm' }}>
                {isRunning && onStop && <Button variant="danger" size="sm" onClick={onStop}>Stop</Button>}
                {hasDownload && (
                  <Button variant="primary" size="sm" component="a" href={downloadURL}>Download</Button>
                )}
                {hasDownload && uploadEnabled && (
                  <Button variant="secondary" size="sm" onClick={() => { setUploadModal(true); setUploadResult(null); setCaseID(''); }}>
                    Upload to Red Hat
                  </Button>
                )}
                {isDone && onDelete && (
                  <Button variant="danger" size="sm" onClick={onDelete}>Delete</Button>
                )}
              </Flex>
            </FlexItem>
          </Flex>
          <Progress
            value={progress}
            title={job.stepLabel || ''}
            variant={variant}
            className="pf-v5-u-mt-sm"
          />
          {job.error && <Alert variant="danger" isInline isPlain title={job.error} className="pf-v5-u-mt-sm" />}
          {logLines.length > 0 && (
            <ExpandableSection toggleText={`Logs (${logLines.length} lines)`} className="pf-v5-u-mt-sm">
              <CodeBlock><CodeBlockCode>{logLines.join('\n')}</CodeBlockCode></CodeBlock>
            </ExpandableSection>
          )}
        </CardBody>
      </Card>

      {uploadModal && (
        <Modal
          variant={ModalVariant.small}
          title="Upload to Red Hat"
          isOpen
          onClose={() => setUploadModal(false)}
          actions={[
            <Button key="upload" variant="primary" onClick={handleUpload} isDisabled={!caseID || uploading} isLoading={uploading}>
              Upload
            </Button>,
            <Button key="cancel" variant="link" onClick={() => setUploadModal(false)}>Cancel</Button>,
          ]}
        >
          {uploadResult && <Alert variant={uploadResult.type} isInline title={uploadResult.msg} className="pf-v5-u-mb-md" />}
          <FormGroup label="Case ID" isRequired className="pf-v5-u-mb-md" fieldId={`upload-case-${job.id}`}>
            <TextInput
              id={`upload-case-${job.id}`}
              value={caseID}
              onChange={(_e, v) => setCaseID(v)}
              placeholder="e.g. 02527285"
            />
          </FormGroup>
          <p className="pf-v5-u-font-size-sm pf-v5-u-color-200">
            The must-gather archive will be uploaded to Red Hat&apos;s secure SFTP server and attached to the specified support case.
          </p>
        </Modal>
      )}
    </>
  );
};

export default JobCard;
