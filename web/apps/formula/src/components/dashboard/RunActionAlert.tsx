import { Alert, Button, Space } from 'antd';
import type { FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';

export function RunActionAlert({
  snapshot,
  focusedStep,
  onInspect,
  onRetry,
  onOpenFinalReport,
}: {
  snapshot: FormulaDashboardSnapshot;
  focusedStep: FormulaDashboardStep | null;
  onInspect: (step: FormulaDashboardStep) => void;
  onRetry: (step: FormulaDashboardStep) => void;
  onOpenFinalReport: () => void;
}) {
  const waiting = snapshot.steps.find(step => step.status === 'waiting_input' && step.human_input_request) || null;
  const failed = snapshot.steps.find(step => step.status === 'failed') || null;
  const running = snapshot.steps.find(step => step.status === 'running') || null;

  if (waiting) {
    return (
      <Alert
        showIcon
        type="warning"
        message="Action required"
        description={`Step “${waiting.title}” is waiting for human input. Submit the requested values to resume the workflow.`}
        action={<Button type="primary" onClick={() => onInspect(waiting)}>Review input step</Button>}
      />
    );
  }

  if (failed) {
    return (
      <Alert
        showIcon
        type="error"
        message="Run blocked"
        description={`Step “${failed.title}” failed. Inspect the error, optionally add retry advice, then restart from this step.`}
        action={(
          <Space>
            <Button onClick={() => onInspect(failed)}>View error</Button>
            <Button danger type="primary" onClick={() => onRetry(failed)}>Retry</Button>
          </Space>
        )}
      />
    );
  }

  if (snapshot.status === 'completed') {
    return (
      <Alert
        showIcon
        type="success"
        message="Run completed"
        description="The workflow has finished. Review the final report or inspect completed steps for traceability."
        action={<Button type="primary" disabled={!snapshot.final_output} onClick={onOpenFinalReport}>Open final report</Button>}
      />
    );
  }

  if (running || focusedStep) {
    const step = running || focusedStep!;
    return (
      <Alert
        showIcon
        type="info"
        message="Run in progress"
        description={`Currently focused on “${step.title}”. You can inspect live output, logs, and dependencies while the workflow runs.`}
        action={<Button onClick={() => onInspect(step)}>Inspect current step</Button>}
      />
    );
  }

  return (
    <Alert
      showIcon
      type="info"
      message="Waiting for scheduler"
      description="No step needs attention yet. The dashboard will update automatically when execution advances."
    />
  );
}
