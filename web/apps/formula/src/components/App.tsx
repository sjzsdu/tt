import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  Modal,
  Progress,
  Radio,
  Segmented,
  Select,
  Tag,
  Timeline,
} from 'antd';
import {
  ApartmentOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  ExpandOutlined,
  LoadingOutlined,
  PartitionOutlined,
  UnorderedListOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { api, normalizeSnapshot } from '../api';
import { ReactFlow, Background, Controls, Handle, MarkerType, MiniMap, Position, type Edge, type Node, type NodeProps } from '@xyflow/react';
import { MarkdownOutput, OutputModal, OutputSurface } from './MarkdownOutput';
import type {
  DashboardView,
  FormulaDashboardMessage,
  FormulaDashboardSnapshot,
  FormulaDashboardStep,
} from '../types';

const { TextArea } = Input;

const statusOrder: Record<string, number> = {
  running: 0,
  waiting_input: 1,
  failed: 2,
  pending: 3,
  skipped: 4,
  completed: 5,
};

const statusTone: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  waiting_input: 'warning',
  completed: 'success',
  failed: 'error',
  skipped: 'warning',
};

function currentView(): DashboardView {
  return localStorage.getItem('formula-dashboard-view') === 'graph' ? 'graph' : 'list';
}

function formatDuration(ms?: number) {
  if (!ms) return '—';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`;
}

function statusIcon(status: string) {
  switch (status) {
    case 'completed':
      return <CheckCircleOutlined />;
    case 'running':
      return <LoadingOutlined />;
    case 'waiting_input':
      return <ClockCircleOutlined />;
    case 'failed':
      return <WarningOutlined />;
    default:
      return <ClockCircleOutlined />;
  }
}

function graphShortId(id: string) {
  return id.includes('.') ? id.slice(id.lastIndexOf('.') + 1) : id;
}

function statusLabel(status: string) {
  return status.replace(/_/g, ' ');
}

function activityShortId(id: string) {
  const iter = id.match(/\.iter(\d+)\.([^.]*)$/);
  if (iter) return `iter ${iter[1]} · ${iter[2]}`;
  return graphShortId(id);
}

function loopActivitySummary(step: FormulaDashboardStep) {
  const activities = step.activities || [];
  const iterations = new Set<string>();
  for (const activity of activities) {
    const match = activity.step_id.match(/\.iter(\d+)\./);
    if (match) iterations.add(match[1]);
  }
  if (!step.loop && !activities.length) return '';
  const bodyCount = step.loop?.body?.length || 0;
  const bits = [];
  if (step.loop?.summary) bits.push(step.loop.summary);
  if (bodyCount) bits.push(`${bodyCount} body step${bodyCount === 1 ? '' : 's'}`);
  if (iterations.size) bits.push(`${iterations.size} iteration${iterations.size === 1 ? '' : 's'} seen`);
  return bits.join(' · ');
}

type StepNodeData = {
  step: FormulaDashboardStep;
  onSelect: (step: FormulaDashboardStep) => void;
};

function StepFlowNode({ data }: NodeProps<Node<StepNodeData>>) {
  const step = data.step;
  const latest = step.activities?.at(-1);
  const loopSummary = loopActivitySummary(step);
  return (
    <button type="button" className={`graph-node flow-graph-node ${step.status}`} onClick={() => data.onSelect(step)}>
      <Handle type="target" position={Position.Left} className="flow-handle" />
      <div className="graph-node-topline">
        <div className="graph-node-id">{graphShortId(step.id)}</div>
        <span className={`graph-node-state ${step.status}`}>{step.status}</span>
      </div>
      <strong>{step.title}</strong>
      <p>{step.description || step.notes || 'Structured execution step in the formula pipeline.'}</p>
      {loopSummary && <div className="loop-summary-pill">↻ {loopSummary}</div>}
      {latest && <div className="step-activity-mini"><span>{latest.at}</span>{activityShortId(latest.step_id)} · {statusLabel(latest.status)}</div>}
      <div className="graph-node-meta">
        <span><PartitionOutlined /> {step.agent || 'default agent'}</span>
        {!!step.depends_on?.length && <span><ExpandOutlined /> {step.depends_on.length} deps</span>}
      </div>
      <Handle type="source" position={Position.Right} className="flow-handle" />
    </button>
  );
}

const nodeTypes = { step: StepFlowNode };

function computeGraphLayout(snapshot: FormulaDashboardSnapshot, onSelect: (step: FormulaDashboardStep) => void) {
  const grouped = new Map<number, FormulaDashboardStep[]>();
  for (const step of snapshot.steps) {
    const depth = step.depth || 0;
    if (!grouped.has(depth)) grouped.set(depth, []);
    grouped.get(depth)!.push(step);
  }

  const depths = Array.from(grouped.keys()).sort((a, b) => a - b);
  for (const depth of depths) {
    grouped.get(depth)!.sort((a, b) => a.index - b.index);
  }

  const nodeWidth = 300;
  const nodeHeight = 178;
  const colGap = 76;
  const rowGap = 28;
  const paddingX = 28;
  const paddingTop = 52;
  const paddingBottom = 28;

  const nodes: Node<StepNodeData>[] = snapshot.steps.map(step => {
    const depth = step.depth || 0;
    const column = grouped.get(depth) || [];
    const row = column.findIndex(candidate => candidate.id === step.id);
    return {
      id: step.id,
      type: 'step',
      data: { step, onSelect },
      position: {
        x: paddingX + depth * (nodeWidth + colGap),
        y: paddingTop + row * (nodeHeight + rowGap),
      },
      style: { width: nodeWidth, height: nodeHeight },
    };
  });

  const nodeIDs = new Set(nodes.map(node => node.id));
  const edges: Edge[] = snapshot.edges.flatMap(edge => {
    if (!nodeIDs.has(edge.from) || !nodeIDs.has(edge.to)) return [];
    return [{
      id: `${edge.from}-${edge.to}`,
      source: edge.from,
      target: edge.to,
      type: 'smoothstep',
      animated: edge.type === 'blocks' || snapshot.steps.some(step => step.id === edge.to && step.status === 'running'),
      markerEnd: { type: MarkerType.ArrowClosed, color: 'rgba(148, 163, 184, 0.82)' },
      className: `flow-edge ${edge.type || 'default'}`,
    }];
  });

  const width = Math.max(720, paddingX * 2 + Math.max(1, depths.length) * nodeWidth + Math.max(0, depths.length-1) * colGap);
  const maxRows = Math.max(1, ...depths.map(depth => grouped.get(depth)?.length || 0));
  const height = Math.max(360, paddingTop + paddingBottom + maxRows * nodeHeight + Math.max(0, maxRows-1) * rowGap);

  return { widths: { nodeWidth, nodeHeight, colGap, paddingX }, depths, nodes, edges, width, height };
}

function GraphPanel({ snapshot, onSelect }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void }) {
  const layout = useMemo(() => computeGraphLayout(snapshot, onSelect), [snapshot, onSelect]);
  const running = snapshot.steps.find(step => step.status === 'running');

  if (!snapshot.steps.length) {
    return <Empty description="No executable steps" />;
  }

  return (
    <div className="graph-panel">
      <div className="graph-header">
        <div>
          <h3>Execution graph</h3>
          <p>Live flow map with luminous stages, dependency paths, and direct click-to-inspect details.</p>
        </div>
        <div className="graph-header-side">
          {running && (
            <div className="graph-now-running">
              <span>Live</span>
              <strong>{running.title}</strong>
            </div>
          )}
          <div className="graph-legend">
          {['pending', 'running', 'completed', 'failed', 'skipped'].map(status => (
            <span className={`legend-pill ${status}`} key={status}>{status}</span>
          ))}
        </div>
        </div>
      </div>
      <div className="graph-canvas react-flow-canvas">
        <ReactFlow
          nodes={layout.nodes}
          edges={layout.edges}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.18 }}
          minZoom={0.35}
          maxZoom={1.4}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable
        >
          <Background color="rgba(125, 211, 252, 0.18)" gap={28} size={1} />
          <MiniMap pannable zoomable nodeStrokeWidth={3} className="flow-minimap" />
          <Controls className="flow-controls" />
        </ReactFlow>
      </div>
    </div>
  );
}

function StepCard({ step, onSelect }: { step: FormulaDashboardStep; onSelect: (step: FormulaDashboardStep) => void }) {
  const latest = step.activities?.at(-1);
  const loopSummary = loopActivitySummary(step);
  return (
    <button type="button" className={`step-card ${step.status}`} onClick={() => onSelect(step)}>
      <div className="step-card-row">
        <div>
          <div className="step-card-kicker">{step.id}</div>
          <h3>{step.title}</h3>
        </div>
        <Tag color={statusTone[step.status] || 'default'} icon={statusIcon(step.status)}>{step.status}</Tag>
      </div>
      <p>{step.description || step.notes || 'No extra description for this step.'}</p>
      {loopSummary && <div className="loop-summary-pill">↻ {loopSummary}</div>}
      {latest && (
        <div className={`step-activity-mini ${latest.status}`}>
          <span>{latest.at}</span>{activityShortId(latest.step_id)} · {latest.title || statusLabel(latest.status)}
        </div>
      )}
      <div className="step-chip-row">
        {step.agent && <span className="step-chip">agent · {step.agent}</span>}
        {step.model && <span className="step-chip">model · {step.model}</span>}
        {!!step.depends_on?.length && <span className="step-chip">deps · {step.depends_on.length}</span>}
        {step.loop && <span className="step-chip loop-chip">loop · {step.loop.body?.length || 0} body</span>}
        {!!step.activities?.length && <span className="step-chip">activity · {step.activities.length}</span>}
        {step.output_key && <span className="step-chip">output · {step.output_key}</span>}
      </div>
    </button>
  );
}

function StepInspector({ step, open, onClose }: { step: FormulaDashboardStep | null; open: boolean; onClose: () => void }) {
  if (!step) return null;
  const metadataEntries = Object.entries(step.metadata || {});
  const labels = step.labels || [];
  const inputCtx = step.input_ctx || [];
  const activities = step.activities || [];
  const loopBody = step.loop?.body || [];

  return (
    <Drawer open={open} onClose={onClose} width="96vw" title="Step Inspector" className="step-inspector" destroyOnClose>
      <div className="inspector-title-block">
        <div className="step-card-kicker">{step.id}</div>
        <h2>{step.title}</h2>
      </div>
      <div className="step-modal-tagbar">
        <Tag color={statusTone[step.status] || 'default'}>{step.status}</Tag>
        {step.agent && <Tag>{step.agent}</Tag>}
        {step.model && <Tag>{step.model}</Tag>}
        {step.type && <Tag>{step.type}</Tag>}
        {step.priority && <Tag>P{step.priority}</Tag>}
      </div>

      <div className="step-modal-body inspector-body">
        <aside className="step-modal-sidebar">
          <Card size="small" title="Runtime" className="step-sidebar-card">
            <Descriptions column={1} size="small" className="step-descriptions">
              <Descriptions.Item label="ID">{step.id}</Descriptions.Item>
              <Descriptions.Item label="Duration">{formatDuration(step.duration_ms)}</Descriptions.Item>
              <Descriptions.Item label="Session">{step.session || '—'}</Descriptions.Item>
              <Descriptions.Item label="Execution">{step.execution || '—'}</Descriptions.Item>
              <Descriptions.Item label="Condition">{step.condition || '—'}</Descriptions.Item>
            </Descriptions>
          </Card>

          <Card size="small" title="Dependencies" className="step-sidebar-card">
            {step.depends_on?.length ? (
              <ul className="pill-list">
                {step.depends_on.map(dep => <li key={dep}>{dep}</li>)}
              </ul>
            ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No dependencies" />}
          </Card>

          <Card size="small" title="Inputs & Labels" className="step-sidebar-card">
            {!!inputCtx.length && (
              <>
                <div className="card-subtitle">Input context</div>
                <ul className="pill-list">{inputCtx.map(key => <li key={key}>{key}</li>)}</ul>
              </>
            )}
            {!!labels.length && (
              <>
                <div className="card-subtitle">Labels</div>
                <ul className="pill-list">{labels.map(label => <li key={label}>{label}</li>)}</ul>
              </>
            )}
            {!inputCtx.length && !labels.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No labels or inputs" />}
          </Card>

          {metadataEntries.length > 0 && (
            <Card size="small" title="Metadata" className="step-sidebar-card">
              <Descriptions column={1} size="small" className="step-descriptions">
                {metadataEntries.map(([key, value]) => (
                  <Descriptions.Item key={key} label={key}>{value}</Descriptions.Item>
                ))}
              </Descriptions>
            </Card>
          )}
        </aside>

        <div className="step-modal-main">
          {(step.description || step.notes) && (
            <section className="step-modal-section">
              <div className="step-modal-section-header">
                <span className="step-modal-section-icon">📋</span>
                <h4>Brief</h4>
              </div>
              <div className="step-modal-section-body">
                {step.description && <p className="step-description-text">{step.description}</p>}
                {step.notes && <pre className="code-block">{step.notes}</pre>}
              </div>
            </section>
          )}

          {step.loop && (
            <section className="step-modal-section loop-plan-section">
              <div className="step-modal-section-header">
                <span className="step-modal-section-icon">🔁</span>
                <h4>Loop plan</h4>
              </div>
              <div className="loop-plan-body">
                <div className="loop-plan-summary">
                  <strong>{step.loop.summary || 'Runtime loop'}</strong>
                  <span>{loopBody.length} planned body step{loopBody.length === 1 ? '' : 's'}</span>
                </div>
                <div className="loop-plan-grid">
                  {step.loop.until && <div><span>Until</span><code>{step.loop.until}</code></div>}
                  {!!step.loop.max && <div><span>Max</span><code>{step.loop.max}</code></div>}
                  {!!step.loop.count && <div><span>Count</span><code>{step.loop.count}</code></div>}
                  {step.loop.range && <div><span>Range</span><code>{step.loop.range}</code></div>}
                  {step.loop.var && <div><span>Var</span><code>{step.loop.var}</code></div>}
                </div>
                {loopBody.length > 0 && (
                  <div className="loop-body-list">
                    {loopBody.map((body, index) => (
                      <div className="loop-body-card" key={`${body.id}-${index}`}>
                        <div className="loop-body-index">#{index + 1}</div>
                        <div className="loop-body-main">
                          <div className="loop-body-head">
                            <strong>{body.title || body.id}</strong>
                            <code>{body.id}</code>
                          </div>
                          {body.description && <p>{body.description}</p>}
                          <div className="loop-body-meta">
                            {(body.agent || body.model) && <span>agent · {[body.agent, body.model].filter(Boolean).join(' / ')}</span>}
                            {body.output_key && <span>output · {body.output_key}</span>}
                            {!!body.input_ctx?.length && <span>input · {body.input_ctx.join(', ')}</span>}
                            {body.condition && <span>if · {body.condition}</span>}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </section>
          )}

          {step.output && (
            <section className="step-modal-section">
              <div className="step-modal-section-header">
                <span className="step-modal-section-icon">📄</span>
                <h4>Output</h4>
              </div>
              <OutputSurface content={step.output} className="step-output-shell" />
            </section>
          )}

          {activities.length > 0 && (
            <section className="step-modal-section">
              <div className="step-modal-section-header">
                <span className="step-modal-section-icon">🛰️</span>
                <h4>Step activity</h4>
              </div>
              <div className="step-activity-list">
                {activities.map(activity => (
                  <div key={`${activity.step_id}-${activity.at}-${activity.status}`} className={`step-activity-row ${activity.status}`}>
                    <div className="step-activity-status-dot" />
                    <div className="step-activity-content">
                      <div className="step-activity-head">
                        <strong>{activity.title || activityShortId(activity.step_id)}</strong>
                        <Tag color={statusTone[activity.status] || 'default'}>{statusLabel(activity.status)}</Tag>
                      </div>
                      <div className="step-activity-meta">{activity.at} · {activity.step_id}{activity.duration_ms ? ` · ${formatDuration(activity.duration_ms)}` : ''}</div>
                      {activity.detail && <p>{activity.detail}</p>}
                      {activity.output && <OutputSurface content={activity.output} className="step-activity-output" />}
                      {activity.error && <pre className="code-block error-block">{activity.error}</pre>}
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}

          {step.error && (
            <section className="step-modal-section">
              <div className="step-modal-section-header">
                <span className="step-modal-section-icon error-icon">⚠️</span>
                <h4>Error</h4>
              </div>
              <pre className="code-block error-block">{step.error}</pre>
            </section>
          )}
        </div>
      </div>
    </Drawer>
  );
}

function HumanInputModal({ step, onSubmit }: { step: FormulaDashboardStep | undefined; onSubmit: (stepID: string, values: Record<string, unknown>) => Promise<void> }) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const request = step?.human_input_request;
  const fields = request?.form?.fields || [];
  const title = request?.form?.title || `Input needed${step?.title ? `: ${step.title}` : ''}`;

  useEffect(() => {
    if (!step) return;
    const initial: Record<string, unknown> = {};
    for (const field of fields) {
      if (field.default) initial[field.name] = field.default;
      if (field.type === 'checkbox' && !initial[field.name]) initial[field.name] = [];
    }
    form.setFieldsValue(initial);
  }, [step?.id]);

  const submit = async () => {
    if (!step) return;
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      await onSubmit(step.id, values);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={!!step}
      title={title}
      okText={request?.form?.submit_label || 'Submit and resume'}
      onOk={submit}
      confirmLoading={submitting}
      closable={false}
      maskClosable={false}
      width={640}
      cancelButtonProps={{ style: { display: 'none' } }}
    >
      {request?.reason && <Alert type="warning" showIcon message={request.reason} style={{ marginBottom: 16 }} />}
      {request?.form?.description && <Alert type="info" showIcon message={request.form.description} style={{ marginBottom: 16 }} />}
      <Form form={form} layout="vertical" preserve={false} requiredMark="optional">
        {fields.map(field => {
          const rules = field.required ? [{ required: true, message: `${field.label || field.name} is required` }] : undefined;
          const options = (field.options || []).map(value => ({ label: value, value }));
          let control = <Input placeholder={field.placeholder} />;
          if (field.type === 'textarea') control = <TextArea rows={4} placeholder={field.placeholder} />;
          if (field.type === 'radio') control = <Radio.Group options={options} />;
          if (field.type === 'checkbox') control = <Checkbox.Group options={options} />;
          if (field.type === 'select') control = <Select options={options} placeholder={field.placeholder} />;
          return (
            <Form.Item key={field.name} name={field.name} label={field.label || field.name} rules={rules} extra={field.help}>
              {control}
            </Form.Item>
          );
        })}
      </Form>
    </Modal>
  );
}

export function App() {
  const { message } = AntdApp.useApp();
  const [snapshot, setSnapshot] = useState<FormulaDashboardSnapshot | null>(null);
  const [view, setView] = useState<DashboardView>(currentView());
  const [selectedStep, setSelectedStep] = useState<FormulaDashboardStep | null>(null);
  const [finalOutputOpen, setFinalOutputOpen] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    api.state().then(setSnapshot).catch(err => {
      setError(String(err));
      message.error(`Failed to load formula dashboard: ${String(err)}`);
    });
  }, [message]);

  useEffect(() => {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    let timer: ReturnType<typeof setTimeout> | undefined;
    let ws: WebSocket | null = null;

    const connect = () => {
      ws = new WebSocket(`${proto}//${location.host}/ws`);
      ws.onmessage = event => {
        try {
          const msg = JSON.parse(event.data) as FormulaDashboardMessage;
          if (msg.type === 'state') {
            setSnapshot(normalizeSnapshot(msg.state));
            setError('');
          }
        } catch (err) {
          console.error(err);
        }
      };
      ws.onclose = () => {
        timer = setTimeout(connect, 1500);
      };
      ws.onerror = event => {
        console.error(event);
      };
    };

    connect();
    return () => {
      if (timer) clearTimeout(timer);
      ws?.close();
    };
  }, []);

  const summary = useMemo(() => {
    if (!snapshot) return null;
    const counts = snapshot.steps.reduce<Record<string, number>>((acc, step) => {
      acc[step.status] = (acc[step.status] || 0) + 1;
      return acc;
    }, {});
    return {
      steps: snapshot.steps.length,
      running: counts.running || 0,
      completed: counts.completed || 0,
      skipped: counts.skipped || 0,
      failed: counts.failed || 0,
      logs: snapshot.logs.length,
    };
  }, [snapshot]);

  const progress = summary?.steps ? Math.round(((summary.completed + summary.skipped) / summary.steps) * 100) : 0;
  const runningStep = snapshot?.steps.find(step => step.status === 'running');
  const waitingInputStep = snapshot?.steps.find(step => step.status === 'waiting_input' && step.human_input_request);

  const orderedSteps = useMemo(() => {
    return [...(snapshot?.steps || [])].sort((a, b) => {
      const statusDelta = (statusOrder[a.status] ?? 99) - (statusOrder[b.status] ?? 99);
      if (statusDelta !== 0) return statusDelta;
      if ((a.depth || 0) !== (b.depth || 0)) return (a.depth || 0) - (b.depth || 0);
      return a.index - b.index;
    });
  }, [snapshot]);

  const setDashboardView = (next: DashboardView) => {
    localStorage.setItem('formula-dashboard-view', next);
    setView(next);
  };

  const submitHumanInput = async (stepID: string, values: Record<string, unknown>) => {
    await api.submitHumanInput(stepID, values);
    message.success('Human input submitted. Resuming workflow…');
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
          </Card>
        </aside>
      </section>

      <StepInspector step={selectedStep} open={!!selectedStep} onClose={() => setSelectedStep(null)} />
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
