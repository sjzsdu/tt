import { Button, Card, Empty, Flex, Segmented, Space, Tag, Timeline, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import type { FormulaDashboardLogEntry, FormulaDashboardStep } from '../../types';
import { activityShortId, formatDuration, statusIcon, statusLabel, statusTone } from '../../utils/status';
import { stepExecutionKind, stepExecutionLabel, stepExecutionTone } from '../../utils/steps';

type TimelineFilter = 'all' | 'errors' | 'linked';

type TimelineItem = {
  log: FormulaDashboardLogEntry;
  isError: boolean;
  step: FormulaDashboardStep | null;
};

function TimelineFilterLabel({ label, count }: { label: string; count: number }) {
  return (
    <span className="timeline-filter-label">
      <span className="timeline-filter-name">{label}</span>
      <span className="timeline-filter-count">{count}</span>
    </span>
  );
}

function matchLogStep(log: FormulaDashboardLogEntry, steps: FormulaDashboardStep[]) {
  const text = log.text.toLowerCase();
  return steps.find(step => text.includes(step.id.toLowerCase()))
    || steps.find(step => step.title && text.includes(step.title.toLowerCase()))
    || null;
}

function stepDuration(step: FormulaDashboardStep, now: number) {
  if (step.duration_ms) return step.duration_ms;
  if (step.status !== 'running' || !step.started_at) return 0;
  const started = Date.parse(step.started_at);
  if (!Number.isFinite(started)) return 0;
  return Math.max(0, now - started);
}

function TimelineStepMeta({ step, now }: { step: FormulaDashboardStep; now: number }) {
  const executionKind = stepExecutionKind(step);
  const duration = stepDuration(step, now);
  const latest = step.activities?.at(-1);

  return (
    <div className={`timeline-step-meta timeline-step-${executionKind}`}>
      <Flex align="center" gap={8} wrap="wrap" className="timeline-step-title-row">
        <Tag color={stepExecutionTone(executionKind)} className="timeline-kind-tag">
          {stepExecutionLabel(executionKind)}
        </Tag>
        <Typography.Text strong className="timeline-step-title">{step.title || step.id}</Typography.Text>
        <Typography.Text type="secondary" className="timeline-step-id">{activityShortId(step.id)}</Typography.Text>
      </Flex>
      <Flex gap={6} wrap="wrap" className="timeline-step-tags">
        <Tag color={statusTone[step.status] || 'default'} icon={statusIcon(step.status)}>{statusLabel(step.status)}</Tag>
        {duration ? <Tag>duration · {formatDuration(duration)}</Tag> : null}
        {step.agent ? <Tag>agent · {step.agent}</Tag> : null}
        {step.model ? <Tag>model · {step.model}</Tag> : null}
        {step.session ? <Tag color="geekblue">session · {step.session}</Tag> : null}
        {step.script_path ? <Tag color="volcano">script · {step.script_path}</Tag> : null}
        {latest?.detail ? <Tag className="timeline-detail-tag">{latest.detail}</Tag> : null}
      </Flex>
    </div>
  );
}

export function ExecutionTimeline({ logs, steps, onSelectStep }: { logs: FormulaDashboardLogEntry[]; steps: FormulaDashboardStep[]; onSelectStep: (step: FormulaDashboardStep) => void }) {
  const [filter, setFilter] = useState<TimelineFilter>('all');
  const [now, setNow] = useState(() => Date.now());
  const hasRunningStep = steps.some(step => step.status === 'running' && step.started_at);

  useEffect(() => {
    if (!hasRunningStep) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [hasRunningStep]);
  const items = useMemo<TimelineItem[]>(() => logs.slice(-40).map(log => ({
    log,
    isError: /fail|error|blocked/i.test(log.text),
    step: matchLogStep(log, steps),
  })), [logs, steps]);

  const counts = {
    all: items.length,
    errors: items.filter(item => item.isError).length,
    linked: items.filter(item => item.step).length,
  };

  const visible = items
    .filter(item => filter === 'all' || (filter === 'errors' && item.isError) || (filter === 'linked' && item.step))
    .slice(-14);

  return (
    <Card
      className="console-card"
      title="Execution timeline"
      extra={(
        <Segmented
          size="small"
          value={filter}
          onChange={value => setFilter(value as TimelineFilter)}
          options={[
            { value: 'all', label: <TimelineFilterLabel label="All" count={counts.all} /> },
            { value: 'errors', label: <TimelineFilterLabel label="Errors" count={counts.errors} /> },
            { value: 'linked', label: <TimelineFilterLabel label="Linked" count={counts.linked} /> },
          ]}
        />
      )}
    >
      {visible.length ? (
        <Timeline
          items={visible.map(({ log, isError, step }) => ({
            color: isError ? 'red' : step ? 'green' : 'blue',
            children: (
              <div className={isError ? 'timeline-entry timeline-entry-error' : 'timeline-entry'}>
                <Space size={8} wrap className="timeline-entry-header">
                  <Typography.Text type="secondary" className="timeline-time">{log.at}</Typography.Text>
                  {step && <Tag bordered={false}>{activityShortId(step.id)}</Tag>}
                </Space>
                <Typography.Paragraph className="timeline-text">{log.text}</Typography.Paragraph>
                {step && <TimelineStepMeta step={step} now={now} />}
                {step && <Button size="small" onClick={() => onSelectStep(step)}>Open step</Button>}
              </div>
            ),
          }))}
        />
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No matching timeline events" />}
    </Card>
  );
}
