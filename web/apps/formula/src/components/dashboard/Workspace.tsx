import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Button, Col, Empty, Input, Row, Segmented, Space, Spin, Typography, type InputRef } from 'antd';
import type { DashboardView, FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { StepCard } from '../steps/StepCard';
import { ExecutionTimeline } from './ExecutionTimeline';

const GraphPanel = lazy(() => import('../graph/GraphPanel').then(module => ({ default: module.GraphPanel })));

type StepFilter = 'all' | 'attention' | 'running' | 'waiting_input' | 'failed' | 'completed';

function matchesStepQuery(step: FormulaDashboardStep, query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return [
    step.id,
    step.title,
    step.description,
    step.notes,
    step.agent,
    step.model,
    step.status,
    step.output_key,
    ...(step.labels || []),
    ...(step.depends_on || []),
  ].some(value => String(value || '').toLowerCase().includes(needle));
}

function matchesStatusFilter(step: FormulaDashboardStep, filter: StepFilter) {
  if (filter === 'all') return true;
  if (filter === 'attention') return step.status === 'failed' || step.status === 'waiting_input' || step.status === 'running';
  return step.status === filter;
}

export function Workspace({
  view,
  snapshot,
  orderedSteps,
  theme,
  onSelectStep,
}: {
  view: DashboardView;
  snapshot: FormulaDashboardSnapshot;
  orderedSteps: FormulaDashboardStep[];
  theme: 'light' | 'dark';
  onSelectStep: (step: FormulaDashboardStep) => void;
}) {
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<StepFilter>('all');
  const searchRef = useRef<InputRef>(null);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const isTyping = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable;
      if (event.key === '/' && !isTyping && view === 'list') {
        event.preventDefault();
        searchRef.current?.focus();
      }
      if (event.key === 'Escape' && view === 'list' && (query || filter !== 'all')) {
        setQuery('');
        setFilter('all');
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [filter, query, view]);

  const counts = useMemo(() => ({
    all: orderedSteps.length,
    attention: orderedSteps.filter(step => matchesStatusFilter(step, 'attention')).length,
    running: orderedSteps.filter(step => step.status === 'running').length,
    waiting_input: orderedSteps.filter(step => step.status === 'waiting_input').length,
    failed: orderedSteps.filter(step => step.status === 'failed').length,
    completed: orderedSteps.filter(step => step.status === 'completed').length,
  }), [orderedSteps]);

  const visibleSteps = useMemo(() => orderedSteps
    .filter(step => matchesStatusFilter(step, filter))
    .filter(step => matchesStepQuery(step, query)), [orderedSteps, filter, query]);

  const clearFilters = () => {
    setQuery('');
    setFilter('all');
  };

  return (
    <Row gutter={[18, 18]} align="stretch">
      <Col xs={24} xl={17}>
        <div className="workspace-main">
          {view === 'list' ? (
            <Space direction="vertical" size="middle" className="step-list-workbench">
              <div className="step-list-toolbar">
                <div>
                  <Typography.Text strong>Steps</Typography.Text>
                  <Typography.Text type="secondary"> {visibleSteps.length} of {orderedSteps.length}</Typography.Text>
                </div>
                <Input.Search
                  ref={searchRef}
                  allowClear
                  placeholder="Search id, title, agent, label, dependency…  Press /"
                  aria-label="Search formula steps"
                  value={query}
                  onChange={event => setQuery(event.target.value)}
                  className="step-search-input"
                />
              </div>
              <Segmented
                aria-label="Filter formula steps by status"
                value={filter}
                onChange={value => setFilter(value as StepFilter)}
                options={[
                  { value: 'all', label: <Badge size="small" count={counts.all} overflowCount={99}>All</Badge> },
                  { value: 'attention', label: <Badge size="small" count={counts.attention}>Attention</Badge> },
                  { value: 'running', label: <Badge size="small" count={counts.running}>Running</Badge> },
                  { value: 'waiting_input', label: <Badge size="small" count={counts.waiting_input}>Input</Badge> },
                  { value: 'failed', label: <Badge size="small" count={counts.failed}>Failed</Badge> },
                  { value: 'completed', label: <Badge size="small" count={counts.completed}>Done</Badge> },
                ]}
              />
              {visibleSteps.length ? (
                <Row gutter={[14, 14]}>
                  {visibleSteps.map(step => (
                    <Col key={step.id} xs={24} lg={12} xxl={8}>
                      <StepCard step={step} onSelect={onSelectStep} />
                    </Col>
                  ))}
                </Row>
              ) : (
                <Empty description="No steps match the current filters">
                  <Button onClick={clearFilters}>Clear filters</Button>
                </Empty>
              )}
            </Space>
          ) : (
            <Suspense fallback={<div className="graph-canvas empty-screen"><Spin tip="Loading graph…" /></div>}>
              <GraphPanel snapshot={snapshot} onSelect={onSelectStep} theme={theme} />
            </Suspense>
          )}
        </div>
      </Col>
      <Col xs={24} xl={7}>
        <ExecutionTimeline logs={snapshot.logs} steps={snapshot.steps} onSelectStep={onSelectStep} />
      </Col>
    </Row>
  );
}
