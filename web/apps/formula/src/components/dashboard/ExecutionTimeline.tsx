import { Button, Card, Empty, Flex, Segmented, Space, Tag, Timeline, Typography } from 'antd';
import { SwapOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useState } from 'react';
import type { FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { formatDuration, statusIcon, statusLabel, statusTone } from '../../utils/status';
import { executionAddressLabel, executionEvents, executionFormulaLabel, executionInstances, executionInstanceStep, isActiveExecution } from '../../utils/execution';

type TimelineFilter = 'current' | 'errors' | 'completed' | 'all';

function displayTime(value: string) {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return value;
  return new Date(parsed).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function ExecutionTimeline({ snapshot, onSelectStep }: { snapshot: FormulaDashboardSnapshot; onSelectStep: (step: FormulaDashboardStep) => void }) {
  const [filter, setFilter] = useState<TimelineFilter>('current');
  const [newestFirst, setNewestFirst] = useState(true);
  const instances = useMemo(() => executionInstances(snapshot), [snapshot]);
  const instanceByAddress = useMemo(() => new Map(instances.map(instance => [instance.address, instance])), [instances]);
  const activeAddresses = useMemo(() => new Set(instances.filter(isActiveExecution).map(instance => instance.address)), [instances]);
  const events = useMemo(() => executionEvents(snapshot), [snapshot]);

  useEffect(() => {
    if (!activeAddresses.size && filter === 'current') setFilter('all');
  }, [activeAddresses.size, filter]);

  const counts = {
    current: events.filter(event => !!event.instance_address && activeAddresses.has(event.instance_address)).length,
    errors: events.filter(event => event.status === 'failed' || event.status === 'interrupted').length,
    completed: events.filter(event => event.status === 'completed' || event.status === 'skipped').length,
    all: events.length,
  };

  const visible = useMemo(() => {
    const filtered = events.filter(event => {
      if (filter === 'current') return !!event.instance_address && activeAddresses.has(event.instance_address);
      if (filter === 'errors') return event.status === 'failed' || event.status === 'interrupted';
      if (filter === 'completed') return event.status === 'completed' || event.status === 'skipped';
      return true;
    }).slice(-100);
    return newestFirst ? filtered.reverse().slice(0, 24) : filtered.slice(-24);
  }, [activeAddresses, events, filter, newestFirst]);

  return (
    <Card
      className="console-card execution-timeline-card"
      title="Execution timeline"
      extra={(
        <Flex gap={6} wrap="wrap" justify="flex-end">
          <Segmented
            size="small"
            value={filter}
            onChange={value => setFilter(value as TimelineFilter)}
            options={[
              { value: 'current', label: `Current ${counts.current}` },
              { value: 'errors', label: `Errors ${counts.errors}` },
              { value: 'completed', label: `Done ${counts.completed}` },
              { value: 'all', label: `All ${counts.all}` },
            ]}
          />
          <Button size="small" icon={<SwapOutlined />} onClick={() => setNewestFirst(value => !value)}>{newestFirst ? 'Newest' : 'Oldest'}</Button>
        </Flex>
      )}
    >
      {visible.length ? (
        <Timeline
          items={visible.map(event => {
            const instance = event.instance_address ? instanceByAddress.get(event.instance_address) : undefined;
            const isError = event.status === 'failed' || event.status === 'interrupted';
            return {
              color: isError ? 'red' : event.status === 'running' ? 'blue' : event.status === 'completed' ? 'green' : 'gray',
              children: (
                <div className={isError ? 'timeline-entry timeline-entry-error' : 'timeline-entry'}>
                  <Space size={8} wrap className="timeline-entry-header">
                    <Typography.Text type="secondary" className="timeline-time">{displayTime(event.at)}</Typography.Text>
                    <Tag color={statusTone[event.status] || 'default'} icon={statusIcon(event.status)}>{statusLabel(event.status)}</Tag>
                    <Tag bordered={false}>{event.type}</Tag>
                  </Space>
					<Typography.Text strong className="timeline-address">{instance ? executionAddressLabel(instance) : event.instance_address || 'workflow'}</Typography.Text>
                  {event.title && <Typography.Text>{event.title}</Typography.Text>}
                  {event.detail && <Typography.Paragraph className="timeline-text">{event.detail}</Typography.Paragraph>}
                  <Flex gap={6} wrap="wrap">
                    {event.duration_ms ? <Tag>duration · {formatDuration(event.duration_ms)}</Tag> : null}
					{instance?.formula_path?.length ? <Tag color="purple">formula · {executionFormulaLabel(instance)}</Tag> : null}
                    {(event.attempt || 0) > 1 ? <Tag color="purple">attempt · {event.attempt}</Tag> : null}
                    {event.session ? <Tag color="geekblue">session</Tag> : null}
                    {event.from_status ? <Tag>{event.from_status} → {event.status}</Tag> : null}
                  </Flex>
                  {instance && <Button size="small" onClick={() => onSelectStep(executionInstanceStep(instance, snapshot))}>Open run</Button>}
                </div>
              ),
            };
          })}
        />
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No matching execution events" />}
    </Card>
  );
}
