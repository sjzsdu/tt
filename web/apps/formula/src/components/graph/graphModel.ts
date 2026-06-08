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
};

export type LoopGroupNodeData = {
  step: FormulaDashboardStep;
  bodyCount: number;
};

export type FormulaGraphNode = {
  id: string;
  data: StepNodeData | LoopGroupNodeData;
  combo?: string;
};

export type FormulaGraphEdge = {
  id: string;
  source: string;
  target: string;
  data?: {
    status?: string;
    kind?: 'dependency' | 'loop-expand' | 'loop-sequence';
    sourcePort?: string;
    targetPort?: string;
    laneOffset?: number;
  };
};

export type FormulaGraphCombo = {
  id: string;
  data: {
    step: FormulaDashboardStep;
    bodyCount: number;
  };
};


const SOURCE_PORTS = ['right-top', 'right', 'right-bottom'];
const TARGET_PORTS = ['left-top', 'left', 'left-bottom'];
const LANE_STEP = 18;

function portForLane(ports: string[], index: number) {
  return ports[index % ports.length];
}

function centeredOffset(index: number, total: number) {
  if (total <= 1) return 0;
  return (index - (total - 1) / 2) * LANE_STEP;
}

function applyEdgeLanes(edges: FormulaGraphEdge[]) {
  const bySource = new Map<string, FormulaGraphEdge[]>();
  const byTarget = new Map<string, FormulaGraphEdge[]>();

  for (const edge of edges) {
    if (!bySource.has(edge.source)) bySource.set(edge.source, []);
    if (!byTarget.has(edge.target)) byTarget.set(edge.target, []);
    bySource.get(edge.source)!.push(edge);
    byTarget.get(edge.target)!.push(edge);
  }

  for (const sourceEdges of bySource.values()) {
    sourceEdges.forEach((edge, index) => {
      edge.data = {
        ...edge.data,
        sourcePort: portForLane(SOURCE_PORTS, index),
        laneOffset: (edge.data?.laneOffset || 0) + centeredOffset(index, sourceEdges.length),
      };
    });
  }

  for (const targetEdges of byTarget.values()) {
    targetEdges.forEach((edge, index) => {
      edge.data = {
        ...edge.data,
        targetPort: portForLane(TARGET_PORTS, index),
        laneOffset: (edge.data?.laneOffset || 0) - centeredOffset(index, targetEdges.length),
      };
    });
  }
}

export function loopBodyGraphID(parentID: string, bodyID: string) {
  return `${parentID}__${bodyID}`;
}

export function loopBodyStep(parent: FormulaDashboardStep, body: FormulaDashboardLoopBody, index: number): FormulaDashboardStep {
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

  for (const step of snapshot.steps) {
    nodes.push({
      id: step.id,
      data: {
        step,
        kind: 'step',
        expanded: expandedLoopIDs.has(step.id),
      },
    });

    if (expandedLoopIDs.has(step.id) && step.loop?.body?.length) {
      for (let i = 0; i < step.loop.body.length; i++) {
        const body = step.loop.body[i];
        const bodyStep = loopBodyStep(step, body, i);
        const bodyNodeID = loopBodyGraphID(step.id, body.id);

        nodes.push({
          id: bodyNodeID,
          data: {
            step: bodyStep,
            kind: 'loop-body',
            parentStep: step,
            body,
          },
          combo: `${step.id}__loop_group`,
        });
      }

      for (let i = 1; i < step.loop.body.length; i++) {
        const prevBody = step.loop.body[i - 1];
        const currBody = step.loop.body[i];
        edges.push({
          id: `${loopBodyGraphID(step.id, prevBody.id)}-${loopBodyGraphID(step.id, currBody.id)}`,
          source: loopBodyGraphID(step.id, prevBody.id),
          target: loopBodyGraphID(step.id, currBody.id),
          data: { kind: 'loop-sequence' },
        });
      }

      if (step.loop.body[0]) {
        edges.push({
          id: `${step.id}-${loopBodyGraphID(step.id, step.loop.body[0].id)}`,
          source: step.id,
          target: loopBodyGraphID(step.id, step.loop.body[0].id),
          data: { kind: 'loop-expand' },
        });
      }
    }
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

  const combos: FormulaGraphCombo[] = [];
  for (const step of snapshot.steps) {
    if (expandedLoopIDs.has(step.id) && step.loop?.body?.length) {
      combos.push({
        id: `${step.id}__loop_group`,
        data: {
          step,
          bodyCount: step.loop.body.length,
        },
      });
    }
  }

  applyEdgeLanes(edges);

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
