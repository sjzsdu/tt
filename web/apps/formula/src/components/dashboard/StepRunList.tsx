import { Card, Empty, Flex, Progress, Tag, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import type { FormulaDashboardStep } from '../../types';
import { activityShortId, formatDuration, statusIcon, statusLabel, statusTone } from '../../utils/status';
import { stepExecutionKind, stepExecutionLabel, stepExecutionTone } from '../../utils/steps';

type StepRunRecord = {
  step: FormulaDashboardStep;
  status: string;
  durationMS: number;
  totalActivities: number;
  completedActivities: number;
  failedActivities: number;
  runningActivities: number;
  waitingActivities: number;
  latestDetail?: string;
  visible: boolean;
};

function runtimeDuration(step: FormulaDashboardStep, now: number) {
  if (step.duration_ms) return step.duration_ms;
  if ((step.status === 'running' || step.status === 'waiting_input') && step.started_at) {
    const started = Date.parse(step.started_at);
    if (Number.isFinite(started)) return Math.max(0, now - started);
  }
  return 0;
}

function aggregateLoopStatus(step: FormulaDashboardStep) {
  const activities = step.activities || [];
  if (!step.loop || activities.length === 0) return step.status;
  if (activities.some(activity => activity.status === 'failed')) return 'failed';
  if (activities.some(activity => activity.status === 'waiting_input')) return 'waiting_input';
  if (activities.some(activity => activity.status === 'running')) return 'running';
  const finished = activities.filter(activity => activity.status === 'completed' || activity.status === 'skipped').length;
  if (finished > 0 && finished === activities.length) return activities.every(activity => activity.status === 'skipped') ? 'skipped' : 'completed';
  return step.status;
}

function buildStepRunRecord(step: FormulaDashboardStep, now: number): StepRunRecord {
  const activities = step.activities || [];
  const status = aggregateLoopStatus(step);
  const completedActivities = activities.filter(activity => activity.status === 'completed' || activity.status === 'skipped').length;
  const latest = activities.at(-1);
  const visible = step.status !== 'pending' || activities.length > 0 || Boolean(step.started_at || step.finished_at);

  return {
    step,
    status,
    durationMS: runtimeDuration(step, now),
    totalActivities: activities.length,
    completedActivities,
    failedActivities: activities.filter(activity => activity.status === 'failed').length,
    runningActivities: activities.filter(activity => activity.status === 'running').length,
    waitingActivities: activities.filter(activity => activity.status === 'waiting_input').length,
    latestDetail: latest?.detail,
    visible,
  };
}

function recordOrder(record: StepRunRecord) {
  if (record.status === 'running') return 0;
  if (record.status === 'waiting_input') return 1;
  if (record.status === 'failed') return 2;
  return 3;
}

export function StepRunList({ steps, onSelectStep }: { steps: FormulaDashboardStep[]; onSelectStep: (step: FormulaDashboardStep) => void }) {
  const [now, setNow] = useState(() => Date.now());
  const hasLiveStep = steps.some(step => (step.status === 'running' || step.status === 'waiting_input') && step.started_at);

  useEffect(() => {
    if (!hasLiveStep) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [hasLiveStep]);

  const records = useMemo(() => steps
    .map(step => buildStepRunRecord(step, now))
    .filter(record => record.visible)
    .sort((a, b) => recordOrder(a) - recordOrder(b) || a.step.index - b.step.index), [steps, now]);

  return (
    <Card className="console-card step-run-card" title="Step runs" extra={<Tag>{records.length} loaded</Tag>}>
      {records.length ? (
        <div className="step-run-list">
          {records.map(record => {
            const { step } = record;
            const executionKind = stepExecutionKind(step);
            const progress = record.totalActivities ? Math.round((record.completedActivities / record.totalActivities) * 100) : undefined;
            return (
              <button key={step.id} type="button" className={`step-run-item ${record.status} step-run-${executionKind}`} onClick={() => onSelectStep(step)}>
                <Flex align="flex-start" justify="space-between" gap={8} className="step-run-head">
                  <div className="step-run-title-block">
                    <Typography.Text strong className="step-run-title">{step.title || step.id}</Typography.Text>
                    <Typography.Text type="secondary" className="step-run-id">{activityShortId(step.id)}</Typography.Text>
                  </div>
                  <Tag color={statusTone[record.status] || 'default'} icon={statusIcon(record.status)}>{statusLabel(record.status)}</Tag>
                </Flex>

                <Flex gap={6} wrap="wrap" className="step-run-tags">
                  <Tag color={stepExecutionTone(executionKind)}>{stepExecutionLabel(executionKind)}</Tag>
                  {record.durationMS ? <Tag>duration · {formatDuration(record.durationMS)}</Tag> : null}
                  {step.agent ? <Tag>agent · {step.agent}</Tag> : null}
                  {step.script_path ? <Tag color="volcano">script</Tag> : null}
                  {step.loop ? <Tag color="purple">loop · {record.completedActivities}/{record.totalActivities || step.loop.body?.length || 0}</Tag> : null}
                  {record.failedActivities ? <Tag color="error">failed · {record.failedActivities}</Tag> : null}
                  {record.runningActivities ? <Tag color="processing">running · {record.runningActivities}</Tag> : null}
                  {record.waitingActivities ? <Tag color="warning">input · {record.waitingActivities}</Tag> : null}
                </Flex>

                {typeof progress === 'number' && <Progress percent={progress} size="small" status={record.failedActivities ? 'exception' : record.status === 'running' ? 'active' : 'normal'} showInfo={false} />}
                {record.latestDetail && <Typography.Text type="secondary" className="step-run-detail">{record.latestDetail}</Typography.Text>}
              </button>
            );
          })}
        </div>
      ) : (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No step has started yet" />
      )}
    </Card>
  );
}
