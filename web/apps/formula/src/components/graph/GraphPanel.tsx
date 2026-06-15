import { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Popover, Switch } from 'antd';
import { EyeOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import { Graph, type GraphOptions } from '@antv/g6';
import type { FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { graphShortId } from '../../utils/status';
import { stepExecutionKind, stepExecutionLabel } from '../../utils/steps';
import { computeGraphData, loopBodyGraphID, resolveClickedStep, type LoopGroupNodeData, type StepNodeData, type VariableNodeData } from './graphModel';

const STATUS_COLORS: Record<string, string> = {
  pending: 'rgba(148, 163, 184, 0.82)',
  running: '#67e8f9',
  completed: '#34d399',
  failed: '#fb7185',
  skipped: '#fbbf24',
  waiting_input: '#fbbf24',
};

const KIND_MARKS: Record<string, string> = {
  agent: '●',
  external_agent: '◉',
  script: '⌁',
  loop: '↻',
  tool: '◆',
  human_input: '?',
  aggregate: 'Σ',
  noop: '○',
  step: '■',
};

const KIND_BG_DARK: Record<string, string> = {
  agent: 'rgba(22, 119, 255, 0.18)',
  external_agent: 'rgba(235, 47, 150, 0.16)',
  script: 'rgba(250, 84, 28, 0.18)',
  loop: 'rgba(168, 85, 247, 0.20)',
  tool: 'rgba(19, 194, 194, 0.17)',
  human_input: 'rgba(250, 173, 20, 0.20)',
  aggregate: 'rgba(101, 163, 13, 0.17)',
  noop: 'rgba(140, 140, 140, 0.16)',
  step: 'rgba(22, 119, 255, 0.14)',
};

const KIND_BG_LIGHT: Record<string, string> = {
  agent: 'rgba(239, 246, 255, 0.98)',
  external_agent: 'rgba(253, 242, 248, 0.98)',
  script: 'rgba(255, 247, 237, 0.98)',
  loop: 'rgba(250, 245, 255, 0.98)',
  tool: 'rgba(236, 254, 255, 0.98)',
  human_input: 'rgba(255, 251, 235, 0.98)',
  aggregate: 'rgba(247, 254, 231, 0.98)',
  noop: 'rgba(248, 250, 252, 0.98)',
  step: 'rgba(239, 246, 255, 0.96)',
};

const STATUS_LEGEND = ['pending', 'running', 'completed', 'failed', 'skipped'];
const KIND_LEGEND = ['agent', 'script', 'tool', 'loop', 'human_input'];
const HOVER_ACCENT = '#38bdf8';
const HOVER_EDGE_ACCENT = 'rgba(56, 189, 248, 0.72)';
const LOOP_TOGGLE_BADGE_SIZE = 26;
const LOOP_TOGGLE_BADGE_OFFSET = 16;
const LOOP_TOGGLE_HITBOX = 36;


function edgeMarkerColor(status?: string) {
  return STATUS_COLORS[status || 'pending'] || STATUS_COLORS.pending;
}

function estimateTextLines(text: string, width: number, fontSize: number) {
  const averageCharWidth = fontSize * 0.58;
  const charsPerLine = Math.max(8, Math.floor(width / averageCharWidth));
  return (text || 'Untitled step')
    .split('\n')
    .map(line => line.replace(/\u200b/g, '').replace(/\s+/g, ' ').trim() || ' ')
    .reduce((total, line) => total + Math.max(1, Math.ceil(line.length / charsPerLine)), 0);
}

function wrapSafeText(text: string) {
  return text
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/([._:/-])/g, '$1 ')
    .replace(/([^\s]{24})/g, '$1 ');
}

function stepNodeLabel(step: FormulaDashboardStep) {
  const id = wrapSafeText(step.id || graphShortId(step.id));
  const title = wrapSafeText(step.title || graphShortId(step.id));
  return title === id ? id : `${id}\n${title}`;
}

function stepNodeMetrics(step: FormulaDashboardStep) {
  const isLoop = !!step.loop?.body?.length;
  const isLoopBody = step.type === 'loop-body';
  const width = isLoopBody ? 460 : 540;
  const labelFontSize = 14;
  const labelLineHeight = 20;
  const labelMaxWidth = width - 48;
  const labelLines = estimateTextLines(stepNodeLabel(step), labelMaxWidth, labelFontSize);
  const height = Math.max(isLoop ? 124 : 108, 56 + labelLines * labelLineHeight);
  return { width, height, labelFontSize, labelLineHeight, labelMaxWidth, labelLines };
}

function kindFill(executionKind: string, isDark: boolean) {
  const palette = isDark ? KIND_BG_DARK : KIND_BG_LIGHT;
  return palette[executionKind] || palette.step;
}

function stepStatusBorderStyle(step: FormulaDashboardStep, isDark: boolean, pulse = 0) {
  const status = step.status || 'pending';
  const statusColor = STATUS_COLORS[status] || STATUS_COLORS.pending;
  const isRunning = status === 'running';
  const pulseBoost = isRunning ? pulse : 0;

  return {
    stroke: isRunning ? '#22d3ee' : statusColor,
    lineWidth: isRunning ? 3 + pulseBoost * 1.15 : 2,
    shadowColor: isRunning ? `rgba(103, 232, 249, ${0.34 + pulseBoost * 0.2})` : isDark ? 'rgba(0, 0, 0, 0.22)' : 'rgba(15, 23, 42, 0.08)',
    shadowBlur: isRunning ? 14 + pulseBoost * 10 : 8,
    shadowOffsetY: 4,
  };
}

function loopToggleBadge(expanded: boolean | undefined, isDark: boolean) {
  return {
    text: expanded ? '−' : '+',
    placement: 'right-top',
    offsetX: -LOOP_TOGGLE_BADGE_OFFSET,
    offsetY: LOOP_TOGGLE_BADGE_OFFSET,
    backgroundCursor: 'pointer',
    backgroundFill: isDark ? 'rgba(15, 23, 42, 0.96)' : 'rgba(255, 255, 255, 0.98)',
    backgroundStroke: isDark ? 'rgba(196, 181, 253, 0.66)' : 'rgba(126, 34, 206, 0.42)',
    backgroundLineWidth: 1.2,
    backgroundRadius: LOOP_TOGGLE_BADGE_SIZE / 2,
    backgroundShadowBlur: 8,
    backgroundShadowColor: isDark ? 'rgba(168, 85, 247, 0.28)' : 'rgba(126, 34, 206, 0.16)',
    fill: isDark ? '#ddd6fe' : '#6b21a8',
    fontSize: 17,
    fontWeight: 900,
    lineHeight: LOOP_TOGGLE_BADGE_SIZE,
    textAlign: 'center',
    textBaseline: 'middle',
    padding: [0, 8],
    zIndex: 9,
  };
}

function stepNodeStyle(step: FormulaDashboardStep, isDark: boolean, expanded?: boolean) {
  const status = step.status || 'pending';
  const executionKind = stepExecutionKind(step);
  const statusColor = STATUS_COLORS[status] || STATUS_COLORS.pending;
  const isLoop = !!step.loop?.body?.length;
  const { width, height, labelFontSize, labelLineHeight, labelMaxWidth, labelLines } = stepNodeMetrics(step);

  return {
    size: [width, height],
    fill: kindFill(executionKind, isDark),
    ...stepStatusBorderStyle(step, isDark),
    radius: 14,
    cursor: 'pointer',
    labelText: stepNodeLabel(step),
    labelFill: isDark ? '#e2e8f0' : '#1e293b',
    labelFontSize,
    labelFontWeight: 700,
    labelLineHeight,
    labelWordWrap: true,
    labelMaxWidth,
    labelMaxLines: Math.max(2, labelLines),
    labelTextOverflow: 'clip',
    labelPlacement: 'center',
    badge: isLoop,
    badges: isLoop ? [loopToggleBadge(expanded, isDark)] : [],
    ports: [
      { key: 'top-left-3', placement: [0.14, 0], r: 2.5, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-left-2', placement: [0.26, 0], r: 2.6, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-left-1', placement: [0.38, 0], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top', placement: [0.5, 0], r: 3.3, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-right-1', placement: [0.62, 0], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-right-2', placement: [0.74, 0], r: 2.6, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-right-3', placement: [0.86, 0], r: 2.5, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'left', placement: [0, 0.5], r: 3.1, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'right', placement: [1, 0.5], r: 3.1, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom-left-3', placement: [0.14, 1], r: 2.5, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom-left-2', placement: [0.26, 1], r: 2.6, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom-left-1', placement: [0.38, 1], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom', placement: [0.5, 1], r: 3.3, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom-right-1', placement: [0.62, 1], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom-right-2', placement: [0.74, 1], r: 2.6, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'bottom-right-3', placement: [0.86, 1], r: 2.5, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
    ],
  };
}

function variableNodeStyle(data: VariableNodeData, isDark: boolean) {
  return {
    size: [220, 54],
    fill: isDark ? 'rgba(8, 145, 178, 0.13)' : 'rgba(236, 254, 255, 0.96)',
    stroke: isDark ? 'rgba(103, 232, 249, 0.56)' : 'rgba(8, 145, 178, 0.48)',
    lineWidth: 1.4,
    radius: 18,
    labelText: `$ ${data.key}`,
    labelFill: isDark ? '#a5f3fc' : '#155e75',
    labelFontSize: 12,
    labelFontWeight: 750,
    labelWordWrap: true,
    labelMaxWidth: 188,
    labelPlacement: 'center',
    badge: false,
    ports: [
      { key: 'bottom', placement: [0.5, 1], r: 2.8, fill: '#22d3ee', stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top', placement: [0.5, 0], r: 2.8, fill: '#22d3ee', stroke: isDark ? '#0f172a' : '#fff' },
    ],
  };
}

function edgeStyle(edgeData: { status?: string; kind?: string; sourcePort?: string; targetPort?: string; laneOffset?: number } | undefined, isDark: boolean) {
  const laneOffset = Math.max(-42, Math.min(42, edgeData?.laneOffset || 0));
  if (edgeData?.kind === 'variable-consume') {
    return {
      sourcePort: edgeData.sourcePort,
      targetPort: edgeData.targetPort,
      curveOffset: [laneOffset, -laneOffset],
      stroke: isDark ? 'rgba(34, 211, 238, 0.62)' : 'rgba(8, 145, 178, 0.56)',
      lineWidth: 1.35,
      endArrow: true,
      endArrowSize: 7,
      lineDash: [3, 5],
    };
  }
  if (edgeData?.kind === 'loop-expand' || edgeData?.kind === 'loop-sequence') {
    return {
      sourcePort: edgeData.sourcePort,
      targetPort: edgeData.targetPort,
      curveOffset: [laneOffset, -laneOffset],
      stroke: '#a855f7',
      lineWidth: edgeData.kind === 'loop-expand' ? 2.4 : 1.8,
      endArrow: true,
      endArrowSize: 8,
      lineDash: edgeData.kind === 'loop-expand' ? [9, 5] : [4, 5],
      opacity: edgeData.kind === 'loop-expand' ? 0.86 : 1,
    };
  }

  const status = edgeData?.status;
  const stroke = edgeMarkerColor(status);
  const baseStroke = isDark ? 'rgba(148, 163, 184, 0.42)' : 'rgba(100, 116, 139, 0.42)';
  return {
    sourcePort: edgeData?.sourcePort,
    targetPort: edgeData?.targetPort,
    curveOffset: [laneOffset, -laneOffset],
    stroke: status ? stroke : baseStroke,
    lineWidth: status === 'running' ? 2.4 : 1.5,
    endArrow: true,
    endArrowSize: 8,
    lineDash: status === 'running' ? [8, 4] : status === 'skipped' ? [5, 5] : undefined,
  };
}

function loopStepIDs(snapshot: FormulaDashboardSnapshot) {
  const ids: string[] = [];
  const collect = (step: FormulaDashboardStep, nodeID: string) => {
    if (!step.loop?.body?.length) return;
    ids.push(nodeID);
    for (const body of step.loop.body) {
      if (!body.loop?.body?.length) continue;
      collect({
        id: loopBodyGraphID(nodeID, body.id),
        title: body.title || body.id,
        agent: body.agent || step.agent,
        status: 'pending',
        index: 0,
        depth: (step.depth || 0) + 1,
        loop: body.loop,
      }, loopBodyGraphID(nodeID, body.id));
    }
  };
  for (const step of snapshot.steps) collect(step, step.id);
  return ids;
}

function relatedPathEdgeIDs(nodeID: string, edges: { id: string; source: string; target: string }[]) {
  const relatedEdges = new Set<string>();
  const outgoing = new Map<string, typeof edges>();
  const incoming = new Map<string, typeof edges>();

  for (const edge of edges) {
    if (!outgoing.has(edge.source)) outgoing.set(edge.source, []);
    if (!incoming.has(edge.target)) incoming.set(edge.target, []);
    outgoing.get(edge.source)!.push(edge);
    incoming.get(edge.target)!.push(edge);
  }

  const walk = (start: string, direction: 'in' | 'out') => {
    const queue = [start];
    const visited = new Set<string>();
    while (queue.length) {
      const current = queue.shift()!;
      if (visited.has(current)) continue;
      visited.add(current);

      const nextEdges = direction === 'out' ? outgoing.get(current) : incoming.get(current);
      for (const edge of nextEdges || []) {
        relatedEdges.add(edge.id);
        const nextNode = direction === 'out' ? edge.target : edge.source;
        queue.push(nextNode);
      }
    }
  };

  walk(nodeID, 'in');
  walk(nodeID, 'out');

  return relatedEdges;
}

type GraphNodePointerEvent = {
  target?: { id?: string };
  originalTarget?: unknown;
  currentTarget?: unknown;
  originalEvent?: { clientX?: number; clientY?: number; stopPropagation?: () => void; preventDefault?: () => void };
  canvas?: { x?: number; y?: number } | [number, number];
  canvasX?: number;
  canvasY?: number;
  x?: number;
  y?: number;
};

function targetDescriptor(target: unknown) {
  if (!target || typeof target !== 'object') return '';
  const value = target as Record<string, unknown>;
  const ctor = (value as { constructor?: { name?: string } }).constructor;
  return [
    value.id,
    value.name,
    value.className,
    value.nodeName,
    value.type,
    value.key,
    value.__name__,
    ctor?.name,
  ].map(part => String(part || '').toLowerCase()).join(' ');
}

function isBadgeClick(evt: GraphNodePointerEvent) {
  return `${targetDescriptor(evt.originalTarget)} ${targetDescriptor(evt.currentTarget)}`.includes('badge');
}

function eventCanvasPoint(graph: Graph, evt: GraphNodePointerEvent): [number, number] | undefined {
  if (Array.isArray(evt.canvas) && typeof evt.canvas[0] === 'number' && typeof evt.canvas[1] === 'number') {
    return [Number(evt.canvas[0]), Number(evt.canvas[1])];
  }
  if (evt.canvas && !Array.isArray(evt.canvas) && typeof evt.canvas.x === 'number' && typeof evt.canvas.y === 'number') {
    return [Number(evt.canvas.x), Number(evt.canvas.y)];
  }
  if (typeof evt.canvasX === 'number' && typeof evt.canvasY === 'number') {
    return [Number(evt.canvasX), Number(evt.canvasY)];
  }
  if (evt.originalEvent && typeof evt.originalEvent.clientX === 'number' && typeof evt.originalEvent.clientY === 'number') {
    const point = graph.getCanvasByClient([Number(evt.originalEvent.clientX), Number(evt.originalEvent.clientY)]);
    return [Number(point[0]), Number(point[1])];
  }
  if (typeof evt.x === 'number' && typeof evt.y === 'number') {
    return [Number(evt.x), Number(evt.y)];
  }
  return undefined;
}

function isLoopToggleClick(graph: Graph, nodeData: StepNodeData | undefined, evt: GraphNodePointerEvent) {
  if (!nodeData?.step?.loop?.body?.length) return false;
  if (isBadgeClick(evt)) return true;

  const point = eventCanvasPoint(graph, evt);
  if (!point) return false;

  const { width, height } = stepNodeMetrics(nodeData.step);
  const x = nodeData.layoutX || 0;
  const y = nodeData.layoutY || 0;
  const halfHitbox = LOOP_TOGGLE_HITBOX / 2;
  const candidates: [number, number][] = [
    [x + width / 2 - LOOP_TOGGLE_BADGE_OFFSET, y - height / 2 + LOOP_TOGGLE_BADGE_OFFSET],
    [x + width - LOOP_TOGGLE_BADGE_OFFSET, y + LOOP_TOGGLE_BADGE_OFFSET],
  ];

  return candidates.some(([centerX, centerY]) => (
    Math.abs(point[0] - centerX) <= halfHitbox
    && Math.abs(point[1] - centerY) <= halfHitbox
  ));
}

function GraphHelpPopover({ runningTitle, nodeCount, edgeCount, loopCount, expandedLoopCount }: { runningTitle?: string; nodeCount: number; edgeCount: number; loopCount: number; expandedLoopCount: number }) {
  return (
    <Popover
      trigger="click"
      placement="bottom"
      content={(
        <div className="graph-help-popover">
          <div className="graph-help-hero">
            <div>
              <strong>Execution graph</strong>
              <p>Overview-first workflow map. Expanded loop bodies render as connected graph islands to the right of their loop step.</p>
            </div>
            {runningTitle && (
              <div className="graph-help-live">
                <span>Live</span>
                <b>{runningTitle}</b>
              </div>
            )}
          </div>
          <div className="graph-help-metrics">
            <span>{nodeCount} nodes</span>
            <span>{edgeCount} edges</span>
            {!!loopCount && <span>{loopCount} loop{loopCount === 1 ? '' : 's'} · {expandedLoopCount} expanded</span>}
          </div>
          <div className="graph-help-section">
            <span className="graph-legend-title">Status</span>
            <div className="graph-legend">
              {STATUS_LEGEND.map(status => (
                <span className={`legend-pill status ${status}`} key={status}>{status}</span>
              ))}
            </div>
          </div>
          <div className="graph-help-section">
            <span className="graph-legend-title">Type</span>
            <div className="graph-legend kind-legend">
              {KIND_LEGEND.map(kind => (
                <span className={`legend-pill kind ${kind}`} key={kind}>
                  <i aria-hidden="true" /> {KIND_MARKS[kind]} {stepExecutionLabel(kind as ReturnType<typeof stepExecutionKind>)}
                </span>
              ))}
            </div>
          </div>
          <div className="graph-help-section compact">
            <span className="graph-legend-title">How to read</span>
            <ul>
              <li><b>Direction</b>: dependency / execution order flows top to bottom.</li>
              <li><b>Border</b>: node status. Running nodes also have a soft pulsing border.</li>
              <li><b>Background</b>: execution type. Agent blue, script orange, tool cyan, loop purple, human input yellow.</li>
              <li><b>$ var nodes</b>: formula template variables like <b>{'{{repo}}'}</b> and where they are consumed.</li>
              <li><b>Solid edges</b>: execution dependencies, meaning the target step waits for or follows the source step.</li>
              <li><b>Purple dashed edges</b>: loop expansion/loop-body flow, connecting a loop step to its expanded internal steps.</li>
              <li><b>Cyan dashed edges</b>: variable consumption, meaning the target step references a formula var such as <b>{'{{repo}}'}</b>.</li>
              <li><b>Other dashed edges</b>: non-normal execution state such as running/skipped, or separated lanes used to reduce overlap.</li>
              <li><b>Hover</b>: hover a node to highlight related upstream/downstream edges.</li>
              <li><b>Layout</b>: dependency depths are layered; ports and curves separate overlapping lanes.</li>
              <li><b>Click</b>: opens details. Use the +/- icon on loop nodes to open or hide the connected loop-body graph on the right.</li>
            </ul>
          </div>
        </div>
      )}
    >
      <button type="button" className="graph-help-button" aria-label="Graph guide">
        <QuestionCircleOutlined />
      </button>
    </Popover>
  );
}

function GraphDisplayToggle({
  showVariables,
  showEdges,
  onShowVariablesChange,
  onShowEdgesChange,
}: {
  showVariables: boolean;
  showEdges: boolean;
  onShowVariablesChange: (value: boolean) => void;
  onShowEdgesChange: (value: boolean) => void;
}) {
  return (
    <Popover
      trigger="click"
      placement="bottomRight"
      content={(
        <div className="graph-display-popover">
          <strong>Graph display</strong>
          <label className="graph-display-row">
            <span>
              <b>Variables</b>
              <em>$ var nodes and variable-consume edges</em>
            </span>
            <Switch size="small" checked={showVariables} onChange={onShowVariablesChange} />
          </label>
          <label className="graph-display-row">
            <span>
              <b>Edges</b>
              <em>Dependency, loop and variable lines</em>
            </span>
            <Switch size="small" checked={showEdges} onChange={onShowEdgesChange} />
          </label>
        </div>
      )}
    >
      <button
        type="button"
        className={`graph-display-button${showVariables && showEdges ? '' : ' active'}`}
        aria-label="Toggle graph variables and edges"
        title="Toggle graph variables and edges"
      >
        <EyeOutlined />
      </button>
    </Popover>
  );
}

export function GraphPanel({ snapshot, onSelect, theme }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void; theme: 'light' | 'dark' }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const renderGraphRef = useRef<(() => void) | null>(null);
  const [graphError, setGraphError] = useState('');
  const [expandedLoopIDs, setExpandedLoopIDs] = useState<Set<string>>(() => new Set(loopStepIDs(snapshot)));
  const [showVariables, setShowVariables] = useState(true);
  const [showEdges, setShowEdges] = useState(true);

  const toggleLoop = (stepID: string) => {
    setExpandedLoopIDs(current => {
      const next = new Set(current);
      if (next.has(stepID)) next.delete(stepID);
      else next.add(stepID);
      return next;
    });
  };

  const isDark = theme === 'dark';

  const rawGraphData = useMemo(() => computeGraphData(snapshot, expandedLoopIDs), [snapshot, expandedLoopIDs]);
  const graphData = useMemo(() => {
    const nodes = showVariables
      ? rawGraphData.nodes
      : rawGraphData.nodes.filter(node => node.data.kind !== 'variable');
    const visibleNodeIDs = new Set(nodes.map(node => node.id));
    const edges = showEdges
      ? rawGraphData.edges.filter(edge => (
          visibleNodeIDs.has(edge.source)
          && visibleNodeIDs.has(edge.target)
          && (showVariables || edge.data?.kind !== 'variable-consume')
        ))
      : [];
    return { nodes, edges, combos: rawGraphData.combos };
  }, [rawGraphData, showEdges, showVariables]);
  const graphDataRef = useRef(graphData);
  const snapshotRef = useRef(snapshot);
  const onSelectRef = useRef(onSelect);

  useEffect(() => {
    graphDataRef.current = graphData;
    snapshotRef.current = snapshot;
    onSelectRef.current = onSelect;
  }, [graphData, onSelect, snapshot]);

  useEffect(() => {
    if (!containerRef.current) return;

    const initialGraphData = graphDataRef.current;

    const config: GraphOptions = {
      container: containerRef.current,
      autoFit: 'view',
      padding: 40,
      data: {
        nodes: initialGraphData.nodes,
        edges: initialGraphData.edges,
        combos: initialGraphData.combos,
      },
      layout: undefined,
      node: {
        type: 'rect',
        style: (node: unknown) => {
          const graphNode = node as { data?: StepNodeData | LoopGroupNodeData | VariableNodeData; style?: Record<string, unknown> & { x?: number; y?: number } };
          const data = graphNode.data;
          if (!data) return {};
          if (data.kind === 'variable') {
            return {
              ...variableNodeStyle(data, isDark),
              ...(graphNode.style || {}),
              x: graphNode.style?.x ?? data.layoutX,
              y: graphNode.style?.y ?? data.layoutY,
            };
          }
          if (data.kind === 'loop-group' || !('step' in data)) return {};
          return {
            ...stepNodeStyle(data.step, isDark, data.expanded),
            ...(graphNode.style || {}),
            x: graphNode.style?.x ?? data.layoutX,
            y: graphNode.style?.y ?? data.layoutY,
          };
        },
        state: {
          active: {
            opacity: 1,
            stroke: HOVER_ACCENT,
            lineWidth: 2.1,
            shadowBlur: 6,
          },
          inactive: {
            opacity: 0.96,
          },
        },
      },
      edge: {
        type: edge => edge.data?.kind === 'loop-expand' ? 'cubic-horizontal' : 'cubic-vertical',
        style: (edge: { data?: { status?: string; kind?: string; sourcePort?: string; targetPort?: string; laneOffset?: number }; style?: Record<string, unknown> }) => ({
          ...edgeStyle(edge.data, isDark),
          ...(edge.style || {}),
        }),
        state: {
          active: {
            opacity: 1,
            stroke: HOVER_ACCENT,
            lineWidth: 1.7,
          },
          inactive: {
            opacity: 0.94,
          },
        },
      },
      combo: {
        type: 'rect',
        style: (combo: { data?: { step?: FormulaDashboardStep; bodyCount?: number }; style?: Record<string, unknown> }) => ({
          radius: 14,
          fill: isDark ? 'rgba(8, 13, 25, 0.42)' : 'rgba(248, 250, 252, 0.72)',
          stroke: combo.data?.step ? (STATUS_COLORS[combo.data.step.status] || '#a855f7') : '#a855f7',
          lineDash: [12, 4],
          lineWidth: 1.4,
          padding: 28,
          labelText: combo.data?.step ? `↻ ${combo.data.step.title} · ${combo.data.bodyCount || 0} body` : 'loop body',
          labelFill: isDark ? '#c4b5fd' : '#7e22ce',
          labelFontSize: 12,
          labelFontWeight: 700,
          ...(combo.style || {}),
        }),
        state: {
          active: { opacity: 1, stroke: HOVER_ACCENT, lineWidth: 1.5 },
          inactive: { opacity: 0.95 },
        },
      },
      behaviors: [
        {
          type: 'zoom-canvas',
          sensitivity: 0.35,
        },
        'drag-canvas',
      ],
      plugins: [
        {
          type: 'minimap',
          size: [160, 100],
          position: 'bottom-left',
          style: {
            container: {
              backgroundColor: isDark ? 'rgba(10, 18, 32, 0.96)' : 'rgba(255, 255, 255, 0.96)',
              borderRadius: 8,
              border: `1px solid ${isDark ? 'rgba(125, 211, 252, 0.14)' : 'rgba(59, 130, 246, 0.16)'}`,
            },
            viewport: {
              fill: isDark ? 'rgba(125, 211, 252, 0.12)' : 'rgba(59, 130, 246, 0.1)',
              stroke: isDark ? 'rgba(125, 211, 252, 0.95)' : 'rgba(59, 130, 246, 0.92)',
            },
          },
        },
      ],
    };

    const graph = new Graph(config);
    graphRef.current = graph;

    let pulseFrame = 0;
    let renderTimer: ReturnType<typeof window.setTimeout> | undefined;
    let renderInFlight = false;
    let renderAgain = false;
    let disposed = false;
    const pulsedNodeIDs = new Set<string>();
    const reportGraphError = (phase: string, err: unknown) => {
      console.error(`Formula graph ${phase} failed`, err);
      if (!disposed) {
        const message = err instanceof Error ? err.message : String(err);
        setGraphError(`${phase}: ${message}`);
      }
    };
    const clearGraphError = () => {
      if (!disposed) setGraphError('');
    };

    const renderGraph = () => {
      if (disposed || document.hidden) {
        renderAgain = true;
        return;
      }
      if (renderInFlight) {
        renderAgain = true;
        return;
      }
      if (renderTimer) window.clearTimeout(renderTimer);
      renderTimer = window.setTimeout(() => {
        renderTimer = undefined;
        if (disposed || document.hidden) {
          renderAgain = true;
          return;
        }
        renderInFlight = true;
        renderAgain = false;
        void Promise.resolve(graph.render())
          .then(clearGraphError)
          .catch(err => reportGraphError('render', err))
          .finally(() => {
            renderInFlight = false;
            if (renderAgain && !disposed) renderGraph();
          });
      }, 180);
    };

    renderGraphRef.current = renderGraph;

    const flushWhenVisible = () => {
      if (!document.hidden && renderAgain) renderGraph();
    };
    document.addEventListener('visibilitychange', flushWhenVisible);

    const applyRunningPulse = () => {
      if (document.hidden || renderInFlight) return;
      const { nodes } = graphDataRef.current;
      const runningIDs = new Set(
        nodes
          .filter(node => node.data.kind !== 'variable' && node.data.step.status === 'running')
          .map(node => node.id),
      );
      const pulse = (Math.sin(pulseFrame / 2) + 1) / 2;
      pulseFrame += 1;

      const updates = nodes
        .flatMap(node => {
          if (node.data.kind === 'variable' || !('step' in node.data)) return [];
          if (!runningIDs.has(node.id) && !pulsedNodeIDs.has(node.id)) return [];
          const isRunning = runningIDs.has(node.id);
          if (isRunning) pulsedNodeIDs.add(node.id);
          else pulsedNodeIDs.delete(node.id);
          return [{
            id: node.id,
            style: stepStatusBorderStyle(node.data.step, isDark, isRunning ? pulse : 0),
          }];
        });

      if (!updates.length) return;
      try {
        graph.updateNodeData(updates);
        void Promise.resolve(graph.draw())
          .then(clearGraphError)
          .catch(err => reportGraphError('pulse draw', err));
      } catch (err) {
        reportGraphError('pulse update', err);
      }
    };

    const pulseTimer = window.setInterval(applyRunningPulse, 520);

    const applyHoverState = (nodeID: string | undefined) => {
      const { edges } = graphDataRef.current;
      const activeEdgeIDs = nodeID ? relatedPathEdgeIDs(nodeID, edges) : new Set<string>();
      try {
        graph.updateEdgeData(edges.map(edge => ({
          id: edge.id,
          style: activeEdgeIDs.has(edge.id)
            ? {
                ...edgeStyle(edge.data, isDark),
                stroke: HOVER_EDGE_ACCENT,
                lineWidth: Math.max(2.8, Number(edgeStyle(edge.data, isDark).lineWidth) + 1.1),
                shadowBlur: 6,
                shadowColor: HOVER_EDGE_ACCENT,
              }
            : edgeStyle(edge.data, isDark),
        })));
        void Promise.resolve(graph.draw())
          .then(clearGraphError)
          .catch(err => reportGraphError('hover draw', err));
      } catch (err) {
        reportGraphError('hover update', err);
      }
    };

    graph.on('node:pointerenter', (evt: unknown) => applyHoverState((evt as { target?: { id?: string } }).target?.id));
    graph.on('node:pointerleave', () => applyHoverState(undefined));

    graph.on('node:click', (evt: unknown) => {
      const event = evt as GraphNodePointerEvent;
      const nodeID = event.target?.id;
      if (!nodeID) return;

      try {
        const datum = graph.getElementData(nodeID) as { data?: StepNodeData } | undefined;
        const step = resolveClickedStep(nodeID, datum?.data, snapshotRef.current);
        if (!step) return;

        if (isLoopToggleClick(graph, datum?.data, event)) {
          event.originalEvent?.preventDefault?.();
          event.originalEvent?.stopPropagation?.();
          toggleLoop(nodeID);
          return;
        }

        onSelectRef.current(step);
      } catch (err) {
        reportGraphError('node click', err);
      }
    });

    renderGraph();

    return () => {
      disposed = true;
      renderGraphRef.current = null;
      if (renderTimer) window.clearTimeout(renderTimer);
      document.removeEventListener('visibilitychange', flushWhenVisible);
      window.clearInterval(pulseTimer);
      try {
        graph.destroy();
      } catch (err) {
        console.error('Formula graph destroy failed', err);
      }
      graphRef.current = null;
    };
  }, [isDark]);

  useEffect(() => {
    const graph = graphRef.current;
    if (!graph) return;
    try {
      graph.setData({
        nodes: graphData.nodes,
        edges: graphData.edges,
        combos: graphData.combos,
      });
      renderGraphRef.current?.();
    } catch (err) {
      console.error('Formula graph data update failed', err);
      const message = err instanceof Error ? err.message : String(err);
      setGraphError(`data update: ${message}`);
    }
  }, [graphData]);

  const running = snapshot.steps.find(step => step.status === 'running');
  const allLoopIDs = loopStepIDs(snapshot);
  const loopSteps = allLoopIDs.length;
  const expandedLoopCount = allLoopIDs.filter(id => expandedLoopIDs.has(id)).length;
  const metricNodeCount = graphData.nodes.length;
  const metricEdgeCount = graphData.edges.length;

  if (!snapshot.steps.length) {
    return <Empty description="No executable steps" />;
  }

  return (
    <div className="graph-panel">
      <div className="graph-header">
        <div>
          <div className="graph-title-row">
            <h3>Execution graph</h3>
            <GraphDisplayToggle
              showVariables={showVariables}
              showEdges={showEdges}
              onShowVariablesChange={setShowVariables}
              onShowEdgesChange={setShowEdges}
            />
            <GraphHelpPopover
              runningTitle={running?.title}
              nodeCount={metricNodeCount}
              edgeCount={metricEdgeCount}
              loopCount={loopSteps}
              expandedLoopCount={expandedLoopCount}
            />
          </div>
        </div>
      </div>
      <div className="graph-workspace">
        <div className="graph-canvas g6-canvas">
          {graphError ? (
            <div className="graph-runtime-warning" role="status">
              Graph view recovered from a render issue. Waiting for the next update… <span>{graphError}</span>
            </div>
          ) : null}
          <div ref={containerRef} className="g6-container" />
        </div>
      </div>
    </div>
  );
}
