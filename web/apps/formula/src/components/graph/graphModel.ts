import type { FormulaDashboardLoopBody, FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';


function loopActivityIteration(activity?: { step_id: string }) {
  return activity?.step_id.match(/\.iter(\d+)\./)?.[1] || '';
}

function loopActivityBodyID(activity?: { step_id: string }) {
  return activity?.step_id.match(/\.iter\d+\.([^.]*)$/)?.[1] || '';
}

export type StepNodeData = {
  step: FormulaDashboardStep;
  kind?: 'step' | 'loop-body';
  parentStep?: FormulaDashboardStep;
  body?: FormulaDashboardLoopBody;
  expanded?: boolean;
  layoutRank?: number;
  layoutOrder?: number;
  layoutX?: number;
  layoutY?: number;
};

export type LoopGroupNodeData = {
  kind: 'loop-group';
  step: FormulaDashboardStep;
  bodyCount: number;
  layoutRank?: number;
  layoutOrder?: number;
  layoutX?: number;
  layoutY?: number;
};

export type VariableNodeData = {
  kind: 'variable';
  key: string;
  consumers: string[];
  layoutRank?: number;
  layoutOrder?: number;
  layoutX?: number;
  layoutY?: number;
};

export type FormulaGraphNode = {
  id: string;
  data: StepNodeData | LoopGroupNodeData | VariableNodeData;
  combo?: string;
  style?: {
    x: number;
    y: number;
  };
};

export type FormulaGraphEdge = {
  id: string;
  source: string;
  target: string;
  data?: {
    status?: string;
    kind?: 'dependency' | 'loop-expand' | 'loop-sequence' | 'variable-consume';
    variable?: string;
    sourcePort?: string;
    targetPort?: string;
    laneOffset?: number;
  };
};

export type FormulaGraphCombo = {
  id: string;
  combo?: string;
  data: {
    step: FormulaDashboardStep;
    bodyCount: number;
    depth?: number;
  };
};


const SOURCE_PORTS = [
  'bottom-left-3',
  'bottom-left-2',
  'bottom-left-1',
  'bottom',
  'bottom-right-1',
  'bottom-right-2',
  'bottom-right-3',
];
const TARGET_PORTS = [
  'top-left-3',
  'top-left-2',
  'top-left-1',
  'top',
  'top-right-1',
  'top-right-2',
  'top-right-3',
];
const CENTER_PORT_INDEX = 3;
const PORT_INDEX_BY_TOTAL: Record<number, number[]> = {
  1: [3],
  2: [2, 4],
  3: [2, 3, 4],
  4: [1, 2, 4, 5],
  5: [1, 2, 3, 4, 5],
  6: [0, 1, 2, 4, 5, 6],
  7: [0, 1, 2, 3, 4, 5, 6],
};
const LANE_STEP = 12;
const NODE_X_GAP = 560;
const NODE_Y_GAP = 250;
const LOOP_BODY_NODE_X_GAP = 500;
const LOOP_BODY_NODE_Y_GAP = 230;
const LOOP_ISLAND_GAP = 120;
const STEP_NODE_WIDTH = 540;
const LOOP_BODY_NODE_WIDTH = 460;
const VARIABLE_NODE_WIDTH = 220;
const STEP_NODE_HEIGHT = 124;
const VARIABLE_NODE_HEIGHT = 54;

function variableNodeID(key: string) {
  return `var::${key.replace(/[^a-zA-Z0-9_.:-]/g, '_')}`;
}

function scopedVariableNodeID(scopeID: string, key: string) {
  return `${scopeID}__${variableNodeID(key)}`;
}

function consumedVariables(step: FormulaDashboardStep) {
  const refs = new Set<string>();
  for (const ref of step.var_refs || []) {
    const root = ref.trim().split('.')[0];
    if (root) refs.add(root);
  }
  return [...refs];
}

function portIndexForLane(index: number, total: number) {
  const indices = PORT_INDEX_BY_TOTAL[Math.min(total, SOURCE_PORTS.length)] || PORT_INDEX_BY_TOTAL[7];
  return indices[index % indices.length];
}

function portForLane(ports: string[], index: number, total: number) {
  return ports[portIndexForLane(index, total)];
}

function portOffset(index: number, total: number) {
  return (portIndexForLane(index, total) - CENTER_PORT_INDEX) * LANE_STEP;
}

function nodeOrder(nodes: FormulaGraphNode[]) {
  return new Map(nodes.map((node, index) => [node.id, index]));
}

function orderOf(order: Map<string, number>, id: string) {
  return order.get(id) ?? Number.MAX_SAFE_INTEGER;
}

function edgeEndpointMap(edges: FormulaGraphEdge[]) {
  const incoming = new Map<string, string[]>();
  const outgoing = new Map<string, string[]>();
  for (const edge of edges) {
    if (!outgoing.has(edge.source)) outgoing.set(edge.source, []);
    if (!incoming.has(edge.target)) incoming.set(edge.target, []);
    outgoing.get(edge.source)!.push(edge.target);
    incoming.get(edge.target)!.push(edge.source);
  }
  return { incoming, outgoing };
}

function computeRanks(nodes: FormulaGraphNode[], edges: FormulaGraphEdge[]) {
  const nodeIDs = new Set(nodes.map(node => node.id));
  const nodeByID = new Map(nodes.map(node => [node.id, node]));
  const originalOrder = nodeOrder(nodes);
  const rank = new Map(nodes.map(node => [node.id, 0]));
  const indegree = new Map(nodes.map(node => [node.id, 0]));
  const outgoing = new Map<string, string[]>();
  const layoutEdges = edges.filter(edge => edge.data?.kind !== 'variable-consume');

  for (const edge of layoutEdges) {
    if (!nodeIDs.has(edge.source) || !nodeIDs.has(edge.target)) continue;
    if (!outgoing.has(edge.source)) outgoing.set(edge.source, []);
    outgoing.get(edge.source)!.push(edge.target);
    indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1);
  }

  const queue = nodes
    .filter(node => (indegree.get(node.id) || 0) === 0)
    .sort((a, b) => orderOf(originalOrder, a.id) - orderOf(originalOrder, b.id));
  const seen = new Set<string>();

  while (queue.length) {
    const node = queue.shift()!;
    if (seen.has(node.id)) continue;
    seen.add(node.id);

    for (const target of outgoing.get(node.id) || []) {
      rank.set(target, Math.max(rank.get(target) || 0, (rank.get(node.id) || 0) + 1));
      indegree.set(target, Math.max(0, (indegree.get(target) || 0) - 1));
      if ((indegree.get(target) || 0) === 0) {
        const targetNode = nodeByID.get(target);
        if (targetNode) queue.push(targetNode);
        queue.sort((a, b) => orderOf(originalOrder, a.id) - orderOf(originalOrder, b.id));
      }
    }
  }

  // Keep cyclic or otherwise unresolved nodes stable, while still placing them
  // one layer after any already-ranked predecessor when possible.
  for (const edge of layoutEdges) {
    if (!nodeIDs.has(edge.source) || !nodeIDs.has(edge.target)) continue;
    if (seen.has(edge.target)) continue;
    rank.set(edge.target, Math.max(rank.get(edge.target) || 0, (rank.get(edge.source) || 0) + 1));
  }

  return rank;
}

function assignLayoutOrder(nodes: FormulaGraphNode[], edges: FormulaGraphEdge[]) {
  const originalOrder = nodeOrder(nodes);
  const rank = computeRanks(nodes, edges);
  if (nodes.some(node => node.data.kind === 'variable')) {
    for (const node of nodes) {
      rank.set(node.id, node.data.kind === 'variable' ? 0 : (rank.get(node.id) || 0) + 1);
    }
  }
  const { incoming, outgoing } = edgeEndpointMap(edges);
  const ranks = new Map<number, FormulaGraphNode[]>();

  for (const node of nodes) {
    const nodeRank = rank.get(node.id) || 0;
    if (!ranks.has(nodeRank)) ranks.set(nodeRank, []);
    ranks.get(nodeRank)!.push(node);
  }

  const rankKeys = [...ranks.keys()].sort((a, b) => a - b);
  for (const key of rankKeys) {
    ranks.get(key)!.sort((a, b) => orderOf(originalOrder, a.id) - orderOf(originalOrder, b.id));
  }

  const order = new Map<string, number>();
  const refreshOrder = () => {
    for (const key of rankKeys) {
      ranks.get(key)!.forEach((node, index) => order.set(node.id, index));
    }
  };
  const barycenter = (ids: string[] | undefined) => {
    const values = (ids || []).map(id => order.get(id)).filter((value): value is number => value !== undefined);
    if (!values.length) return Number.POSITIVE_INFINITY;
    return values.reduce((sum, value) => sum + value, 0) / values.length;
  };

  refreshOrder();
  for (let sweep = 0; sweep < 6; sweep++) {
    for (const key of rankKeys.slice(1)) {
      ranks.get(key)!.sort((a, b) => (
        barycenter(incoming.get(a.id)) - barycenter(incoming.get(b.id))
        || orderOf(originalOrder, a.id) - orderOf(originalOrder, b.id)
      ));
    }
    refreshOrder();

    for (const key of rankKeys.slice(0, -1).reverse()) {
      ranks.get(key)!.sort((a, b) => (
        barycenter(outgoing.get(a.id)) - barycenter(outgoing.get(b.id))
        || orderOf(originalOrder, a.id) - orderOf(originalOrder, b.id)
      ));
    }
    refreshOrder();
  }

  const totalCrossings = () => {
    let crossings = 0;
    const visibleEdges = edges.filter(edge => rank.has(edge.source) && rank.has(edge.target));

    for (let i = 0; i < visibleEdges.length; i++) {
      const a = visibleEdges[i];
      const aSourceRank = rank.get(a.source)!;
      const aTargetRank = rank.get(a.target)!;
      const aSourceOrder = order.get(a.source)!;
      const aTargetOrder = order.get(a.target)!;

      for (let j = i + 1; j < visibleEdges.length; j++) {
        const b = visibleEdges[j];
        if (a.source === b.source || a.target === b.target) continue;
        if (aSourceRank !== rank.get(b.source) || aTargetRank !== rank.get(b.target)) continue;

        const sourceDelta = aSourceOrder - order.get(b.source)!;
        const targetDelta = aTargetOrder - order.get(b.target)!;
        if (sourceDelta * targetDelta < 0) crossings++;
      }
    }

    return crossings;
  };

  refreshOrder();
  let bestCrossings = totalCrossings();
  for (let pass = 0; pass < 10; pass++) {
    let improved = false;

    for (const key of rankKeys) {
      const rankNodes = ranks.get(key)!;
      for (let index = 0; index < rankNodes.length - 1; index++) {
        [rankNodes[index], rankNodes[index + 1]] = [rankNodes[index + 1], rankNodes[index]];
        refreshOrder();
        const nextCrossings = totalCrossings();

        if (nextCrossings < bestCrossings) {
          bestCrossings = nextCrossings;
          improved = true;
        } else {
          [rankNodes[index], rankNodes[index + 1]] = [rankNodes[index + 1], rankNodes[index]];
          refreshOrder();
        }
      }
    }

    if (!improved) break;
  }

  for (const key of rankKeys) {
    ranks.get(key)!.forEach((node, index) => {
      node.data = { ...node.data, layoutRank: key, layoutOrder: index };
    });
  }

  nodes.sort((a, b) => (
    ((a.data.layoutRank || 0) - (b.data.layoutRank || 0))
    || ((a.data.layoutOrder || 0) - (b.data.layoutOrder || 0))
    || orderOf(originalOrder, a.id) - orderOf(originalOrder, b.id)
  ));
}

function assignLayoutPositions(nodes: FormulaGraphNode[], options: { xGap?: number; yGap?: number } = {}) {
  const xGap = options.xGap || NODE_X_GAP;
  const yGap = options.yGap || NODE_Y_GAP;
  const ranks = new Map<number, FormulaGraphNode[]>();
  for (const node of nodes) {
    const rank = node.data.layoutRank || 0;
    if (!ranks.has(rank)) ranks.set(rank, []);
    ranks.get(rank)!.push(node);
  }

  for (const rank of [...ranks.keys()].sort((a, b) => a - b)) {
    const rankNodes = ranks.get(rank)!.sort((a, b) => (a.data.layoutOrder || 0) - (b.data.layoutOrder || 0));
    const center = (rankNodes.length - 1) / 2;
    rankNodes.forEach((node, index) => {
      const x = (index - center) * xGap;
      const y = rank * yGap;
      node.data = { ...node.data, layoutX: x, layoutY: y };
      node.style = { ...(node.style || {}), x, y };
    });
  }
}

function applyEdgeLanes(edges: FormulaGraphEdge[], nodes: FormulaGraphNode[]) {
  const bySource = new Map<string, FormulaGraphEdge[]>();
  const byTarget = new Map<string, FormulaGraphEdge[]>();
  const order = nodeOrder(nodes);

  for (const edge of edges) {
    if (!bySource.has(edge.source)) bySource.set(edge.source, []);
    if (!byTarget.has(edge.target)) byTarget.set(edge.target, []);
    bySource.get(edge.source)!.push(edge);
    byTarget.get(edge.target)!.push(edge);
  }

  for (const sourceEdges of bySource.values()) {
    sourceEdges
      .sort((a, b) => orderOf(order, a.target) - orderOf(order, b.target) || a.id.localeCompare(b.id))
      .forEach((edge, index) => {
        edge.data = {
          ...edge.data,
          sourcePort: portForLane(SOURCE_PORTS, index, sourceEdges.length),
          laneOffset: (edge.data?.laneOffset || 0) + portOffset(index, sourceEdges.length),
        };
      });
  }

  for (const targetEdges of byTarget.values()) {
    targetEdges
      .sort((a, b) => orderOf(order, a.source) - orderOf(order, b.source) || a.id.localeCompare(b.id))
      .forEach((edge, index) => {
        edge.data = {
          ...edge.data,
          targetPort: portForLane(TARGET_PORTS, index, targetEdges.length),
          laneOffset: (edge.data?.laneOffset || 0) - portOffset(index, targetEdges.length),
        };
      });
  }
}

export function loopBodyGraphID(parentID: string, bodyID: string) {
  return `${parentID}__${bodyID}`;
}

export function loopBodyStep(parent: FormulaDashboardStep, body: FormulaDashboardLoopBody, index: number, activitySource = parent): FormulaDashboardStep {
  const bodyActivity = [...(activitySource.activities || [])]
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
    var_refs: body.var_refs,
    condition: body.condition,
    depends_on: body.depends_on,
    loop: body.loop,
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

export function materializeLoopBodySteps(parentStep: FormulaDashboardStep, parentNodeID = parentStep.id, activitySource = parentStep) {
  return (parentStep.loop?.body || []).map((body, index) => {
    const bodyStep = loopBodyStep(parentStep, body, index, activitySource);
    bodyStep.id = loopBodyGraphID(parentNodeID, body.id);
    bodyStep.depth = (parentStep.depth || 0) + 1;
    return bodyStep;
  });
}

export function computeLoopBodyGraphData(
  parentStep: FormulaDashboardStep,
  parentNodeID = parentStep.id,
  activitySource = parentStep,
  expandedLoopIDs: Set<string> = new Set(),
): { nodes: FormulaGraphNode[]; edges: FormulaGraphEdge[]; combos: FormulaGraphCombo[]; bodySteps: FormulaDashboardStep[] } {
  const bodySteps = materializeLoopBodySteps(parentStep, parentNodeID, activitySource);
  const bodyIDToNodeID = new Map(
    (parentStep.loop?.body || []).map(body => [body.id, loopBodyGraphID(parentNodeID, body.id)]),
  );
  const stepMap = new Map(bodySteps.map(step => [step.id, step]));
  const nodes: FormulaGraphNode[] = [];
  const edges: FormulaGraphEdge[] = [];
  const variableConsumers = new Map<string, string[]>();

  const addVariableConsumers = (step: FormulaDashboardStep, nodeID: string) => {
    for (const key of consumedVariables(step)) {
      if (!variableConsumers.has(key)) variableConsumers.set(key, []);
      variableConsumers.get(key)!.push(nodeID);
    }
  };

  for (let index = 0; index < bodySteps.length; index++) {
    const step = bodySteps[index];
    nodes.push({
      id: step.id,
      data: {
        step,
        kind: 'loop-body',
        parentStep,
        body: parentStep.loop?.body?.[index],
        expanded: expandedLoopIDs.has(step.id),
      },
    });

    addVariableConsumers(step, step.id);
  }

  const edgeIDs = new Set<string>();
  for (const step of bodySteps) {
    for (const dep of step.depends_on || []) {
      const sourceID = bodyIDToNodeID.get(dep) || dep;
      if (!stepMap.has(sourceID)) continue;
      const edgeID = `${sourceID}-${step.id}`;
      if (edgeIDs.has(edgeID)) continue;
      edgeIDs.add(edgeID);
      edges.push({
        id: edgeID,
        source: sourceID,
        target: step.id,
        data: { status: step.status, kind: 'dependency' },
      });
    }
  }

  for (const [key, consumers] of variableConsumers) {
    const variableID = scopedVariableNodeID(parentNodeID, key);
    nodes.push({
      id: variableID,
      data: {
        kind: 'variable',
        key,
        consumers,
      },
    });

    for (const consumerID of consumers) {
      edges.push({
        id: `${variableID}-${consumerID}`,
        source: variableID,
        target: consumerID,
        data: { kind: 'variable-consume', variable: key },
      });
    }
  }

  assignLayoutOrder(nodes, edges);
  assignLayoutPositions(nodes, { xGap: LOOP_BODY_NODE_X_GAP, yGap: LOOP_BODY_NODE_Y_GAP });
  applyEdgeLanes(edges, nodes);

  return { nodes, edges, combos: [], bodySteps };
}

function approximateNodeSize(node: FormulaGraphNode): [number, number] {
  if (node.data.kind === 'variable') return [VARIABLE_NODE_WIDTH, VARIABLE_NODE_HEIGHT];
  if (node.data.kind === 'loop-group') return [LOOP_BODY_NODE_WIDTH, STEP_NODE_HEIGHT];
  return [node.data.kind === 'loop-body' ? LOOP_BODY_NODE_WIDTH : STEP_NODE_WIDTH, STEP_NODE_HEIGHT];
}

function graphNodeBounds(nodes: FormulaGraphNode[]) {
  let minX = Number.POSITIVE_INFINITY;
  let minY = Number.POSITIVE_INFINITY;
  let maxX = Number.NEGATIVE_INFINITY;
  let maxY = Number.NEGATIVE_INFINITY;

  for (const node of nodes) {
    const [width, height] = approximateNodeSize(node);
    const x = node.style?.x ?? node.data.layoutX ?? 0;
    const y = node.style?.y ?? node.data.layoutY ?? 0;
    minX = Math.min(minX, x - width / 2);
    minY = Math.min(minY, y - height / 2);
    maxX = Math.max(maxX, x + width / 2);
    maxY = Math.max(maxY, y + height / 2);
  }

  if (!Number.isFinite(minX) || !Number.isFinite(minY) || !Number.isFinite(maxX) || !Number.isFinite(maxY)) {
    return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0 };
  }

  return { minX, minY, maxX, maxY, width: maxX - minX, height: maxY - minY };
}

function shiftGraphNodes(nodes: FormulaGraphNode[], dx: number, dy: number) {
  for (const node of nodes) {
    const x = (node.style?.x ?? node.data.layoutX ?? 0) + dx;
    const y = (node.style?.y ?? node.data.layoutY ?? 0) + dy;
    node.data = { ...node.data, layoutX: x, layoutY: y };
    node.style = { ...(node.style || {}), x, y };
  }
}

function placeLoopBodyGraphToRight(bodyNodes: FormulaGraphNode[], parentNode: FormulaGraphNode) {
  if (!bodyNodes.length) return;
  const bounds = graphNodeBounds(bodyNodes);
  const [parentWidth] = approximateNodeSize(parentNode);
  const parentX = parentNode.style?.x ?? parentNode.data.layoutX ?? 0;
  const parentY = parentNode.style?.y ?? parentNode.data.layoutY ?? 0;
  const bodyCenterX = (bounds.minX + bounds.maxX) / 2;
  const bodyCenterY = (bounds.minY + bounds.maxY) / 2;
  const targetCenterX = parentX + parentWidth / 2 + LOOP_ISLAND_GAP + bounds.width / 2;
  const targetCenterY = parentY;

  shiftGraphNodes(bodyNodes, targetCenterX - bodyCenterX, targetCenterY - bodyCenterY);
}

function loopBodyEntryNodeIDs(bodyGraph: { nodes: FormulaGraphNode[]; edges: FormulaGraphEdge[] }) {
  const dependencyTargets = new Set(
    bodyGraph.edges
      .filter(edge => edge.data?.kind !== 'variable-consume')
      .map(edge => edge.target),
  );
  const bodyNodeIDs = bodyGraph.nodes
    .filter(node => node.data.kind === 'loop-body')
    .map(node => node.id);
  const roots = bodyNodeIDs.filter(id => !dependencyTargets.has(id));
  return roots.length ? roots : bodyNodeIDs.slice(0, 1);
}

export function computeGraphData(
  snapshot: FormulaDashboardSnapshot,
  expandedLoopIDs: Set<string>,
): { nodes: FormulaGraphNode[]; edges: FormulaGraphEdge[]; combos: FormulaGraphCombo[] } {
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

  const nodes: FormulaGraphNode[] = [];
  const edges: FormulaGraphEdge[] = [];
  const combos: FormulaGraphCombo[] = [];
  const variableConsumers = new Map<string, string[]>();

  const addVariableConsumers = (step: FormulaDashboardStep, nodeID: string) => {
    for (const key of consumedVariables(step)) {
      if (!variableConsumers.has(key)) variableConsumers.set(key, []);
      variableConsumers.get(key)!.push(nodeID);
    }
  };

  for (const step of snapshot.steps) {
    nodes.push({
      id: step.id,
      data: {
        step,
        kind: 'step',
        expanded: expandedLoopIDs.has(step.id),
      },
    });

    addVariableConsumers(step, step.id);
  }

  for (const edge of graphEdges) {
    if (!stepMap.has(edge.from) || !stepMap.has(edge.to)) continue;
    edges.push({
      id: `${edge.from}-${edge.to}`,
      source: edge.from,
      target: edge.to,
      data: { status: stepMap.get(edge.to)?.status, kind: 'dependency' },
    });
  }

  for (const [key, consumers] of variableConsumers) {
    const variableID = variableNodeID(key);
    nodes.push({
      id: variableID,
      data: {
        kind: 'variable',
        key,
        consumers,
      },
    });

    for (const consumerID of consumers) {
      edges.push({
        id: `${variableID}-${consumerID}`,
        source: variableID,
        target: consumerID,
        data: { kind: 'variable-consume', variable: key },
      });
    }
  }

  assignLayoutOrder(nodes, edges);
  assignLayoutPositions(nodes);
  applyEdgeLanes(edges, nodes);

  const nodeByID = new Map(nodes.map(node => [node.id, node]));
  const appendedLoopIDs = new Set<string>();

  const appendExpandedLoopBodyGraph = (
    parentNodeID: string,
    parentStep: FormulaDashboardStep,
    activitySource: FormulaDashboardStep,
  ) => {
    if (!parentStep.loop?.body?.length || !expandedLoopIDs.has(parentNodeID) || appendedLoopIDs.has(parentNodeID)) return;
    const parentNode = nodeByID.get(parentNodeID);
    if (!parentNode) return;

    appendedLoopIDs.add(parentNodeID);
    const bodyGraph = computeLoopBodyGraphData(parentStep, parentNodeID, activitySource, expandedLoopIDs);
    if (!bodyGraph.nodes.length) return;

    placeLoopBodyGraphToRight(bodyGraph.nodes, parentNode);

    for (const node of bodyGraph.nodes) {
      nodes.push(node);
      nodeByID.set(node.id, node);
    }
    for (const entryID of loopBodyEntryNodeIDs(bodyGraph)) {
      edges.push({
        id: `${parentNodeID}-loop-entry-${entryID}`,
        source: parentNodeID,
        target: entryID,
        data: {
          status: parentStep.status,
          kind: 'loop-expand',
          sourcePort: 'right',
          targetPort: 'left',
        },
      });
    }
    edges.push(...bodyGraph.edges);

    for (const node of bodyGraph.nodes) {
      if (node.data.kind === 'variable' || node.data.kind === 'loop-group' || !node.data.step.loop?.body?.length) continue;
      appendExpandedLoopBodyGraph(node.id, node.data.step, activitySource);
    }
  };

  for (const step of snapshot.steps) {
    appendExpandedLoopBodyGraph(step.id, step, step);
  }

  return { nodes, edges, combos };
}

export function resolveClickedStep(
  nodeID: string,
  nodeData: StepNodeData | undefined,
  snapshot: FormulaDashboardSnapshot,
): FormulaDashboardStep | undefined {
  if (nodeData?.kind === 'loop-body') return nodeData.step;
  return nodeData?.step || snapshot.steps.find(step => step.id === nodeID);
}
