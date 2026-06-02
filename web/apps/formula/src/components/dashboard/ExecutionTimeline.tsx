import { Button, Card, Empty, Segmented, Timeline, Typography } from 'antd';
import { useMemo, useState } from 'react';
import type { FormulaDashboardLogEntry, FormulaDashboardStep } from '../../types';

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

export function ExecutionTimeline({ logs, steps, onSelectStep }: { logs: FormulaDashboardLogEntry[]; steps: FormulaDashboardStep[]; onSelectStep: (step: FormulaDashboardStep) => void }) {
  const [filter, setFilter] = useState<TimelineFilter>('all');
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
                <Typography.Text type="secondary" className="timeline-time">{log.at}</Typography.Text>
                <Typography.Paragraph className="timeline-text">{log.text}</Typography.Paragraph>
                {step && <Button size="small" onClick={() => onSelectStep(step)}>Open step</Button>}
              </div>
            ),
          }))}
        />
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No matching timeline events" />}
    </Card>
  );
}
