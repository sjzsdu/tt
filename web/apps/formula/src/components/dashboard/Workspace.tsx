import { Col, Row } from 'antd';
import type { DashboardView, FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { GraphPanel } from '../graph/GraphPanel';
import { StepCard } from '../steps/StepCard';
import { ExecutionTimeline } from './ExecutionTimeline';

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
            <GraphPanel snapshot={snapshot} onSelect={onSelectStep} theme={theme} />
          )}
        </div>
      </Col>
      <Col xs={24} xl={7}>
        <ExecutionTimeline logs={snapshot.logs} />
      </Col>
    </Row>
  );
}
