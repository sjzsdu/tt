import { Card, Col, Progress, Row, Statistic, Typography } from 'antd';
import type { FormulaDashboardStep } from '../../types';

export type RunSummary = {
  steps: number;
  completed: number;
  skipped: number;
  running: number;
  failed: number;
  logs: number;
  repairs: number;
};

export function RunOverview({ summary, progress, runningStep, status }: { summary: RunSummary; progress: number; runningStep?: FormulaDashboardStep; status: string }) {
  return (
    <Card className="run-overview-card">
      <Row gutter={[24, 18]} align="middle">
        <Col xs={24} lg={10}>
          <Typography.Text type="secondary" className="dashboard-kicker">Run progress</Typography.Text>
          <Typography.Title level={3} className="overview-title">{progress}% complete</Typography.Title>
          <Progress percent={progress} showInfo={false} />
          <Typography.Text type="secondary">
            {runningStep ? `Current: ${runningStep.title}` : status === 'completed' ? 'Run finished.' : 'Waiting for the next executable step.'}
          </Typography.Text>
        </Col>
        <Col xs={24} lg={14}>
          <Row gutter={[12, 12]}>
            <Col xs={12} sm={8} xl={4}><Statistic title="Total" value={summary.steps} /></Col>
            <Col xs={12} sm={8} xl={4}><Statistic title="Done" value={summary.completed} /></Col>
            <Col xs={12} sm={8} xl={4}><Statistic title="Running" value={summary.running} /></Col>
            <Col xs={12} sm={8} xl={4}><Statistic title="Skipped" value={summary.skipped} /></Col>
            <Col xs={12} sm={8} xl={4}><Statistic title="Failed" value={summary.failed} valueStyle={summary.failed ? { color: 'var(--ant-color-error)' } : undefined} /></Col>
            <Col xs={12} sm={8} xl={4}><Statistic title="Repairs" value={summary.repairs} /></Col>
            <Col xs={12} sm={8} xl={4}><Statistic title="Logs" value={summary.logs} /></Col>
          </Row>
        </Col>
      </Row>
    </Card>
  );
}
