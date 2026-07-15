import { Card, Empty, Flex, Segmented, Tag, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import type { FormulaDashboardSnapshot, FormulaDashboardStep, FormulaExecutionInstance } from '../../types';
import { formatDuration, statusIcon, statusLabel, statusTone } from '../../utils/status';
import {
  compareExecutionRecency,
  executionAddressLabel,
  executionInstances,
  executionInstanceStep,
  executionUpdatedAt,
  isActiveExecution,
  isTerminalExecution,
  iterationLabel,
} from '../../utils/execution';

type RunFilter = 'active' | 'recent' | 'all';

function runtimeDuration(instance: FormulaExecutionInstance, now: number) {
  if (instance.duration_ms) return instance.duration_ms;
  if (isActiveExecution(instance) && instance.started_at) {
    const started = Date.parse(instance.started_at);
    if (Number.isFinite(started)) return Math.max(0, now - started);
  }
  return 0;
}

function attentionOrder(instance: FormulaExecutionInstance) {
  if (instance.status === 'failed' || instance.status === 'interrupted') return 0;
  if (instance.status === 'waiting_input') return 1;
  if (instance.status === 'running') return 2;
  return 3;
}

function groupLabel(instance: FormulaExecutionInstance) {
  if (!instance.parent_loop_id) return 'Top-level steps';
  return `${instance.parent_loop_id} · ${iterationLabel(instance.iteration_path) || 'loop'}`;
}

export function StepRunList({ snapshot, onSelectStep }: { snapshot: FormulaDashboardSnapshot; onSelectStep: (step: FormulaDashboardStep) => void }) {
  const [filter, setFilter] = useState<RunFilter>('active');
  const [now, setNow] = useState(() => Date.now());
  const instances = useMemo(() => {
    const all = executionInstances(snapshot);
    const loopsWithChildren = new Set(all.map(instance => instance.parent_loop_id).filter(Boolean));
    return all.filter(instance => !(
      loopsWithChildren.has(instance.address)
      && snapshot.steps.some(step => step.id === instance.address && !!step.loop)
    ));
  }, [snapshot]);
  const active = useMemo(() => instances.filter(isActiveExecution), [instances]);
  const recent = useMemo(() => instances.filter(isTerminalExecution).sort(compareExecutionRecency).slice(0, 30), [instances]);

  useEffect(() => {
    if (!active.some(instance => instance.started_at)) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);

  useEffect(() => {
    if (!active.length && filter === 'active') setFilter('recent');
  }, [active.length, filter]);

  const visible = useMemo(() => {
    const source = filter === 'active' ? active : filter === 'recent' ? recent : [...active, ...recent];
    return [...source].sort((a, b) => attentionOrder(a) - attentionOrder(b) || compareExecutionRecency(a, b));
  }, [active, filter, recent]);

  const groups = useMemo(() => {
    const grouped = new Map<string, FormulaExecutionInstance[]>();
    for (const instance of visible) {
      const label = groupLabel(instance);
      if (!grouped.has(label)) grouped.set(label, []);
      grouped.get(label)!.push(instance);
    }
    return [...grouped.entries()];
  }, [visible]);

  return (
    <Card
      className="console-card step-run-card"
      title={<Flex align="center" gap={8}><span>Step runs</span>{active.length ? <Tag color="processing">{active.length} live</Tag> : null}</Flex>}
      extra={(
        <Segmented
          size="small"
          value={filter}
          onChange={value => setFilter(value as RunFilter)}
          options={[
            { value: 'active', label: `Active ${active.length}` },
            { value: 'recent', label: `Recent ${recent.length}` },
            { value: 'all', label: 'All' },
          ]}
        />
      )}
    >
      {groups.length ? (
        <div className="step-run-groups">
          {groups.map(([label, records]) => (
            <section className="step-run-group" key={label}>
              <div className="step-run-group-title">
                <Typography.Text strong>{label}</Typography.Text>
                <Tag>{records.length}</Tag>
              </div>
              <div className="step-run-list">
                {records.map(instance => {
                  const duration = runtimeDuration(instance, now);
                  return (
                    <button
                      key={instance.address}
                      type="button"
                      className={`step-run-item ${instance.status} ${isActiveExecution(instance) ? 'step-run-live' : ''}`}
                      onClick={() => onSelectStep(executionInstanceStep(instance, snapshot))}
                    >
                      <Flex align="flex-start" justify="space-between" gap={8} className="step-run-head">
                        <div className="step-run-title-block">
                          <Typography.Text strong className="step-run-title">{instance.title || instance.body_step_id || instance.definition_step_id}</Typography.Text>
                          <Typography.Text type="secondary" className="step-run-id">{executionAddressLabel(instance)}</Typography.Text>
                        </div>
                        <Tag color={statusTone[instance.status] || 'default'} icon={statusIcon(instance.status)}>{statusLabel(instance.status)}</Tag>
                      </Flex>
                      <Flex gap={6} wrap="wrap" className="step-run-tags">
                        {duration ? <Tag>duration · {formatDuration(duration)}</Tag> : null}
                        {(instance.attempt || 0) > 1 ? <Tag color="purple">attempt · {instance.attempt}</Tag> : null}
                        {instance.session ? <Tag color="geekblue">session</Tag> : null}
                        {executionUpdatedAt(instance) ? <Tag>{executionUpdatedAt(instance)}</Tag> : null}
                      </Flex>
                      {instance.detail && <Typography.Text type="secondary" className="step-run-detail">{instance.detail}</Typography.Text>}
                    </button>
                  );
                })}
              </div>
            </section>
          ))}
        </div>
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={filter === 'active' ? 'No execution instance is active' : 'No finished execution instances'} />}
    </Card>
  );
}
