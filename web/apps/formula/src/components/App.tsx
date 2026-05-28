import { useCallback, useEffect, useState } from 'react';
import { App as AntdApp, Button, Card, Empty, Progress, Segmented, Tag, Timeline } from 'antd';
import { ApartmentOutlined, CopyOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { api } from '../api';
import type { DashboardView, FormulaDashboardStep } from '../types';
import { useFormulaDashboard } from '../hooks/useFormulaDashboard';
import { currentView, persistDashboardView } from '../store/dashboardView';
import { attentionCopy, attentionStep } from '../utils/steps';
import { statusIcon, statusLabel, statusTone } from '../utils/status';
import { GraphPanel } from './graph/GraphPanel';
import { OutputModal } from './MarkdownOutput';
import { HumanInputModal } from './modals/HumanInputModal';
import { RetryStepModal } from './modals/RetryStepModal';
import { StepCard } from './steps/StepCard';
import { StepInspector } from './steps/StepInspector';

export function App() {
  const { message } = AntdApp.useApp();
  const [view, setView] = useState<DashboardView>(currentView());
  const [selectedStep, setSelectedStep] = useState<FormulaDashboardStep | null>(null);
  const [retryStep, setRetryStep] = useState<FormulaDashboardStep | null>(null);
  const [finalOutputOpen, setFinalOutputOpen] = useState(false);

  const handleLoadError = useCallback((err: unknown) => {
    message.error(`Failed to load formula dashboard: ${String(err)}`);
  }, [message]);

  const { snapshot, error, summary, orderedSteps } = useFormulaDashboard(handleLoadError);

  useEffect(() => {
    if (!snapshot) return;
    setSelectedStep(current => {
      if (!current) return current;
      return snapshot.steps.find(step => step.id === current.id) || current;
    });
  }, [snapshot]);

  const progress = summary?.steps ? Math.round(((summary.completed + summary.skipped) / summary.steps) * 100) : 0;
  const runningStep = snapshot?.steps.find(step => step.status === 'running');
  const waitingInputStep = snapshot?.steps.find(step => step.status === 'waiting_input' && step.human_input_request);
  const focusedStep = snapshot ? attentionStep(snapshot) : null;
  const focusCopy = snapshot ? attentionCopy(focusedStep, snapshot.status) : null;

  const setDashboardView = (next: DashboardView) => {
    persistDashboardView(next);
    setView(next);
  };

  const submitHumanInput = async (stepID: string, values: Record<string, unknown>) => {
    try {
      await api.submitHumanInput(stepID, values);
      message.success('Human input submitted. Resuming workflow…');
    } catch (err) {
      message.error(`Human input submit failed: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  const submitRetryStep = async (stepID: string, advice?: string) => {
    try {
      await api.retryStep(stepID, advice);
      setRetryStep(null);
      setSelectedStep(null);
      message.success('Step restarted. Workflow is resuming…');
    } catch (err) {
      message.error(`Step restart failed: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  const copyRunID = async () => {
    const runID = snapshot?.run_id;
    if (!runID) return;
    await navigator.clipboard.writeText(runID);
    message.success('Run ID copied');
  };

  if (error && !snapshot) {
    return <main className="formula-app empty-screen"><Empty description={error} /></main>;
  }
  if (!snapshot || !summary) {
    return <main className="formula-app empty-screen"><Empty description="Loading formula dashboard…" /></main>;
  }

  return (
    <main className="formula-app">
      <section className="hero-panel">
        <div>
          <div className="hero-kicker">tt formula dashboard</div>
          <div className="hero-title-row">
            <h1>{snapshot.recipe_name}</h1>
            <Tag color={statusTone[snapshot.status] || 'processing'} icon={statusIcon(snapshot.status)}>{statusLabel(snapshot.status)}</Tag>
          </div>
          {snapshot.run_id && (
            <button type="button" className="run-id-copy" onClick={copyRunID} title="Copy run id">
              <span>Run ID</span>
              <code>{snapshot.run_id}</code>
              <CopyOutlined />
            </button>
          )}
          <p>{snapshot.description || 'Live execution control room for formula runs.'}</p>
        </div>
        <div className="hero-actions">
          <Segmented
            value={view}
            onChange={value => setDashboardView(value as DashboardView)}
            options={[
              { label: 'List', value: 'list', icon: <UnorderedListOutlined /> },
              { label: 'Graph', value: 'graph', icon: <ApartmentOutlined /> },
            ]}
          />
          {snapshot.final_output ? (
            <Button type="primary" onClick={() => setFinalOutputOpen(true)}>Open final report</Button>
          ) : (
            <Button disabled>Waiting for final report</Button>
          )}
        </div>
      </section>

      {focusCopy && (
        <section className={`run-attention-strip ${focusCopy.tone}`}>
          <div>
            <span className="attention-label">Next focus</span>
            <strong>{focusCopy.title}</strong>
            <p>{focusCopy.detail}</p>
          </div>
          <div className="attention-actions">
            {focusedStep && <Button type="primary" onClick={() => setSelectedStep(focusedStep)}>Inspect step</Button>}
            {focusedStep?.status === 'failed' && <Button danger onClick={() => setRetryStep(focusedStep)}>Retry</Button>}
            {snapshot.final_output && <Button onClick={() => setFinalOutputOpen(true)}>Final report</Button>}
          </div>
        </section>
      )}

      <section className="run-overview-panel">
        <div className="run-overview-main">
          <div className="overview-kicker">Run progress</div>
          <strong>{progress}% complete</strong>
          <Progress percent={progress} showInfo={false} strokeColor={{ '0%': '#67e8f9', '100%': '#66e3c4' }} trailColor="rgba(148, 163, 184, 0.16)" />
          <p>{runningStep ? `Current: ${runningStep.title}` : snapshot.status === 'completed' ? 'Run finished.' : 'Waiting for the next executable step.'}</p>
        </div>
        <div className="overview-metrics">
          <div><span>Total</span><strong>{summary.steps}</strong></div>
          <div><span>Done</span><strong>{summary.completed}</strong></div>
          <div><span>Running</span><strong>{summary.running}</strong></div>
          <div><span>Skipped</span><strong>{summary.skipped}</strong></div>
          <div className={summary.failed ? 'danger' : ''}><span>Failed</span><strong>{summary.failed}</strong></div>
          <div><span>Logs</span><strong>{summary.logs}</strong></div>
        </div>
      </section>

      <section className="workspace-grid">
        <div className="workspace-main">
          {view === 'list' ? (
            <div className="step-grid">
              {orderedSteps.map(step => <StepCard key={step.id} step={step} onSelect={setSelectedStep} />)}
            </div>
          ) : (
            <GraphPanel snapshot={snapshot} onSelect={setSelectedStep} />
          )}
        </div>

        <aside className="workspace-side">
          <Card className="console-card" title="Execution timeline">
            {snapshot.logs.length ? (
              <Timeline
                items={snapshot.logs.slice(-14).map(log => ({
                  children: (
                    <div>
                      <div className="timeline-time">{log.at}</div>
                      <div className="timeline-text">{log.text}</div>
                    </div>
                  ),
                }))}
              />
            ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No timeline events yet" />}
          </Card>
        </aside>
      </section>

      <StepInspector step={selectedStep} snapshot={snapshot} open={!!selectedStep} onClose={() => setSelectedStep(null)} onRetry={setRetryStep} />
      <RetryStepModal step={retryStep} open={!!retryStep} onCancel={() => setRetryStep(null)} onSubmit={submitRetryStep} />
      <HumanInputModal step={waitingInputStep} onSubmit={submitHumanInput} />
      {snapshot.final_output ? (
        <OutputModal
          open={finalOutputOpen}
          onClose={() => setFinalOutputOpen(false)}
          title="Final output"
          content={snapshot.final_output}
          className="final-output-modal"
        />
      ) : null}
    </main>
  );
}
