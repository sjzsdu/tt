import { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Collapse, Descriptions, Drawer, Empty, Modal, Space, Tag, Typography, message } from 'antd';
import type { FormulaDashboardSnapshot, FormulaDashboardStep, FormulaStepActivity } from '../../types';
import { OutputSurface } from '../OutputSurface';
import { api, type AgentSessionTranscript } from '../../api';
import { activityShortId, formatDuration, statusLabel, statusTone } from '../../utils/status';
import { collectStepInputValues, groupLoopActivities, latestLoopActivity, loopActivityIteration, resolveLoopIterationInput, sameOutput, stepExecutionKind, stepExecutionLabel, stepExecutionTone } from '../../utils/steps';

export function StepInspector({ step, snapshot, open, onClose, onRetry }: { step: FormulaDashboardStep | null; snapshot: FormulaDashboardSnapshot | null; open: boolean; onClose: () => void; onRetry: (step: FormulaDashboardStep) => void }) {
  const [selectedSession, setSelectedSession] = useState<AgentSessionView | null>(null);
  const agentSessionViews = useMemo(() => step ? collectAgentSessionViews(step) : [], [step]);
  const primaryAgentSession = agentSessionViews[0];
  if (!step) return null;
  const executionKind = stepExecutionKind(step);
  const metadataEntries = Object.entries(step.metadata || {});
  const labels = step.labels || [];
  const inputCtx = step.input_ctx || [];
  const inputValues = collectStepInputValues(step, snapshot);
  const activities = step.activities || [];
  const loopBody = step.loop?.body || [];
  const loopActivityGroups = step.loop ? groupLoopActivities(activities) : [];
  const latestLoopIteration = step.loop ? loopActivityIteration(latestLoopActivity(step)) : '';
  const defaultOpenLoopActivityKey = latestLoopIteration || loopActivityGroups.at(-1)?.iteration || 'step';
  const hiddenDuplicateOutputs = activities.filter(activity => activity.output && sameOutput(activity.output, step.output)).length;
  const outputSummary = summarizeContent(step.output);
  const externalAgentOutput = parseExternalAgentOutput(step);
  const activityDefaultOpen = step.status === 'failed' || step.status === 'running' || step.status === 'waiting_input' ? ['activity'] : [];

  const statusSummary = (() => {
    if (step.status === 'failed') return { type: 'error' as const, message: 'Step failed', description: step.error || 'Inspect the activity log and retry this step with optional guidance.' };
    if (step.status === 'waiting_input') return { type: 'warning' as const, message: 'Input required', description: step.human_input_request?.reason || 'This step needs human input before the workflow can continue.' };
    if (step.status === 'completed') return { type: 'success' as const, message: 'Step completed', description: step.output ? 'Review the output below or open advanced runtime details.' : 'This step completed without a captured output.' };
    if (step.status === 'running') return { type: 'info' as const, message: 'Step running', description: 'Live execution is in progress. Activity and output will refresh as the run advances.' };
    return null;
  })();

  const sectionLabel = (icon: string, title: string, extra?: string) => (
    <div className="step-modal-section-header collapsible-section-label">
      <span className="step-modal-section-icon">{icon}</span>
      <h4>{title}</h4>
      {extra && <span className="collapsible-section-extra">{extra}</span>}
    </div>
  );

  const renderActivityRow = (activity: FormulaStepActivity) => {
    const activityOutputSummary = summarizeContent(activity.output);
    return (
    <div key={`${activity.step_id}-${activity.at}-${activity.status}`} className={`step-activity-row ${activity.status}`}>
      <div className="step-activity-status-dot" />
      <div className="step-activity-content">
        <div className="step-activity-head">
          <strong>{activity.title || activityShortId(activity.step_id)}</strong>
          <Tag color={statusTone[activity.status] || 'default'}>{statusLabel(activity.status)}</Tag>
        </div>
        <div className="step-activity-meta">{activity.at} · {activity.step_id}{activity.duration_ms ? ` · ${formatDuration(activity.duration_ms)}` : ''}</div>
        {activity.session && (
          <Button size="small" className="agent-session-inline-button" onClick={() => setSelectedSession({ session: activity.session!, agent: step.agent, stepID: activity.step_id, title: activity.title || activityShortId(activity.step_id), status: activity.status })}>
            View agent session
          </Button>
        )}
        {activity.detail && <p>{activity.detail}</p>}
        {activity.output && !sameOutput(activity.output, step.output) && (
          <Collapse
            className="step-activity-output-collapse"
            items={[{
              key: 'activity-output',
              label: sectionLabel('📄', 'Activity output', activityOutputSummary),
              children: <OutputSurface content={activity.output} className="step-activity-output" />,
            }]}
          />
        )}
        {activity.output && sameOutput(activity.output, step.output) && <div className="step-activity-output-note">Output matches final step output.</div>}
        {activity.error && <pre className="code-block error-block">{activity.error}</pre>}
      </div>
    </div>
    );
  };

  const loopActivityItems = loopActivityGroups.map(group => {
    const key = group.iteration || 'step';
    const iterationInput = resolveLoopIterationInput(step, snapshot, group.iteration);
    const statusCounts = group.activities.reduce<Record<string, number>>((acc, activity) => {
      acc[activity.status] = (acc[activity.status] || 0) + 1;
      return acc;
    }, {});
    const summary = Object.entries(statusCounts).map(([status, count]) => `${count} ${statusLabel(status)}`).join(' · ');
    const running = group.activities.some(activity => activity.status === 'running' || activity.status === 'waiting_input');
    return {
      key,
      label: (
        <div className="loop-activity-collapse-label">
          <strong>{group.iteration ? `Iteration ${group.iteration}` : 'Step activity'}</strong>
          <span>{summary || `${group.activities.length} event${group.activities.length === 1 ? '' : 's'}`}</span>
          {key === defaultOpenLoopActivityKey && <Tag color={running ? 'processing' : 'blue'}>{running ? 'current' : 'latest'}</Tag>}
        </div>
      ),
      children: (
        <div className="step-activity-list loop-activity-list">
          {iterationInput && (
            <Collapse
              className="step-iteration-input-collapse"
              items={[{
                key: 'iteration-input',
                label: sectionLabel('📥', `Iteration ${iterationInput.iteration} input`, `${iterationInput.key} · ${iterationInput.source}`),
                children: <OutputSurface content={iterationInput.value} className="step-input-output" />,
              }]}
            />
          )}
          {group.activities.map(renderActivityRow)}
        </div>
      ),
    };
  });

  const advancedItems = [
    {
      key: 'runtime',
      label: sectionLabel('⚙️', 'Advanced runtime', 'debug details'),
      children: (
        <Descriptions column={1} size="small" className="step-descriptions">
          <Descriptions.Item label="ID">{step.id}</Descriptions.Item>
          <Descriptions.Item label="Duration">{formatDuration(step.duration_ms)}</Descriptions.Item>
          <Descriptions.Item label="Session">{step.session || '—'}</Descriptions.Item>
          <Descriptions.Item label="Execution">{step.execution || '—'}</Descriptions.Item>
          <Descriptions.Item label="Condition">{step.condition || '—'}</Descriptions.Item>
        </Descriptions>
      ),
    },
    {
      key: 'dependencies',
      label: sectionLabel('🔗', 'Dependencies', step.depends_on?.length ? `${step.depends_on.length}` : 'none'),
      children: step.depends_on?.length ? <ul className="pill-list">{step.depends_on.map(dep => <li key={dep}>{dep}</li>)}</ul> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No dependencies" />,
    },
    {
      key: 'inputs-labels',
      label: sectionLabel('🏷️', 'Inputs & labels', `${inputCtx.length + labels.length}`),
      children: inputCtx.length || labels.length ? (
        <div className="advanced-stack">
          {!!inputCtx.length && <><div className="card-subtitle">Input context</div><ul className="pill-list">{inputCtx.map(key => <li key={key}>{key}</li>)}</ul></>}
          {!!labels.length && <><div className="card-subtitle">Labels</div><ul className="pill-list">{labels.map(label => <li key={label}>{label}</li>)}</ul></>}
        </div>
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No labels or inputs" />,
    },
    ...(metadataEntries.length ? [{
      key: 'metadata',
      label: sectionLabel('🧾', 'Metadata', `${metadataEntries.length}`),
      children: (
        <Descriptions column={1} size="small" className="step-descriptions">
          {metadataEntries.map(([key, value]) => <Descriptions.Item key={key} label={key}>{value}</Descriptions.Item>)}
        </Descriptions>
      ),
    }] : []),
  ];

  return (
    <Drawer open={open} onClose={onClose} width="96vw" title="Step Inspector" className="step-inspector" destroyOnHidden>
      <div className="inspector-title-block">
        <div>
          <div className="step-card-kicker">{step.id}</div>
          <h2>{step.title}</h2>
        </div>
        {step.status === 'failed' && <Button type="primary" danger onClick={() => onRetry(step)}>Retry this step</Button>}
        {primaryAgentSession && <Button type="primary" onClick={() => setSelectedSession(primaryAgentSession)}>View agent session</Button>}
      </div>

      <div className="step-modal-tagbar">
        <Tag color={statusTone[step.status] || 'default'}>{statusLabel(step.status)}</Tag>
        <Tag color={stepExecutionTone(executionKind)}>{stepExecutionLabel(executionKind)}</Tag>
        {step.agent && <Tag>{step.agent}</Tag>}
        {step.session && <Tag color="geekblue">session · {step.session}</Tag>}
        {step.model && <Tag>{step.model}</Tag>}
        {step.type && <Tag>{step.type}</Tag>}
        {step.priority && <Tag>P{step.priority}</Tag>}
      </div>

      {statusSummary && <Alert showIcon type={statusSummary.type} message={statusSummary.message} description={statusSummary.description} action={step.status === 'failed' ? <Button danger onClick={() => onRetry(step)}>Retry this step</Button> : undefined} className="step-status-summary" />}

      <div className="step-modal-main inspector-main-only">
        {(step.description || step.notes) && (
          <section className="step-modal-section">
            <div className="step-modal-section-header"><span className="step-modal-section-icon">📋</span><h4>Brief</h4></div>
            <div className="step-modal-section-body">
              {step.description && <p className="step-description-text">{step.description}</p>}
              {step.notes && <pre className="code-block">{step.notes}</pre>}
            </div>
          </section>
        )}

        {step.error && (
          <section className="step-modal-section step-critical-section">
            <div className="step-modal-section-header"><span className="step-modal-section-icon error-icon">⚠️</span><h4>Error</h4></div>
            <pre className="code-block error-block">{step.error}</pre>
          </section>
        )}

        {step.output && (
          <Collapse className="step-modal-collapse" items={[{ key: 'output', label: sectionLabel('📄', 'Output', outputSummary), children: <OutputSurface content={step.output} className="step-output-shell" /> }]} />
        )}

        {step.script_path && step.script_content && (
          <Collapse
            className="step-modal-collapse script-content-collapse"
            defaultActiveKey={['script-content']}
            items={[{
              key: 'script-content',
              label: sectionLabel('📜', 'Script file', step.script_path),
              children: (
                <div className="script-content-section">
                  <div className="script-file-path">
                    <Tag color="orange">External script</Tag>
                    <Typography.Text code>{step.script_path}</Typography.Text>
                  </div>
                  <pre className="code-block script-content-block">{step.script_content}</pre>
                </div>
              ),
            }]}
          />
        )}

        {externalAgentOutput && (
          <Collapse
            className="step-modal-collapse external-agent-details-collapse"
            items={[{
              key: 'external-agent-details',
              label: sectionLabel('🤖', 'External agent details', externalAgentOutput.driver || 'external'),
              children: <ExternalAgentDetails output={externalAgentOutput} />,
            }]}
          />
        )}

        <Collapse
          className="step-modal-collapse step-input-section"
          items={[{
            key: 'input',
            label: sectionLabel('📥', 'Input', inputValues.length ? `${inputValues.length} item${inputValues.length === 1 ? '' : 's'}` : 'empty'),
              children: inputValues.length > 0 ? (
                <Collapse className="step-input-collapse" items={inputValues.map(input => ({
                  key: input.key,
                  label: <div className="step-input-head input-collapse-label"><strong>{input.key}</strong><Tag>{input.source}</Tag><span className="collapsible-section-extra">{summarizeContent(input.value)}</span></div>,
                  children: <OutputSurface content={input.value} className="step-input-output" />,
                }))} />
            ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No resolved input data yet" />,
          }]}
        />

        {step.loop && (
          <Collapse
            className="step-modal-collapse loop-plan-section"
            items={[{
              key: 'loop-plan',
              label: sectionLabel('🔁', 'Loop plan', `${loopBody.length} body step${loopBody.length === 1 ? '' : 's'}`),
              children: (
                <div className="loop-plan-body">
                  <div className="loop-plan-summary"><strong>{step.loop.summary || 'Runtime loop'}</strong><span>{loopBody.length} planned body step{loopBody.length === 1 ? '' : 's'}</span></div>
                  <div className="loop-plan-grid">
                    {step.loop.until && <div><span>Until</span><code>{step.loop.until}</code></div>}
                    {!!step.loop.max && <div><span>Max</span><code>{step.loop.max}</code></div>}
                    {!!step.loop.count && <div><span>Count</span><code>{step.loop.count}</code></div>}
                    {step.loop.range && <div><span>Range</span><code>{step.loop.range}</code></div>}
                    {step.loop.for_each && <div><span>For each</span><code>{step.loop.for_each}</code></div>}
                    {step.loop.var && <div><span>Var</span><code>{step.loop.var}</code></div>}
                    {step.loop.parallel && <div><span>Mode</span><code>parallel</code></div>}
                    {!!step.loop.max_concurrency && <div><span>Max concurrency</span><code>{step.loop.max_concurrency}</code></div>}
                  </div>
                  {loopBody.length > 0 && <div className="loop-body-list">{loopBody.map((body, index) => (
                    <div className="loop-body-card" key={`${body.id}-${index}`}>
                      <div className="loop-body-index">#{index + 1}</div>
                      <div className="loop-body-main">
                        <div className="loop-body-head"><strong>{body.title || body.id}</strong><code>{body.id}</code></div>
                        {body.description && <p>{body.description}</p>}
                        <div className="loop-body-meta">
                          {(body.agent || body.model) && <span>agent · {[body.agent, body.model].filter(Boolean).join(' / ')}</span>}
                          {body.output_key && <span>output · {body.output_key}</span>}
                          {!!body.depends_on?.length && <span>depends · {body.depends_on.join(', ')}</span>}
                          {!!body.input_ctx?.length && <span>input · {body.input_ctx.join(', ')}</span>}
                          {body.condition && <span>if · {body.condition}</span>}
                        </div>
                      </div>
                    </div>
                  ))}</div>}
                </div>
              ),
            }]}
          />
        )}

        {activities.length > 0 && (
          <Collapse
            className="step-modal-collapse activity-timeline-collapse"
            defaultActiveKey={activityDefaultOpen}
            items={[{
              key: 'activity',
              label: sectionLabel('🛰️', 'Activity timeline', `${activities.length} event${activities.length === 1 ? '' : 's'}`),
              children: (
                <>
                  <p className="step-section-hint">{step.loop ? 'Internal loop body activity grouped by iteration. These are not separate top-level steps.' : 'Status events for this step. The final output is shown once in the Output section above.'}</p>
                  {step.loop && loopActivityItems.length ? <Collapse className="loop-activity-collapse" defaultActiveKey={[defaultOpenLoopActivityKey]} items={loopActivityItems} /> : <div className="step-activity-list">{activities.map(renderActivityRow)}</div>}
                  {hiddenDuplicateOutputs > 0 && <div className="step-section-footnote">Hidden duplicate output in {hiddenDuplicateOutputs} timeline event{hiddenDuplicateOutputs === 1 ? '' : 's'}.</div>}
                </>
              ),
            }]}
          />
        )}

        <Collapse className="step-modal-collapse advanced-step-collapse" items={advancedItems} />
      </div>
      <AgentSessionModal sessionView={selectedSession} snapshot={snapshot} onClose={() => setSelectedSession(null)} />
    </Drawer>
  );
}

type AgentSessionView = {
  session: string;
  agent?: string;
  stepID: string;
  title: string;
  status?: string;
};

type ExternalAgentOutput = {
  driver?: string;
  provider?: string;
  model?: string;
  mode?: string;
  session_id?: string;
  exit_code?: number;
  duration_ms?: number;
  stderr?: string;
  text?: string;
};

function parseExternalAgentOutput(step: FormulaDashboardStep): ExternalAgentOutput | null {
  if (stepExecutionKind(step) !== 'external_agent' || !step.output) return null;
  try {
    const parsed: unknown = JSON.parse(step.output);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
    const record = parsed as Record<string, unknown>;
    return {
      driver: stringValue(record.driver),
      provider: stringValue(record.provider),
      model: stringValue(record.model),
      mode: stringValue(record.mode),
      session_id: stringValue(record.session_id),
      exit_code: numberValue(record.exit_code),
      duration_ms: numberValue(record.duration_ms),
      stderr: stringValue(record.stderr),
      text: stringValue(record.text),
    };
  } catch {
    return null;
  }
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : undefined;
}

function numberValue(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function ExternalAgentDetails({ output }: { output: ExternalAgentOutput }) {
  return (
    <div className="advanced-stack external-agent-details">
      <Descriptions column={1} size="small" className="step-descriptions">
        <Descriptions.Item label="Driver">{output.driver || '—'}</Descriptions.Item>
        <Descriptions.Item label="Provider">{output.provider || '—'}</Descriptions.Item>
        <Descriptions.Item label="Model">{output.model || 'default'}</Descriptions.Item>
        <Descriptions.Item label="Mode">{output.mode || '—'}</Descriptions.Item>
        <Descriptions.Item label="Session ID">{output.session_id || '—'}</Descriptions.Item>
        <Descriptions.Item label="Exit code">{typeof output.exit_code === 'number' ? output.exit_code : '—'}</Descriptions.Item>
        <Descriptions.Item label="Duration">{formatDuration(output.duration_ms)}</Descriptions.Item>
      </Descriptions>
      {output.stderr && (
        <div>
          <div className="card-subtitle">stderr</div>
          <pre className="code-block error-block">{output.stderr}</pre>
        </div>
      )}
    </div>
  );
}

function collectAgentSessionViews(step: FormulaDashboardStep): AgentSessionView[] {
  const views: AgentSessionView[] = [];
  const seen = new Set<string>();
  const add = (view: AgentSessionView) => {
    const key = `${view.session}\u0000${view.stepID}`;
    if (!view.session || seen.has(key)) return;
    seen.add(key);
    views.push(view);
  };
  if (step.session) add({ session: step.session, agent: step.agent, stepID: step.id, title: step.title, status: step.status });
  for (const activity of step.activities || []) {
    if (!activity.session) continue;
    add({ session: activity.session, agent: step.agent, stepID: activity.step_id, title: activity.title || activityShortId(activity.step_id), status: activity.status });
  }
  return views;
}

function AgentSessionModal({ sessionView, snapshot, onClose }: { sessionView: AgentSessionView | null; snapshot: FormulaDashboardSnapshot | null; onClose: () => void }) {
  const [transcript, setTranscript] = useState<AgentSessionTranscript | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const sessionID = sessionView?.session || '';
  const sessionsDir = snapshot?.workspace_dir ? `${snapshot.workspace_dir}/.tt/sessions` : '.tt/sessions';

  useEffect(() => {
    if (!sessionView?.session) {
      setTranscript(null);
      setError('');
      return;
    }
    setLoading(true);
    setError('');
    api.agentSession(sessionView.session, sessionView.agent)
      .then(setTranscript)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }, [sessionView?.session, sessionView?.agent]);
  const copyText = async (text: string, label: string) => {
    await navigator.clipboard.writeText(text);
    message.success(`${label} copied`);
  };

  return (
    <Modal open={!!sessionView} title="Agent session" onCancel={onClose} footer={<Button onClick={onClose}>Close</Button>} width={720} destroyOnHidden>
      {sessionView ? (
        <Space direction="vertical" size="middle" className="agent-session-modal-body">
          <Alert
            type="info"
            showIcon
            message="Use this session id to inspect the long-running agent trace outside the formula output."
            description="The formula dashboard records the exact session used by this agent step, so you can audit prompts, tool calls, and responses while optimizing the formula."
          />
          <Descriptions column={1} size="small">
            <Descriptions.Item label="Step">{sessionView.stepID}</Descriptions.Item>
            <Descriptions.Item label="Title">{sessionView.title}</Descriptions.Item>
            <Descriptions.Item label="Status">{sessionView.status || '—'}</Descriptions.Item>
            <Descriptions.Item label="Session">
              <Typography.Text code copyable>{sessionID}</Typography.Text>
            </Descriptions.Item>
            <Descriptions.Item label="Agent">{sessionView.agent || '—'}</Descriptions.Item>
            <Descriptions.Item label="Workspace">{snapshot?.workspace_dir || '—'}</Descriptions.Item>
            <Descriptions.Item label="Transcript path">{transcript?.path || sessionsDir}</Descriptions.Item>
          </Descriptions>
          {loading && <Alert type="info" showIcon message="Loading transcript…" />}
          {(error || transcript?.missing) && <Alert type="warning" showIcon message="Transcript not found" description={error || transcript?.message || `No matching .jsonl file found under ${sessionsDir}`} />}
          {transcript?.content && (
            <div className="agent-session-transcript-section">
              <div className="card-subtitle">Transcript tail</div>
              <pre className="code-block session-command-block agent-session-transcript">{transcript.content}</pre>
            </div>
          )}
          <Space wrap>
            <Button onClick={() => copyText(sessionID, 'Session id')}>Copy session id</Button>
            {transcript?.content && <Button type="primary" onClick={() => copyText(transcript.content || '', 'Transcript')}>Copy transcript</Button>}
          </Space>
        </Space>
      ) : null}
    </Modal>
  );
}

function summarizeContent(content?: string) {
  const text = (content || '').trim();
  if (!text) return 'empty';
  const lineCount = text.split('\n').length;
  const charCount = text.length;
  const jsonKind = summarizeJSON(text);
  const prefix = jsonKind ? `${jsonKind} · ` : '';
  return `${prefix}${lineCount} line${lineCount === 1 ? '' : 's'} · ${charCount} char${charCount === 1 ? '' : 's'}`;
}

function summarizeJSON(content: string) {
  try {
    const parsed: unknown = JSON.parse(content);
    if (Array.isArray(parsed)) return `array(${parsed.length})`;
    if (parsed && typeof parsed === 'object') return `object(${Object.keys(parsed).length})`;
    return typeof parsed;
  } catch {
    return '';
  }
}
