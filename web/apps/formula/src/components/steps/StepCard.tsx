import type { KeyboardEvent } from 'react';
import { Card, Flex, Space, Tag, Typography } from 'antd';
import type { FormulaDashboardStep } from '../../types';
import { activityShortId, formatDuration, statusIcon, statusLabel, statusTone } from '../../utils/status';
import { loopActivitySummary, stepExecutionKind, stepExecutionLabel, stepExecutionTone } from '../../utils/steps';

export function StepCard({ step, onSelect }: { step: FormulaDashboardStep; onSelect: (step: FormulaDashboardStep) => void }) {
  const latest = step.activities?.at(-1);
  const loopSummary = loopActivitySummary(step);
  const executionKind = stepExecutionKind(step);
  const description = step.description || step.notes || 'No extra description for this step.';
  const openStep = () => onSelect(step);
  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    openStep();
  };

  return (
    <Card
      hoverable
      size="small"
      className={`step-card ${step.status}`}
      onClick={openStep}
      onKeyDown={handleKeyDown}
      role="button"
      tabIndex={0}
      aria-label={`Open step ${step.title}, status ${statusLabel(step.status)}`}
      title={(
        <Space direction="vertical" size={2} className="step-card-title-block">
          <Typography.Text type="secondary" className="step-card-kicker">{step.id}</Typography.Text>
          <Typography.Text strong className="step-card-title">{step.title}</Typography.Text>
        </Space>
      )}
      extra={<Tag color={statusTone[step.status] || 'default'} icon={statusIcon(step.status)}>{statusLabel(step.status)}</Tag>}
    >
      <Space direction="vertical" size="small" className="step-card-body">
        <Typography.Paragraph type="secondary" ellipsis={{ rows: 2 }} className="step-card-description">
          {description}
        </Typography.Paragraph>

        {loopSummary && <Tag color="purple" className="step-card-loop-summary">↻ {loopSummary}</Tag>}

        {latest && (
          <Typography.Text type="secondary" className={`step-activity-mini ${latest.status}`}>
            <span>{latest.at}</span>{activityShortId(latest.step_id)} · {latest.title || statusLabel(latest.status)}
          </Typography.Text>
        )}

        <Flex gap={6} wrap="wrap">
          <Tag color={stepExecutionTone(executionKind)}>{stepExecutionLabel(executionKind)}</Tag>
          {step.agent && <Tag>agent · {step.agent}</Tag>}
          {step.session && <Tag color="geekblue">session · {step.session}</Tag>}
          {step.model && <Tag>model · {step.model}</Tag>}
          {step.queued_at && <Tag>queued · {new Date(step.queued_at).toLocaleTimeString()}</Tag>}
          {step.started_at && <Tag>started · {new Date(step.started_at).toLocaleTimeString()}</Tag>}
          {step.finished_at && <Tag>finished · {new Date(step.finished_at).toLocaleTimeString()}</Tag>}
          {step.duration_ms ? <Tag>duration · {formatDuration(step.duration_ms)}</Tag> : null}
          {!!step.depends_on?.length && <Tag>deps · {step.depends_on.length}</Tag>}
          {step.loop && <Tag color="purple">loop · {step.loop.body?.length || 0} body</Tag>}
          {!!step.activities?.length && <Tag>activity · {step.activities.length}</Tag>}
          {step.output_key && <Tag>output · {step.output_key}</Tag>}
        </Flex>
      </Space>
    </Card>
  );
}
