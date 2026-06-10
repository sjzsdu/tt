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

function variableNodeID(key: string) {
  return `var::${key.replace(/[^a-zA-Z0-9_.:-]/g, '_')}`;
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

function assignLayoutPositions(nodes: FormulaGraphNode[]) {
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
      const x = (index - center) * NODE_X_GAP;
      const y = rank * NODE_Y_GAP;
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

function loopGroupID(stepID: string) {
  return `${stepID}__loop_group`;
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

  const addLoopBody = (parentStep: FormulaDashboardStep, parentNodeID: string, parentComboID: string | undefined, activitySource: FormulaDashboardStep) => {
    if (!expandedLoopIDs.has(parentNodeID) || !parentStep.loop?.body?.length) return;

    const comboID = loopGroupID(parentNodeID);
    combos.push({
      id: comboID,
      combo: parentComboID,
      data: {
        step: parentStep,
        bodyCount: parentStep.loop.body.length,
        depth: parentStep.depth || 0,
      },
    });

    for (let i = 0; i < parentStep.loop.body.length; i++) {
      const body = parentStep.loop.body[i];
      const bodyStep = loopBodyStep(parentStep, body, i, activitySource);
      bodyStep.id = loopBodyGraphID(parentNodeID, body.id);
      bodyStep.depth = (parentStep.depth || 0) + 1;
      const bodyNodeID = bodyStep.id;

      nodes.push({
        id: bodyNodeID,
        data: {
          step: bodyStep,
          kind: 'loop-body',
          parentStep,
          body,
          expanded: expandedLoopIDs.has(bodyNodeID),
        },
        combo: comboID,
      });

      addVariableConsumers(bodyStep, bodyNodeID);
      addLoopBody(bodyStep, bodyNodeID, comboID, activitySource);
    }

    for (let i = 1; i < parentStep.loop.body.length; i++) {
      const prevBody = parentStep.loop.body[i - 1];
      const currBody = parentStep.loop.body[i];
      edges.push({
        id: `${loopBodyGraphID(parentNodeID, prevBody.id)}-${loopBodyGraphID(parentNodeID, currBody.id)}`,
        source: loopBodyGraphID(parentNodeID, prevBody.id),
        target: loopBodyGraphID(parentNodeID, currBody.id),
        data: { kind: 'loop-sequence' },
      });
    }

    if (parentStep.loop.body[0]) {
      edges.push({
        id: `${parentNodeID}-${loopBodyGraphID(parentNodeID, parentStep.loop.body[0].id)}`,
        source: parentNodeID,
        target: loopBodyGraphID(parentNodeID, parentStep.loop.body[0].id),
        data: { kind: 'loop-expand' },
      });
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
    addLoopBody(step, step.id, undefined, step);
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

  return { nodes, edges, combos };
}

export function resolveClickedStep(
  nodeID: string,
  nodeData: StepNodeData | undefined,
  snapshot: FormulaDashboardSnapshot,
): FormulaDashboardStep | undefined {
  if (nodeData?.kind === 'loop-body') return nodeData.parentStep;
  return nodeData?.step || snapshot.steps.find(step => step.id === nodeID);
}

export function shouldToggleLoopOnClick(nodeID: string, step: FormulaDashboardStep | undefined) {
  return !!step && step.id === nodeID && !!step.loop?.body?.length;
}
