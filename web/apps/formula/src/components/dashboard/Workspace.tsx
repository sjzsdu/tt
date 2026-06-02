import { lazy, Suspense } from 'react';
import { Col, Row } from 'antd';
import type { DashboardView, FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { StepCard } from '../steps/StepCard';
import { ExecutionTimeline } from './ExecutionTimeline';

const GraphPanel = lazy(() => import('../graph/GraphPanel').then(module => ({ default: module.GraphPanel })));

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
  return (
    <Row gutter={[18, 18]} align="stretch">
      <Col xs={24} xl={17}>
        <div className="workspace-main">
          {view === 'list' ? (
            <Row gutter={[14, 14]}>
              {orderedSteps.map(step => (
                <Col key={step.id} xs={24} lg={12} xxl={8}>
                  <StepCard step={step} onSelect={onSelectStep} />
                </Col>
              ))}
            </Row>
          ) : (
            <Suspense fallback={<div className="graph-canvas empty-screen">Loading graph…</div>}>
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
