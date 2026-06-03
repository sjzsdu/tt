import { useCallback, useMemo, useState } from 'react';
import { Empty } from 'antd';
import { ReactFlow, Background, Handle, MarkerType, MiniMap, Position, type Edge, type Node, type NodeProps } from '@xyflow/react';
import type { FormulaDashboardLoopBody, FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { activityShortId, graphShortId, statusLabel } from '../../utils/status';
import { latestLoopActivity, loopActivityBodyID, loopActivityIteration, loopActivitySummary, stepExecutionKind, stepExecutionLabel } from '../../utils/steps';

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
  const executionKind = stepExecutionKind(step);
  if (isLoopBody) {
    const parent = data.parentStep || step;
    return (
      <button type="button" className={`graph-node flow-graph-node loop-body-node ${step.status}`} onClick={() => data.onSelect(parent)}>
        <Handle type="target" position={Position.Top} className="flow-handle" />
        <div className="graph-node-topline">
          <div className="graph-node-id">body · {graphShortId(step.id)}</div>
          <span className={`graph-node-kind ${executionKind}`}>{stepExecutionLabel(executionKind)}</span>
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
    <button type="button" className={`graph-node flow-graph-node ${isLoop ? 'compound-loop' : ''} ${data.expanded ? 'expanded' : ''} ${step.status}`} onClick={() => data.onSelect(step)}>
      <Handle type="target" position={Position.Left} className="flow-handle" />
        <div className="graph-node-topline">
          <div className="graph-node-id">{isLoop ? '↻ loop' : graphShortId(step.id)}</div>
          <span className={`graph-node-kind ${executionKind}`}>{stepExecutionLabel(executionKind)}</span>
          <span className={`graph-node-state ${step.status}`}>{step.status}</span>
        </div>
      <strong>{step.title}</strong>
      <div className="graph-node-meta">
        {isLoop && <span>loop · {step.loop?.body?.length || 0}</span>}
        {step.agent && <span>agent · {step.agent}</span>}
        {step.human_input_request && <span>input required</span>}
        {!!step.depends_on?.length && <span>{step.depends_on.length} deps</span>}
        {!!step.activities?.length && <span>{step.activities.length} events</span>}
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
          {data.expanded ? '▾ Expanded loop' : '▸ Expand loop'}
        </span>
      )}
      {isLoop && data.expanded && <Handle id="loop-expand" type="source" position={Position.Bottom} className="flow-handle loop-expand-handle" />}
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
  const loopMeta = [
    step.loop?.parallel ? 'parallel' : 'serial',
    step.loop?.max_concurrency ? `max concurrency ${step.loop.max_concurrency}` : '',
    step.loop?.for_each ? `foreach ${step.loop.for_each}` : '',
  ].filter(Boolean);
  return (
    <div className={`loop-group-node ${step.status}`}>
      <div className="loop-group-header">
        <div>
          <div className="loop-group-title">↻ Expanded loop body</div>
          <strong>{step.title}</strong>
        </div>
        <span className="loop-group-badge">{data.bodyCount} step{data.bodyCount === 1 ? '' : 's'}</span>
      </div>
      <div className="loop-group-meta">
        <span>{summary}</span>
        {loopMeta.map(item => <span key={item}>{item}</span>)}
      </div>
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
const TREE_RANK_GAP = TREE_H_GAP * 3;
const LOOP_BODY_NODE_W = 260;
const TREE_LOOP_H_GAP = 28;
const TREE_LOOP_V_GAP = 74;
const TREE_LOOP_CONTAINER_GAP = 46;
const TREE_LOOP_CONTAINER_PAD_X = 28;
const TREE_LOOP_CONTAINER_PAD_Y = 20;
const TREE_LOOP_CONTAINER_HEADER_H = 78;
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

type LoopBodyLayout = {
  bodySteps: FormulaDashboardStep[];
  bodyEdges: Array<{ from: string; to: string; synthetic?: boolean }>;
  bodyHeights: number[];
  bodyPositions: Map<string, { x: number; y: number }>;
  containerWidth: number;
  containerHeight: number;
};

function computeLoopBodyLayout(step: FormulaDashboardStep): LoopBodyLayout | null {
  if (!step.loop?.body?.length) return null;

  const bodySteps = step.loop.body.map((body, i) => loopBodyStep(step, body, i));
  const bodyIDs = new Set(step.loop.body.map(body => body.id));
  const bodyEdges: Array<{ from: string; to: string; synthetic?: boolean }> = [];
  for (let i = 0; i < step.loop.body.length; i++) {
    const body = step.loop.body[i];
    const deps = body.depends_on?.filter(dep => bodyIDs.has(dep)) || [];
    if (deps.length) {
      for (const dep of deps) bodyEdges.push({ from: dep, to: body.id });
    } else if (i > 0) {
      bodyEdges.push({ from: step.loop.body[i - 1].id, to: body.id, synthetic: true });
    }
  }

  const bodyParents = new Map<string, string[]>();
  for (const edge of bodyEdges) {
    bodyParents.set(edge.to, [...(bodyParents.get(edge.to) || []), edge.from]);
  }
  const bodyRankMemo = new Map<string, number>();
  const computeBodyRank = (bodyID: string, visiting = new Set<string>()): number => {
    if (bodyRankMemo.has(bodyID)) return bodyRankMemo.get(bodyID)!;
    if (visiting.has(bodyID)) return 0;
    visiting.add(bodyID);
    const parents = bodyParents.get(bodyID) || [];
    const rank = parents.length ? Math.max(...parents.map(parentID => computeBodyRank(parentID, visiting) + 1)) : 0;
    visiting.delete(bodyID);
    bodyRankMemo.set(bodyID, rank);
    return rank;
  };

  const bodyLayers = new Map<number, number[]>();
  for (let i = 0; i < step.loop.body.length; i++) {
    const rank = computeBodyRank(step.loop.body[i].id);
    bodyLayers.set(rank, [...(bodyLayers.get(rank) || []), i]);
  }
  const bodyRanks = Array.from(bodyLayers.keys()).sort((a, b) => a - b);
  for (const rank of bodyRanks) {
    bodyLayers.get(rank)!.sort((a, b) => {
      const bodyA = step.loop!.body![a];
      const bodyB = step.loop!.body![b];
      return (bodyParents.get(bodyA.id)?.length || 0) - (bodyParents.get(bodyB.id)?.length || 0) || a - b;
    });
  }

  const bodyHeights = bodySteps.map(graphLoopBodyNodeHeight);
  const maxRankWidth = Math.max(
    LOOP_BODY_NODE_W,
    ...bodyRanks.map(rank => {
      const count = bodyLayers.get(rank)?.length || 0;
      return count * LOOP_BODY_NODE_W + Math.max(0, count - 1) * TREE_LOOP_H_GAP;
    }),
  );
  const containerWidth = Math.max(TREE_NODE_W + TREE_LOOP_CONTAINER_PAD_X * 2, maxRankWidth + TREE_LOOP_CONTAINER_PAD_X * 2);
  const bodyPositions = new Map<string, { x: number; y: number }>();
  let rankY = TREE_LOOP_CONTAINER_HEADER_H + TREE_LOOP_CONTAINER_PAD_Y;
  for (const rank of bodyRanks) {
    const indexes = bodyLayers.get(rank) || [];
    const rankW = indexes.length * LOOP_BODY_NODE_W + Math.max(0, indexes.length - 1) * TREE_LOOP_H_GAP;
    let bodyX = (containerWidth - rankW) / 2;
    const rankH = Math.max(...indexes.map(index => bodyHeights[index]));
    for (const index of indexes) {
      const body = step.loop.body[index];
      bodyPositions.set(body.id, { x: bodyX, y: rankY });
      bodyX += LOOP_BODY_NODE_W + TREE_LOOP_H_GAP;
    }
    rankY += rankH + TREE_LOOP_V_GAP;
  }

  const bodyBottom = Math.max(
    TREE_LOOP_CONTAINER_HEADER_H + TREE_LOOP_CONTAINER_PAD_Y + 120,
    ...step.loop.body.map((body, index) => (bodyPositions.get(body.id)?.y || 0) + bodyHeights[index]),
  );
  const containerHeight = bodyBottom + TREE_LOOP_CONTAINER_PAD_Y;

  return {
    bodySteps,
    bodyEdges,
    bodyHeights,
    bodyPositions,
    containerWidth,
    containerHeight,
  };
}

function computeGraphLayout(snapshot: FormulaDashboardSnapshot, onSelect: (step: FormulaDashboardStep) => void, expandedLoopIDs: Set<string>, onToggleExpand: (stepID: string) => void) {
  const stepMap = new Map(snapshot.steps.map(s => [s.id, s]));
  const graphEdges = [...snapshot.edges];
  const graphEdgeIDs = new Set(graphEdges.map(edge => `${edge.from}\u0000${edge.to}`));
  for (const step of snapshot.steps) {
    for (const dep of step.depends_on || []) {
      if (!stepMap.has(dep)) continue;
      const edgeID = `${dep}\u0000${step.id}`;
      if (graphEdgeIDs.has(edgeID)) continue;
      graphEdgeIDs.add(edgeID);
      graphEdges.push({ from: dep, to: step.id, type: 'depends_on' });
    }
  }
  const stepHeight = new Map<string, number>();
  for (const step of snapshot.steps) {
    stepHeight.set(step.id, graphStepNodeHeight(step));
  }

  const outEdges = new Map<string, string[]>();
  const inEdges = new Map<string, string[]>();
  for (const edge of graphEdges) {
    const existing = outEdges.get(edge.from);
    if (existing) existing.push(edge.to);
    else outEdges.set(edge.from, [edge.to]);

    const inExisting = inEdges.get(edge.to);
    if (inExisting) inExisting.push(edge.from);
    else inEdges.set(edge.to, [edge.from]);
  }

  // Compute DAG ranks from actual dependency edges, not from backend-provided depth.
  const rankMemo = new Map<string, number>();
  const computeRank = (id: string, visiting = new Set<string>()): number => {
    if (rankMemo.has(id)) return rankMemo.get(id)!;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const parents = (inEdges.get(id) || []).filter(parentID => stepMap.has(parentID));
    const rank = parents.length
      ? Math.max(...parents.map(parentID => computeRank(parentID, visiting) + 1))
      : 0;
    visiting.delete(id);
    rankMemo.set(id, rank);
    return rank;
  };

  const layers = new Map<number, string[]>();
  for (const step of snapshot.steps) {
    const rank = computeRank(step.id);
    const existing = layers.get(rank);
    if (existing) existing.push(step.id);
    else layers.set(rank, [step.id]);
  }
  const sortedRanks = Array.from(layers.keys()).sort((a, b) => a - b);

  for (const rank of sortedRanks) {
    layers.get(rank)!.sort((a, b) => {
      const sa = stepMap.get(a);
      const sb = stepMap.get(b);
      return (sa?.index ?? 0) - (sb?.index ?? 0);
    });
  }

  const layerIndex = (rank: number) => {
    const index = new Map<string, number>();
    for (const [i, id] of (layers.get(rank) || []).entries()) index.set(id, i);
    return index;
  };
  const averageNeighborIndex = (ids: string[], index: Map<string, number>, fallback: number) => {
    const values = ids.map(id => index.get(id)).filter((value): value is number => value !== undefined);
    return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : fallback;
  };

  // Barycenter sweeps reduce crossings for fan-in/fan-out edges.
  for (let pass = 0; pass < 4; pass++) {
    for (let ri = 1; ri < sortedRanks.length; ri++) {
      const rank = sortedRanks[ri];
      const prevIndex = layerIndex(sortedRanks[ri - 1]);
      layers.get(rank)!.sort((a, b) => {
        const ai = averageNeighborIndex(inEdges.get(a) || [], prevIndex, stepMap.get(a)?.index ?? 0);
        const bi = averageNeighborIndex(inEdges.get(b) || [], prevIndex, stepMap.get(b)?.index ?? 0);
        return ai - bi || (stepMap.get(a)?.index ?? 0) - (stepMap.get(b)?.index ?? 0);
      });
    }
    for (let ri = sortedRanks.length - 2; ri >= 0; ri--) {
      const rank = sortedRanks[ri];
      const nextIndex = layerIndex(sortedRanks[ri + 1]);
      layers.get(rank)!.sort((a, b) => {
        const ai = averageNeighborIndex(outEdges.get(a) || [], nextIndex, stepMap.get(a)?.index ?? 0);
        const bi = averageNeighborIndex(outEdges.get(b) || [], nextIndex, stepMap.get(b)?.index ?? 0);
        return ai - bi || (stepMap.get(a)?.index ?? 0) - (stepMap.get(b)?.index ?? 0);
      });
    }
  }

  const loopLayouts = new Map<string, LoopBodyLayout>();
  for (const step of snapshot.steps) {
    if (!expandedLoopIDs.has(step.id)) continue;
    const layout = computeLoopBodyLayout(step);
    if (layout) loopLayouts.set(step.id, layout);
  }

  const footprintHeight = (step: FormulaDashboardStep) => {
    const h = stepHeight.get(step.id) ?? TREE_NODE_H;
    const loopLayout = loopLayouts.get(step.id);
    if (loopLayout) return h + TREE_LOOP_CONTAINER_GAP + loopLayout.containerHeight;
    return h;
  };
  const footprintWidth = (step: FormulaDashboardStep) => {
    const loopLayout = loopLayouts.get(step.id);
    return Math.max(TREE_NODE_W, loopLayout?.containerWidth || TREE_NODE_W);
  };

  const rankWidths = new Map<number, number>();
  for (const rank of sortedRanks) {
    const layer = layers.get(rank) || [];
    const width = layer.reduce((max, nodeID) => {
      const step = stepMap.get(nodeID);
      return step ? Math.max(max, footprintWidth(step)) : max;
    }, TREE_NODE_W);
    rankWidths.set(rank, width);
  }

  const rankCenters = new Map<number, number>();
  let rankX = TREE_PAD_X;
  for (const rank of sortedRanks) {
    const rankWidth = rankWidths.get(rank) || TREE_NODE_W;
    rankCenters.set(rank, rankX + rankWidth / 2);
    rankX += rankWidth + TREE_RANK_GAP;
  }

  const nodeX = new Map<string, number>();
  const nodeY = new Map<string, number>();
  for (const rank of sortedRanks) {
    const layer = layers.get(rank) || [];
    let y = TREE_PAD_Y;
    for (const nodeID of layer) {
      const step = stepMap.get(nodeID);
      if (!step) continue;
      nodeX.set(nodeID, rankCenters.get(rank) || (TREE_PAD_X + TREE_NODE_W / 2));
      nodeY.set(nodeID, y);
      y += footprintHeight(step) + TREE_V_GAP;
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

    // Expanded loop body is rendered as an embedded subgraph below the parent loop node.
    const loopLayout = loopLayouts.get(step.id);
    if (loopLayout && step.loop?.body?.length) {
      for (const bodyStep of loopLayout.bodySteps) {
        stepStatusMap.set(bodyStep.id, bodyStep.status);
      }
      const containerLeft = x - loopLayout.containerWidth / 2;
      const containerTop = y + h + TREE_LOOP_CONTAINER_GAP;

      // Loop group container
      nodes.push({
        id: `${step.id}__loop_group`,
        type: 'loopGroup',
        data: { step, bodyCount: loopLayout.bodySteps.length },
        position: { x: containerLeft, y: containerTop },
        style: { width: loopLayout.containerWidth, height: loopLayout.containerHeight },
        selectable: false,
        draggable: false,
        zIndex: -1,
      });
      maxX = Math.max(maxX, containerLeft + loopLayout.containerWidth);
      maxY = Math.max(maxY, containerTop + loopLayout.containerHeight);

      for (let i = 0; i < loopLayout.bodySteps.length; i++) {
        const bodyStep = loopLayout.bodySteps[i];
        const bodyNodeID = loopBodyGraphID(step.id, step.loop.body[i].id);
        const pos = loopLayout.bodyPositions.get(step.loop.body[i].id) || { x: TREE_LOOP_CONTAINER_PAD_X, y: TREE_LOOP_CONTAINER_HEADER_H };
        const bodyH = loopLayout.bodyHeights[i];

        nodes.push({
          id: bodyNodeID,
          type: 'step',
          data: { step: bodyStep, kind: 'loop-body', parentStep: step, body: step.loop.body[i], onSelect },
          position: { x: containerLeft + pos.x, y: containerTop + pos.y },
          style: { width: LOOP_BODY_NODE_W, height: bodyH },
          zIndex: 2,
        });

        maxX = Math.max(maxX, containerLeft + pos.x + LOOP_BODY_NODE_W);
        maxY = Math.max(maxY, containerTop + pos.y + bodyH);
      }

      // Loop body edges
      const firstBodyID = step.loop.body[0]?.id;
      if (firstBodyID) {
        edges.push({
          id: `${step.id}-${loopBodyGraphID(step.id, firstBodyID)}`,
          source: step.id,
          target: loopBodyGraphID(step.id, firstBodyID),
          sourceHandle: 'loop-expand',
          type: 'smoothstep',
          animated: statusFor(loopBodyGraphID(step.id, firstBodyID)) === 'running',
          markerEnd: { type: MarkerType.ArrowClosed, color: edgeMarkerColor(statusFor(loopBodyGraphID(step.id, firstBodyID))) },
          className: edgeVisualClass(statusFor(step.id), statusFor(loopBodyGraphID(step.id, firstBodyID)), 'loop-expand-edge'),
        });
      }
      for (const edge of loopLayout.bodyEdges) {
        const source = loopBodyGraphID(step.id, edge.from);
        const target = loopBodyGraphID(step.id, edge.to);
        const targetStatus = statusFor(target);
        const sourceStatus = statusFor(source);
        edges.push({
          id: `${source}-${target}`,
          source,
          target,
          type: 'smoothstep',
          animated: targetStatus === 'running',
          markerEnd: { type: MarkerType.ArrowClosed, color: edgeMarkerColor(targetStatus) },
          className: edgeVisualClass(sourceStatus, targetStatus, `loop-body-edge ${edge.synthetic ? 'synthetic' : 'dependency'}`),
        });
      }
    }
  }

  // Main edges
  for (const edge of graphEdges) {
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
    depths: sortedRanks,
    nodes,
    edges,
    width,
    height,
  };
}

export function GraphPanel({ snapshot, onSelect, theme }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void; theme: 'light' | 'dark' }) {
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
          <Background color={theme === 'dark' ? 'rgba(125, 211, 252, 0.18)' : 'rgba(100, 116, 139, 0.18)'} gap={28} size={1} />
          <MiniMap
            pannable
            zoomable
            nodeStrokeWidth={3}
            bgColor={theme === 'dark' ? 'rgba(10, 18, 32, 0.96)' : 'rgba(255, 255, 255, 0.96)'}
            maskColor={theme === 'dark' ? 'rgba(125, 211, 252, 0.12)' : 'rgba(59, 130, 246, 0.10)'}
            nodeColor={theme === 'dark' ? 'rgba(191, 219, 254, 0.92)' : 'rgba(148, 163, 184, 0.92)'}
            nodeStrokeColor={theme === 'dark' ? 'rgba(125, 211, 252, 0.95)' : 'rgba(59, 130, 246, 0.92)'}
            className="flow-minimap"
          />
        </ReactFlow>
      </div>
    </div>
  );
}
