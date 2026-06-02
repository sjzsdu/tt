import { useCallback, useEffect, useState } from 'react';
import { App as AntdApp, Empty, Layout, Space } from 'antd';
import { api } from '../api';
import type { DashboardView, FormulaDashboardStep } from '../types';
import { useFormulaDashboard } from '../hooks/useFormulaDashboard';
import { currentView, persistDashboardView } from '../store/dashboardView';
import { attentionStep } from '../utils/steps';
import { DashboardHeader } from './dashboard/DashboardHeader';
import { RunActionAlert } from './dashboard/RunActionAlert';
import { RunOverview } from './dashboard/RunOverview';
import { Workspace } from './dashboard/Workspace';
import { FinalReportModal } from './MarkdownOutput';
import { HumanInputModal } from './modals/HumanInputModal';
import { RetryStepModal } from './modals/RetryStepModal';
import { StepInspector } from './steps/StepInspector';

export function App({ theme, onThemeChange }: { theme: 'light' | 'dark'; onThemeChange: (theme: 'light' | 'dark') => void }) {
  const { message } = AntdApp.useApp();
  const [view, setView] = useState<DashboardView>(currentView());
  const [selectedStep, setSelectedStep] = useState<FormulaDashboardStep | null>(null);
  const [retryStep, setRetryStep] = useState<FormulaDashboardStep | null>(null);
  const [finalOutputOpen, setFinalOutputOpen] = useState(false);
  const [finalReportChatBusy, setFinalReportChatBusy] = useState(false);
  const [finalReportChatError, setFinalReportChatError] = useState('');

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

  const ensureFinalReportChat = async () => {
    setFinalReportChatError('');
    setFinalReportChatBusy(true);
    try {
      await api.ensureFinalReportChat();
    } catch (err) {
      const next = err instanceof Error ? err.message : String(err);
      setFinalReportChatError(next);
      message.error(`Final report chat failed: ${next}`);
      throw err;
    } finally {
      setFinalReportChatBusy(false);
    }
  };

  const sendFinalReportChatMessage = async (content: string) => {
    setFinalReportChatError('');
    setFinalReportChatBusy(true);
    try {
      if (!snapshot?.final_report_chat?.session_id) {
        await api.ensureFinalReportChat();
      }
      await api.sendFinalReportChatMessage(content);
    } catch (err) {
      const next = err instanceof Error ? err.message : String(err);
      setFinalReportChatError(next);
      message.error(`Final report chat failed: ${next}`);
    } finally {
      setFinalReportChatBusy(false);
    }
  };

  const promoteFinalReportChatResponse = async () => {
    setFinalReportChatError('');
    setFinalReportChatBusy(true);
    try {
      await api.promoteFinalReportChatResponse();
      message.success('Final report updated from latest chat response');
    } catch (err) {
      const next = err instanceof Error ? err.message : String(err);
      setFinalReportChatError(next);
      message.error(`Final report update failed: ${next}`);
    } finally {
      setFinalReportChatBusy(false);
    }
  };

  if (error && !snapshot) {
    return <main className="formula-app empty-screen"><Empty description={error} /></main>;
  }
  if (!snapshot || !summary) {
    return <main className="formula-app empty-screen"><Empty description="Loading formula dashboard…" /></main>;
  }

  const progress = summary.steps ? Math.round(((summary.completed + summary.skipped) / summary.steps) * 100) : 0;
  const runningStep = snapshot.steps.find(step => step.status === 'running');
  const waitingInputStep = snapshot.steps.find(step => step.status === 'waiting_input' && step.human_input_request);
  const focusedStep = attentionStep(snapshot);

  return (
    <Layout className="formula-app">
      <Layout.Content className="formula-content">
        <Space direction="vertical" size="middle" className="formula-stack">
          <DashboardHeader
            snapshot={snapshot}
            view={view}
            theme={theme}
            onViewChange={setDashboardView}
            onThemeChange={onThemeChange}
            onCopyRunID={copyRunID}
            onOpenFinalReport={() => setFinalOutputOpen(true)}
          />
          <RunActionAlert
            snapshot={snapshot}
            focusedStep={focusedStep}
            onInspect={setSelectedStep}
            onRetry={setRetryStep}
            onOpenFinalReport={() => setFinalOutputOpen(true)}
          />
          <RunOverview summary={summary} progress={progress} runningStep={runningStep} status={snapshot.status} />
          <Workspace view={view} snapshot={snapshot} orderedSteps={orderedSteps} theme={theme} onSelectStep={setSelectedStep} />
        </Space>

        <StepInspector step={selectedStep} snapshot={snapshot} open={!!selectedStep} onClose={() => setSelectedStep(null)} onRetry={setRetryStep} />
        <RetryStepModal step={retryStep} open={!!retryStep} onCancel={() => setRetryStep(null)} onSubmit={submitRetryStep} />
        <HumanInputModal step={waitingInputStep} onSubmit={submitHumanInput} />
        {snapshot.final_output ? (
          <FinalReportModal
            open={finalOutputOpen}
            onClose={() => setFinalOutputOpen(false)}
            title="Final output"
            content={snapshot.final_output}
            className="final-output-modal"
            chat={snapshot.final_report_chat}
            chatBusy={finalReportChatBusy || snapshot.final_report_chat?.status === 'running'}
            chatError={finalReportChatError || snapshot.final_report_chat?.error || ''}
            onStartChat={ensureFinalReportChat}
            onSendMessage={sendFinalReportChatMessage}
            onPromoteLatest={promoteFinalReportChatResponse}
          />
        ) : null}
      </Layout.Content>
    </Layout>
  );
}
