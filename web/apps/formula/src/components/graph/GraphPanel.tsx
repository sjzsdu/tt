import { useCallback, useMemo, useState } from 'react';
import { Empty } from 'antd';
import { ExpandOutlined, PartitionOutlined } from '@ant-design/icons';
import { ReactFlow, Background, Controls, Handle, MarkerType, MiniMap, Position, type Edge, type Node, type NodeProps } from '@xyflow/react';
import type { FormulaDashboardLoopBody, FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { activityShortId, graphShortId, statusLabel } from '../../utils/status';
import { latestLoopActivity, loopActivityBodyID, loopActivityIteration, loopActivitySummary } from '../../utils/steps';

type StepNodeData = {
  step: FormulaDashboardStep;
  kind?: 'step' | 'loop-body';
  parentStep?: FormulaDashboardStep;
  body?: FormulaDashboardLoopBody;
  onSelect: (step: FormulaDashboardStep) => void;
  onToggleExpand?: (stepID: string) => void;
  expanded?: boolean;
};

type LoopGroupNodeData = {
  step: FormulaDashboardStep;
  bodyCount: number;
};

function StepFlowNode({ data }: NodeProps<Node<StepNodeData>>) {
  const step = data.step;
  const isLoopBody = data.kind === 'loop-body';
  const isLoop = !!step.loop?.body?.length;
  const latest = step.activities?.at(-1);
  const loopSummary = loopActivitySummary(step);
  if (isLoopBody) {
    const parent = data.parentStep || step;
    return (
      <button type="button" className={`graph-node flow-graph-node loop-body-node ${step.status}`} onClick={() => data.onSelect(parent)}>
        <Handle type="target" position={Position.Top} className="flow-handle" />
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
        <Handle type="source" position={Position.Bottom} className="flow-handle" />
      </button>
    );
  }
  return (
    <button type="button" className={`graph-node flow-graph-node ${isLoop ? 'compound-loop' : ''} ${step.status}`} onClick={() => data.onSelect(step)}>
      <Handle type="target" position={Position.Top} className="flow-handle" />
      <div className="graph-node-topline">
        <div className="graph-node-id">{isLoop ? '↻ loop' : graphShortId(step.id)}</div>
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
      {isLoop && data.onToggleExpand && (
        <span
          role="button"
          tabIndex={0}
          className="graph-node-action"
          onClick={(event) => {
            event.stopPropagation();
            data.onToggleExpand?.(step.id);
          }}
          onKeyDown={(event) => {
            if (event.key !== 'Enter' && event.key !== ' ') return;
            event.preventDefault();
            event.stopPropagation();
            data.onToggleExpand?.(step.id);
          }}
        >
          {data.expanded ? 'Collapse loop' : 'Expand loop'}
        </span>
      )}
      <Handle type="source" position={Position.Bottom} className="flow-handle" />
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

// ─── Layered DAG Layout ──────────────────────────────────────────────────────

const TREE_NODE_W = 300;
const TREE_NODE_H = 200;
const TREE_V_GAP = 80;
const TREE_H_GAP = 50;
const TREE_LOOP_INDENT = 36;
const TREE_LOOP_H_GAP = 16;
const TREE_PAD_X = 40;
const TREE_PAD_Y = 60;
const TREE_PAD_BOTTOM = 40;

function estimateLines(text: string, containerWidth: number, fontSizePx: number): number {
  if (!text) return 1;
  const avgCharWidth = fontSizePx * 0.6;
  const charsPerLine = Math.max(Math.floor(containerWidth / avgCharWidth), 1);
  return Math.max(1, Math.ceil(text.length / charsPerLine));
}

function graphStepNodeHeight(step: FormulaDashboardStep): number {
  const paddingV = 32;
  const gap = 10;
  const contentWidth = 268;

  const titleLines = Math.min(estimateLines(step.title || 't', contentWidth, 16), 2);
  const titleH = Math.max(titleLines, 1) * 22;

  const desc = step.description || step.notes || 'Structured execution step in the formula pipeline.';
  const descLines = Math.min(estimateLines(desc, contentWidth, 13), 3);
  const descH = Math.max(descLines, 1) * 19;

  const coreH = 26 + titleH + descH + 24;
  const coreGaps = 3 * gap;

  let extrasH = 0;
  let extraGaps = 0;

  if (step.loop?.body?.length) {
    extrasH += 28;
    extraGaps++;
  }
  if (step.activities?.length) {
    extrasH += 27;
    extraGaps++;
  }
  if (step.execution === 'script' || step.type === 'script') {
    extrasH += 6;
  }

  const metaText = `${step.agent || 'default agent'} · ${step.depends_on?.length || 0} deps`;
  if (metaText.length > 44) {
    extrasH += 18;
  }

  const total = paddingV + coreGaps + extraGaps * gap + coreH + extrasH;
  return Math.max(total, 180);
}

function graphLoopBodyNodeHeight(step: FormulaDashboardStep): number {
  const paddingV = 30;
  const gap = 10;

  let metaH = 24;
  if (step.input_ctx?.length) metaH += 16;

  const items = 3;
  const coreGaps = (items - 1) * gap;
  let coreH = 26 + 22 + metaH;

  if (step.metadata?.iteration) {
    coreH += 28 + gap;
  }

  return Math.max(paddingV + coreGaps + coreH, 150);
}

function computeGraphLayout(snapshot: FormulaDashboardSnapshot, onSelect: (step: FormulaDashboardStep) => void, expandedLoopIDs: Set<string>, onToggleExpand: (stepID: string) => void) {
  const stepMap = new Map(snapshot.steps.map(s => [s.id, s]));
  const stepHeight = new Map<string, number>();
  for (const step of snapshot.steps) {
    stepHeight.set(step.id, graphStepNodeHeight(step));
  }

  const outEdges = new Map<string, string[]>();
  const inEdges = new Map<string, string[]>();
  for (const edge of snapshot.edges) {
    const existing = outEdges.get(edge.from);
    if (existing) existing.push(edge.to);
    else outEdges.set(edge.from, [edge.to]);

    const inExisting = inEdges.get(edge.to);
    if (inExisting) inExisting.push(edge.from);
    else inEdges.set(edge.to, [edge.from]);
  }

  // Group by depth (layer)
  const layers = new Map<number, string[]>();
  for (const step of snapshot.steps) {
    const depth = step.depth || 0;
    const existing = layers.get(depth);
    if (existing) existing.push(step.id);
    else layers.set(depth, [step.id]);
  }
  const sortedDepths = Array.from(layers.keys()).sort((a, b) => a - b);
  for (const depth of sortedDepths) {
    layers.get(depth)!.sort((a, b) => {
      const sa = stepMap.get(a);
      const sb = stepMap.get(b);
      return (sa?.index ?? 0) - (sb?.index ?? 0);
    });
  }

  // Assign x positions: each node's x is the average of its parents' x positions
  // First pass: compute positions layer by layer
  const nodeX = new Map<string, number>();
  const nodeY = new Map<string, number>();

  // Root layer (no parents): evenly spaced
  const rootLayer = sortedDepths[0] || [];
  const rootWidth = rootLayer.length * TREE_NODE_W + (rootLayer.length - 1) * TREE_H_GAP;
  let startX = -rootWidth / 2;
  for (let i = 0; i < rootLayer.length; i++) {
    nodeX.set(rootLayer[i], startX + TREE_NODE_W / 2 + i * (TREE_NODE_W + TREE_H_GAP));
    nodeY.set(rootLayer[i], TREE_PAD_Y);
  }

  // Subsequent layers: position based on parents
  for (let di = 1; di < sortedDepths.length; di++) {
    const depth = sortedDepths[di];
    const layer = layers.get(depth) || [];
    const prevDepth = sortedDepths[di - 1];
    const prevLayer = layers.get(prevDepth) || [];

    // For each node in this layer, compute ideal x from parents
    const nodeIdealX = new Map<string, number>();
    const fanInNodes = new Set<string>();

    for (const nodeID of layer) {
      const parents = (inEdges.get(nodeID) || []).filter(p => nodeX.has(p));
      if (parents.length === 0) {
        // No parents in earlier layers: position after all previous layer nodes
        const prevMaxX = prevLayer.length > 0
          ? Math.max(...prevLayer.map(id => nodeX.get(id) || 0))
          : 0;
        nodeIdealX.set(nodeID, prevMaxX + TREE_NODE_W + TREE_H_GAP);
      } else if (parents.length === 1) {
        nodeIdealX.set(nodeID, nodeX.get(parents[0])!);
      } else {
        // Fan-in: center between parents
        const xs = parents.map(p => nodeX.get(p)!);
        const avg = xs.reduce((a, b) => a + b, 0) / xs.length;
        nodeIdealX.set(nodeID, avg);
        fanInNodes.add(nodeID);
      }
    }

    // Sort nodes by ideal x for stable ordering
    const sorted = [...layer].sort((a, b) => (nodeIdealX.get(a) || 0) - (nodeIdealX.get(b) || 0));

    // Resolve overlaps: if two nodes are too close, push them apart
    const minSpacing = TREE_NODE_W + TREE_H_GAP;
    for (let i = 1; i < sorted.length; i++) {
      const prev = sorted[i - 1];
      const curr = sorted[i];
      const prevX = nodeX.get(prev) ?? nodeIdealX.get(prev) ?? 0;
      let currX = nodeX.get(curr) ?? nodeIdealX.get(curr) ?? 0;
      if (currX - prevX < minSpacing) {
        currX = prevX + minSpacing;
        nodeIdealX.set(curr, currX);
      }
      nodeX.set(curr, currX);
    }

    // First node in layer
    if (sorted.length > 0) {
      const first = sorted[0];
      if (!nodeX.has(first)) {
        nodeX.set(first, nodeIdealX.get(first) ?? 0);
      }
    }

    // Assign y
    const y = TREE_PAD_Y + (di) * (TREE_NODE_H + TREE_V_GAP);
    for (const nodeID of layer) {
      nodeY.set(nodeID, y);
      if (!nodeX.has(nodeID)) {
        nodeX.set(nodeID, nodeIdealX.get(nodeID) ?? 0);
      }
    }
  }

  // Build nodes and edges
  const nodes: Node<StepNodeData | LoopGroupNodeData>[] = [];
  const edges: Edge[] = [];
  const stepStatusMap = new Map(snapshot.steps.map(step => [step.id, step.status]));
  const statusFor = (id: string) => stepStatusMap.get(id) || 'pending';
  let maxX = 0;
  let maxY = 0;

  for (const step of snapshot.steps) {
    const x = nodeX.get(step.id) ?? 0;
    const y = nodeY.get(step.id) ?? 0;
    const h = stepHeight.get(step.id) ?? TREE_NODE_H;

    nodes.push({
      id: step.id,
      type: 'step',
      data: { step, kind: 'step', onSelect, onToggleExpand, expanded: expandedLoopIDs.has(step.id) },
      position: { x: x - TREE_NODE_W / 2, y },
      style: { width: TREE_NODE_W, height: h },
    });

    maxX = Math.max(maxX, x + TREE_NODE_W / 2);
    maxY = Math.max(maxY, y + h);

    // Loop body
    if (step.loop?.body?.length && expandedLoopIDs.has(step.id)) {
      const bodySteps = step.loop.body.map((body, i) => loopBodyStep(step, body, i));
      const bodyHeights = bodySteps.map(graphLoopBodyNodeHeight);
      const bodyGap = TREE_LOOP_H_GAP;

      const totalBodyW = bodySteps.length * TREE_NODE_W + (bodySteps.length - 1) * bodyGap;
      let bodyX = x - totalBodyW / 2 + TREE_LOOP_INDENT;
      const bodyY = y + h + 40;

      for (let i = 0; i < bodySteps.length; i++) {
        const bodyStep = bodySteps[i];
        const bodyNodeID = loopBodyGraphID(step.id, step.loop.body[i].id);
        const bodyH = bodyHeights[i];

        nodes.push({
          id: bodyNodeID,
          type: 'step',
          data: { step: bodyStep, kind: 'loop-body', parentStep: step, body: step.loop.body[i], onSelect },
          position: { x: bodyX - TREE_NODE_W / 2, y: bodyY },
          style: { width: TREE_NODE_W, height: bodyH },
        });

        maxX = Math.max(maxX, bodyX + TREE_NODE_W / 2);
        maxY = Math.max(maxY, bodyY + bodyH);

        bodyX += TREE_NODE_W + bodyGap;
      }

      // Loop group container
      if (bodySteps.length > 0) {
        const firstBodyY = bodyY;
        const lastBodyY = bodyY + bodyHeights[bodyHeights.length - 1];
        const groupHeight = lastBodyY - firstBodyY + 40;
        nodes.push({
          id: `${step.id}__loop_group`,
          type: 'loopGroup',
          data: { step, bodyCount: bodySteps.length },
          position: { x: x - totalBodyW / 2 + TREE_LOOP_INDENT - 18, y: firstBodyY - 28 },
          style: { width: totalBodyW + 36, height: groupHeight },
          selectable: false,
          draggable: false,
          zIndex: -1,
        });
      }

      // Loop body edges
      for (let i = 0; i < step.loop.body.length; i++) {
        const body = step.loop.body[i];
        const target = loopBodyGraphID(step.id, body.id);
        const bodyIDs = new Set(step.loop.body.map(b => b.id));
        const deps = body.depends_on?.filter(d => bodyIDs.has(d)) || [];
        const sources = deps.length
          ? deps.map(d => loopBodyGraphID(step.id, d))
          : i === 0
            ? [step.id]
            : [loopBodyGraphID(step.id, step.loop.body[i - 1].id)];
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
  }

  // Main edges
  for (const edge of snapshot.edges) {
    if (!nodeX.has(edge.from) || !nodeX.has(edge.to)) continue;
    const targetStatus = statusFor(edge.to);
    const sourceStatus = statusFor(edge.from);
    edges.push({
      id: `${edge.from}-${edge.to}`,
      source: edge.from,
      target: edge.to,
      type: 'smoothstep',
      animated: targetStatus === 'running',
      markerEnd: { type: MarkerType.ArrowClosed, color: edgeMarkerColor(targetStatus) },
      className: edgeVisualClass(sourceStatus, targetStatus, edge.type || 'default'),
    });
  }

  const width = Math.max(720, maxX + TREE_PAD_X * 2);
  const height = Math.max(420, maxY + TREE_PAD_BOTTOM);

  return {
    widths: { nodeWidth: TREE_NODE_W, nodeHeight: TREE_NODE_H, colGap: TREE_H_GAP, paddingX: TREE_PAD_X },
    depths: sortedDepths,
    nodes,
    edges,
    width,
    height,
  };
}

export function GraphPanel({ snapshot, onSelect }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void }) {
  const [expandedLoopIDs, setExpandedLoopIDs] = useState<Set<string>>(() => new Set());
  const toggleLoop = useCallback((stepID: string) => {
    setExpandedLoopIDs(current => {
      const next = new Set(current);
      if (next.has(stepID)) next.delete(stepID);
      else next.add(stepID);
      return next;
    });
  }, []);
  const layout = useMemo(() => computeGraphLayout(snapshot, onSelect, expandedLoopIDs, toggleLoop), [snapshot, onSelect, expandedLoopIDs, toggleLoop]);
  const running = snapshot.steps.find(step => step.status === 'running');
  const loopSteps = snapshot.steps.filter(step => !!step.loop?.body?.length).length;
  const expandedLoopCount = expandedLoopIDs.size;

  if (!snapshot.steps.length) {
    return <Empty description="No executable steps" />;
  }

  return (
    <div className="graph-panel">
      <div className="graph-header">
        <div>
          <h3>Execution graph</h3>
          <p>Overview-first workflow map. Loops stay collapsed until you expand them, with details available in the inspector.</p>
          <div className="graph-header-metrics">
            <span>{layout.nodes.length} nodes</span>
            <span>{layout.edges.length} edges</span>
            {!!loopSteps && <span>{loopSteps} loop{loopSteps === 1 ? '' : 's'} · {expandedLoopCount} expanded</span>}
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
