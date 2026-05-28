import { useEffect, useState } from 'react';
import { Alert, Input, Modal } from 'antd';
import type { FormulaDashboardStep } from '../../types';

const { TextArea } = Input;

export function RetryStepModal({ step, open, onCancel, onSubmit }: { step: FormulaDashboardStep | null; open: boolean; onCancel: () => void; onSubmit: (stepID: string, advice?: string) => Promise<void> }) {
  const [advice, setAdvice] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const isScript = step?.execution === 'script';

  useEffect(() => {
    if (open) setAdvice('');
  }, [open, step?.id]);

  const submit = async () => {
    if (!step) return;
    setSubmitting(true);
    try {
      await onSubmit(step.id, isScript ? '' : advice);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={open}
      title={isScript ? `Retry script step: ${step?.title || ''}` : `Retry agent step: ${step?.title || ''}`}
      okText="Restart step"
      onOk={submit}
      onCancel={onCancel}
      confirmLoading={submitting}
      width={640}
    >
      {isScript ? (
        <Alert type="info" showIcon message="This script step will be restarted with the same command and context." />
      ) : (
        <>
          <Alert type="info" showIcon message="Add optional guidance. It will be appended to the agent prompt for this retry." style={{ marginBottom: 16 }} />
          <TextArea rows={6} value={advice} onChange={event => setAdvice(event.target.value)} placeholder="Tell the agent what to adjust before retrying…" />
        </>
      )}
    </Modal>
  );
}
