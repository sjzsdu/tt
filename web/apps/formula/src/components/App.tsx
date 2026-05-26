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
  CopyOutlined,
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
  FormulaStepActivity,
  FormulaDashboardLoopBody,
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

function latestLoopActivity(step: FormulaDashboardStep) {
  return [...(step.activities || [])].reverse().find(activity => /\.iter\d+\./.test(activity.step_id));
}

function loopActivityIteration(activity?: { step_id: string }) {
  return activity?.step_id.match(/\.iter(\d+)\./)?.[1] || '';
}

function loopActivityBodyID(activity?: { step_id: string }) {
  return activity?.step_id.match(/\.iter\d+\.([^.]*)$/)?.[1] || '';
}

function groupLoopActivities(activities: FormulaStepActivity[]) {
  const groups: Array<{ iteration: string; activities: FormulaStepActivity[] }> = [];
  const byIteration = new Map<string, FormulaStepActivity[]>();
  for (const activity of activities) {
    const iteration = loopActivityIteration(activity) || 'step';
    if (!byIteration.has(iteration)) {
      byIteration.set(iteration, []);
      groups.push({ iteration: iteration === 'step' ? '' : iteration, activities: byIteration.get(iteration)! });
    }
    byIteration.get(iteration)!.push(activity);
  }
  return groups;
}

type StepNodeData = {
  step: FormulaDashboardStep;
  kind?: 'step' | 'loop-body';
  parentStep?: FormulaDashboardStep;
  body?: FormulaDashboardLoopBody;
  onSelect: (step: FormulaDashboardStep) => void;
};

type LoopGroupNodeData = {
  step: FormulaDashboardStep;
  bodyCount: number;
};

function StepFlowNode({ data }: NodeProps<Node<StepNodeData>>) {
  const step = data.step;
  const isLoopBody = data.kind === 'loop-body';
  const latest = step.activities?.at(-1);
  const loopSummary = loopActivitySummary(step);
  if (isLoopBody) {
    const parent = data.parentStep || step;
    return (
      <button type="button" className={`graph-node flow-graph-node loop-body-node ${step.status}`} onClick={() => data.onSelect(parent)}>
        <Handle type="target" position={Position.Left} className="flow-handle" />
        <div className="graph-node-topline">
          <div className="graph-node-id">body · {graphShortId(step.id)}</div>
          <span className={`graph-node-state ${step.status}`}>{statusLabel(step.status)}</span>
        </div>
        <strong>{step.title}</strong>
        {step.metadata?.iteration && <div className="loop-iteration-pill">iteration {step.metadata.iteration}</div>}
        <div className="graph-node-meta">
          {step.output_key && <span>out · {step.output_key}</span>}
          {!!step.input_ctx?.length && <span>in · {step.input_ctx.join(', ')}</span>}
        </div>
        <Handle type="source" position={Position.Right} className="flow-handle" />
      </button>
    );
  }
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

function LoopGroupNode({ data }: NodeProps<Node<LoopGroupNodeData>>) {
  const step = data.step;
  const summary = loopActivitySummary(step) || 'loop body';
  const latest = latestLoopActivity(step);
  const iteration = loopActivityIteration(latest);
  const bodyID = loopActivityBodyID(latest);
  return (
    <div className={`loop-group-node ${step.status}`}>
      <div className="loop-group-title">↻ Loop subgraph</div>
      <strong>{step.title}</strong>
      <span>{summary} · {data.bodyCount} step{data.bodyCount === 1 ? '' : 's'}</span>
      {latest && <em>active: iter {iteration || '?'} · {bodyID || activityShortId(latest.step_id)} · {statusLabel(latest.status)}</em>}
    </div>
  );
}

const nodeTypes = { step: StepFlowNode, loopGroup: LoopGroupNode };

function edgeVisualClass(sourceStatus?: string, targetStatus?: string, extra = '') {
  const state = targetStatus === 'running'
    ? 'active'
    : targetStatus === 'failed'
      ? 'failed'
      : sourceStatus === 'completed' && targetStatus === 'completed'
        ? 'completed'
        : targetStatus === 'skipped'
          ? 'skipped'
          : 'pending';
  return `flow-edge ${extra} ${state}`.trim();
}

function edgeMarkerColor(status?: string) {
  if (status === 'running') return '#67e8f9';
  if (status === 'completed') return '#34d399';
  if (status === 'failed') return '#fb7185';
  if (status === 'skipped') return '#fbbf24';
  return 'rgba(148, 163, 184, 0.82)';
}

function loopBodyGraphID(parentID: string, bodyID: string) {
  return `${parentID}__${bodyID}`;
}

function loopBodyStep(parent: FormulaDashboardStep, body: FormulaDashboardLoopBody, index: number): FormulaDashboardStep {
  const bodyActivity = [...(parent.activities || [])]
    .reverse()
    .find(activity => loopActivityBodyID(activity) === body.id);
  return {
    id: loopBodyGraphID(parent.id, body.id),
    title: body.title || body.id,
    description: body.description,
    type: 'loop-body',
    agent: body.agent || parent.agent,
    model: body.model || parent.model,
    status: bodyActivity?.status || 'pending',
    output: bodyActivity?.output,
    error: bodyActivity?.error,
    duration_ms: bodyActivity?.duration_ms,
    output_key: body.output_key,
    input_ctx: body.input_ctx,
    condition: body.condition,
    depends_on: body.depends_on,
    metadata: {
      parent_step: parent.id,
      body_id: body.id,
      iteration: loopActivityIteration(bodyActivity),
    },
    activities: bodyActivity ? [bodyActivity] : [],
    depth: (parent.depth || 0) + 1,
    index,
  };
}

function computeGraphLayout(snapshot: FormulaDashboardSnapshot, onSelect: (step: FormulaDashboardStep) => void) {
  const grouped = new Map<number, FormulaDashboardStep[]>();
  for (const step of snapshot.steps) {
    const depth = step.depth || 0;
    if (!grouped.has(depth)) grouped.set(depth, []);
    grouped.get(depth)!.push(step);
  }

  const depths = Array.from(grouped.keys()).sort((a, b) => a - b);
  for (const depth of depths) grouped.get(depth)!.sort((a, b) => a.index - b.index);

  const nodeWidth = 300;
  const nodeHeight = 178;
  const colGap = 110;
  const rowGap = 44;
  const paddingX = 28;
  const paddingTop = 52;
  const paddingBottom = 28;
  const nodes: Node<StepNodeData | LoopGroupNodeData>[] = [];
  const nodeIDs = new Set<string>();
  const stepStatus = new Map(snapshot.steps.map(step => [step.id, step.status]));
  let maxY = paddingTop;

  const depthWidths = new Map<number, number>();
  for (const depth of depths) {
    depthWidths.set(depth, nodeWidth);
  }
  const depthX = new Map<number, number>();
  let nextX = paddingX;
  for (const depth of depths) {
    depthX.set(depth, nextX);
    nextX += (depthWidths.get(depth) || nodeWidth) + colGap;
  }

  for (const depth of depths) {
    let y = paddingTop;
    const column = grouped.get(depth) || [];
    for (const step of column) {
      const x = depthX.get(depth) || paddingX;
      nodes.push({ id: step.id, type: 'step', data: { step, kind: 'step', onSelect }, position: { x, y }, style: { width: nodeWidth, height: nodeHeight } });
      nodeIDs.add(step.id);

      const loopBody = step.loop?.body || [];
      if (loopBody.length) {
        const bodyX = x + nodeWidth + Math.floor(colGap * 0.72);
        let bodyY = y + 26;
        for (const [bodyIndex, body] of loopBody.entries()) {
          const bodyStep = loopBodyStep(step, body, bodyIndex);
          const bodyNodeID = loopBodyGraphID(step.id, body.id);
          nodes.push({
            id: bodyNodeID,
            type: 'step',
            data: { step: bodyStep, kind: 'loop-body', parentStep: step, body, onSelect },
            position: { x: bodyX, y: bodyY },
            style: { width: nodeWidth - 28, height: 132 },
          });
          bodyY += 152;
          nodeIDs.add(bodyNodeID);
          stepStatus.set(bodyNodeID, bodyStep.status);
        }
        maxY = Math.max(maxY, bodyY + paddingBottom);
      }

      y += nodeHeight + rowGap;
      maxY = Math.max(maxY, y);
    }
  }
  const statusFor = (id: string) => stepStatus.get(id) || 'pending';
  const edges: Edge[] = snapshot.edges.flatMap(edge => {
    if (!nodeIDs.has(edge.from) || !nodeIDs.has(edge.to)) return [];
    const targetStatus = statusFor(edge.to);
    const sourceStatus = statusFor(edge.from);
    return [{
      id: `${edge.from}-${edge.to}`,
      source: edge.from,
      target: edge.to,
      type: 'smoothstep',
      animated: targetStatus === 'running',
      markerEnd: { type: MarkerType.ArrowClosed, color: edgeMarkerColor(targetStatus) },
      className: edgeVisualClass(sourceStatus, targetStatus, edge.type || 'default'),
    }];
  });

  for (const step of snapshot.steps) {
    const loopBody = step.loop?.body || [];
    if (!loopBody.length) continue;
    const bodyIDs = new Set(loopBody.map(body => body.id));
    for (const [index, body] of loopBody.entries()) {
      const target = loopBodyGraphID(step.id, body.id);
      const deps = body.depends_on?.filter(dep => bodyIDs.has(dep)) || [];
      const sources = deps.length
        ? deps.map(dep => loopBodyGraphID(step.id, dep))
        : index === 0
          ? [step.id]
          : [loopBodyGraphID(step.id, loopBody[index - 1].id)];
      for (const source of sources) {
        const targetStatus = statusFor(target);
        const sourceStatus = statusFor(source);
        edges.push({
          id: `${source}-${target}`,
          source,
          target,
          type: 'smoothstep',
          animated: targetStatus === 'running',
          markerEnd: { type: MarkerType.ArrowClosed, color: edgeMarkerColor(targetStatus) },
          className: edgeVisualClass(sourceStatus, targetStatus, 'loop-body-edge'),
        });
      }
    }
  }

  const hasLoopBodyNodes = snapshot.steps.some(step => !!step.loop?.body?.length);
  const width = Math.max(720, nextX + paddingX - colGap + (hasLoopBodyNodes ? nodeWidth : 0));
  const height = Math.max(420, paddingBottom + maxY);
  return { widths: { nodeWidth, nodeHeight, colGap, paddingX }, depths, nodes, edges, width, height };
}

function GraphPanel({ snapshot, onSelect }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void }) {
  const layout = useMemo(() => computeGraphLayout(snapshot, onSelect), [snapshot, onSelect]);
  const running = snapshot.steps.find(step => step.status === 'running');
  const loopSteps = snapshot.steps.filter(step => !!step.loop?.body?.length).length;

  if (!snapshot.steps.length) {
    return <Empty description="No executable steps" />;
  }

  return (
    <div className="graph-panel">
      <div className="graph-header">
        <div>
          <h3>Execution graph</h3>
          <p>Top-level steps plus loop body nodes. Click any node to inspect output, errors, and iteration activity.</p>
          <div className="graph-header-metrics">
            <span>{layout.nodes.length} nodes</span>
            <span>{layout.edges.length} edges</span>
            {!!loopSteps && <span>{loopSteps} loop{loopSteps === 1 ? '' : 's'} expanded</span>}
          </div>
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

function attentionStep(snapshot: FormulaDashboardSnapshot) {
  return snapshot.steps.find(step => step.status === 'waiting_input')
    || snapshot.steps.find(step => step.status === 'failed')
    || snapshot.steps.find(step => step.status === 'running')
    || null;
}

function attentionCopy(step: FormulaDashboardStep | null, status: string) {
  if (!step) {
    if (status === 'completed') return { title: 'Run completed', detail: 'Open the final report or inspect completed steps.', tone: 'completed' };
    return { title: 'No active step', detail: 'The run is waiting for the scheduler or next update.', tone: 'pending' };
  }
  if (step.status === 'waiting_input') return { title: 'Input required', detail: step.title, tone: 'waiting_input' };
  if (step.status === 'failed') return { title: 'Action needed', detail: step.title, tone: 'failed' };
  if (step.status === 'running') return { title: 'Currently running', detail: step.title, tone: 'running' };
  return { title: statusLabel(step.status), detail: step.title, tone: step.status };
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

function StepInspector({ step, snapshot, open, onClose, onRetry }: { step: FormulaDashboardStep | null; snapshot: FormulaDashboardSnapshot | null; open: boolean; onClose: () => void; onRetry: (step: FormulaDashboardStep) => void }) {
  if (!step) return null;
  const metadataEntries = Object.entries(step.metadata || {});
  const labels = step.labels || [];
  const inputCtx = step.input_ctx || [];
  const inputValues = collectStepInputValues(step, snapshot);
  const activities = step.activities || [];
  const loopBody = step.loop?.body || [];
  const loopActivityGroups = step.loop ? groupLoopActivities(activities) : [];
  const hiddenDuplicateOutputs = activities.filter(activity => activity.output && sameOutput(activity.output, step.output)).length;

  return (
    <Drawer open={open} onClose={onClose} width="96vw" title="Step Inspector" className="step-inspector" destroyOnClose>
      <div className="inspector-title-block">
        <div className="step-card-kicker">{step.id}</div>
        <h2>{step.title}</h2>
        {step.status === 'failed' && (
          <Button type="primary" danger onClick={() => onRetry(step)}>Retry this step</Button>
        )}
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
                  {step.loop.for_each && <div><span>For each</span><code>{step.loop.for_each}</code></div>}
                  {step.loop.var && <div><span>Var</span><code>{step.loop.var}</code></div>}
                  {step.loop.parallel && <div><span>Mode</span><code>parallel</code></div>}
                  {!!step.loop.max_concurrency && <div><span>Max concurrency</span><code>{step.loop.max_concurrency}</code></div>}
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

          <section className="step-modal-section step-input-section">
            <div className="step-modal-section-header">
              <span className="step-modal-section-icon">📥</span>
              <h4>Input</h4>
            </div>
            {inputValues.length > 0 ? (
              <div className="step-input-list">
                {inputValues.map(input => (
                  <div className="step-input-card" key={input.key}>
                    <div className="step-input-head">
                      <strong>{input.key}</strong>
                      <Tag>{input.source}</Tag>
                    </div>
                    <OutputSurface content={input.value} className="step-input-output" />
                  </div>
                ))}
              </div>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No resolved input data yet" />
            )}
          </section>

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
                <h4>Activity timeline</h4>
              </div>
              <p className="step-section-hint">
                {step.loop ? 'Internal loop body activity grouped by iteration. These are not separate top-level steps.' : 'Status events for this step. The final output is shown once in the Output section above.'}
              </p>
              <div className="step-activity-list">
                {(step.loop && loopActivityGroups.length ? loopActivityGroups : [{ iteration: '', activities }]).map(group => (
                  <div key={group.iteration || 'step'} className="step-activity-group">
                    {group.iteration && <div className="step-activity-group-title">Iteration {group.iteration}</div>}
                    {group.activities.map(activity => (
                  <div key={`${activity.step_id}-${activity.at}-${activity.status}`} className={`step-activity-row ${activity.status}`}>
                    <div className="step-activity-status-dot" />
                    <div className="step-activity-content">
                      <div className="step-activity-head">
                        <strong>{activity.title || activityShortId(activity.step_id)}</strong>
                        <Tag color={statusTone[activity.status] || 'default'}>{statusLabel(activity.status)}</Tag>
                      </div>
                      <div className="step-activity-meta">{activity.at} · {activity.step_id}{activity.duration_ms ? ` · ${formatDuration(activity.duration_ms)}` : ''}</div>
                      {activity.detail && <p>{activity.detail}</p>}
                      {activity.output && !sameOutput(activity.output, step.output) && <OutputSurface content={activity.output} className="step-activity-output" />}
                      {activity.output && sameOutput(activity.output, step.output) && <div className="step-activity-output-note">Output matches final step output.</div>}
                      {activity.error && <pre className="code-block error-block">{activity.error}</pre>}
                    </div>
                  </div>
                    ))}
                  </div>
                ))}
              </div>
              {hiddenDuplicateOutputs > 0 && <div className="step-section-footnote">Hidden duplicate output in {hiddenDuplicateOutputs} timeline event{hiddenDuplicateOutputs === 1 ? '' : 's'}.</div>}
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

function sameOutput(a?: string, b?: string) {
  return !!a && !!b && a.trim() === b.trim();
}

function collectStepInputValues(step: FormulaDashboardStep, snapshot: FormulaDashboardSnapshot | null) {
  if (!snapshot) return [];
  const byID = new Map(snapshot.steps.map(item => [item.id, item]));
  const keys: Array<{ key: string; source: string }> = [];
  const seen = new Set<string>();
  const add = (key?: string, source = 'context') => {
    const clean = (key || '').trim();
    if (!clean || seen.has(clean)) return;
    seen.add(clean);
    keys.push({ key: clean, source });
  };
  step.input_ctx?.forEach(key => add(key, 'input_context'));
  step.depends_on?.forEach(key => add(key, 'dependency'));
  if (step.loop?.for_each) add(step.loop.for_each, 'for_each');

  return keys.map(({ key, source }) => {
    const root = key.split('.')[0];
    const sourceStep = byID.get(root);
    const value = resolveStepInputValue(key, sourceStep?.output);
    return value ? { key, source, value } : null;
  }).filter((item): item is { key: string; source: string; value: string } => !!item);
}

function resolveStepInputValue(key: string, output?: string) {
  if (!output) return '';
  const parts = key.split('.').slice(1);
  if (!parts.length) return output;
  try {
    let current: unknown = JSON.parse(output);
    for (const part of parts) {
      if (!current || typeof current !== 'object' || !(part in current)) return output;
      current = (current as Record<string, unknown>)[part];
    }
    return typeof current === 'string' ? current : JSON.stringify(current, null, 2);
  } catch {
    return output;
  }
}

function RetryStepModal({ step, open, onCancel, onSubmit }: { step: FormulaDashboardStep | null; open: boolean; onCancel: () => void; onSubmit: (stepID: string, advice?: string) => Promise<void> }) {
  const [advice, setAdvice] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const isScript = step?.execution === 'script';

  useEffect(() => {
    if (open) setAdvice('');
  }, [open, step?.id]);

  const submit = async () => {
    if (!step) return;
    setSubmitting(true);
    try {
      await onSubmit(step.id, isScript ? '' : advice);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={open}
      title={isScript ? `Retry script step: ${step?.title || ''}` : `Retry agent step: ${step?.title || ''}`}
      okText="Restart step"
      onOk={submit}
      onCancel={onCancel}
      confirmLoading={submitting}
      width={640}
    >
      {isScript ? (
        <Alert type="info" showIcon message="This script step will be restarted with the same command and context." />
      ) : (
        <>
          <Alert type="info" showIcon message="Add optional guidance. It will be appended to the agent prompt for this retry." style={{ marginBottom: 16 }} />
          <TextArea rows={6} value={advice} onChange={event => setAdvice(event.target.value)} placeholder="Tell the agent what to adjust before retrying…" />
        </>
      )}
    </Modal>
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
  const [retryStep, setRetryStep] = useState<FormulaDashboardStep | null>(null);
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

  useEffect(() => {
    if (!snapshot) return;
    setSelectedStep(current => {
      if (!current) return current;
      return snapshot.steps.find(step => step.id === current.id) || current;
    });
  }, [snapshot]);

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
  const focusedStep = snapshot ? attentionStep(snapshot) : null;
  const focusCopy = snapshot ? attentionCopy(focusedStep, snapshot.status) : null;

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
