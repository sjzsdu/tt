import { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Popover } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { Graph, type GraphOptions } from '@antv/g6';
import type { FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { graphShortId } from '../../utils/status';
import { stepExecutionKind, stepExecutionLabel } from '../../utils/steps';
import { computeGraphData, loopBodyGraphID, resolveClickedStep, shouldToggleLoopOnClick, type LoopGroupNodeData, type StepNodeData, type VariableNodeData } from './graphModel';

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

function stepNodeStyle(step: FormulaDashboardStep, isDark: boolean) {
  const status = step.status || 'pending';
  const executionKind = stepExecutionKind(step);
  const statusColor = STATUS_COLORS[status] || STATUS_COLORS.pending;
  const isLoop = !!step.loop?.body?.length;
  const isLoopBody = step.type === 'loop-body';
  const width = isLoopBody ? 460 : 540;
  const labelFontSize = 14;
  const labelLineHeight = 20;
  const labelMaxWidth = width - 48;
  const labelLines = estimateTextLines(stepNodeLabel(step), labelMaxWidth, labelFontSize);
  const height = Math.max(isLoop ? 124 : 108, 56 + labelLines * labelLineHeight);

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
    badge: false,
    ports: [
      { key: 'top-left-3', placement: [0.14, 0], r: 2.5, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-left-2', placement: [0.26, 0], r: 2.6, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-left-1', placement: [0.38, 0], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top', placement: [0.5, 0], r: 3.3, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-right-1', placement: [0.62, 0], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-right-2', placement: [0.74, 0], r: 2.6, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'top-right-3', placement: [0.86, 0], r: 2.5, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
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
      lineWidth: edgeData.kind === 'loop-expand' ? 2.2 : 1.8,
      endArrow: true,
      endArrowSize: 8,
      lineDash: edgeData.kind === 'loop-expand' ? [10, 5] : [4, 5],
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

function relatedPathIDs(nodeID: string, edges: { id: string; source: string; target: string }[]) {
  const relatedNodes = new Set<string>([nodeID]);
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
        relatedNodes.add(nextNode);
        queue.push(nextNode);
      }
    }
  };

  walk(nodeID, 'in');
  walk(nodeID, 'out');

  return new Set([...relatedNodes, ...relatedEdges]);
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
              <p>Overview-first workflow map. Loops are expanded by default, and hover subtly emphasizes upstream/downstream paths.</p>
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
              <li><b>Edges</b>: target step status. Purple dashed edges are loop structure; cyan dashed edges are variable consumption.</li>
              <li><b>Layout</b>: dependency depths are layered; ports and curves separate overlapping lanes.</li>
              <li><b>Click</b>: opens details. Click a loop node to expand/collapse its body.</li>
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

export function GraphPanel({ snapshot, onSelect, theme }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void; theme: 'light' | 'dark' }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const [expandedLoopIDs, setExpandedLoopIDs] = useState<Set<string>>(() => new Set(loopStepIDs(snapshot)));

  const toggleLoop = (stepID: string) => {
    setExpandedLoopIDs(current => {
      const next = new Set(current);
      if (next.has(stepID)) next.delete(stepID);
      else next.add(stepID);
      return next;
    });
  };

  const isDark = theme === 'dark';

  const graphData = useMemo(() => computeGraphData(snapshot, expandedLoopIDs), [snapshot, expandedLoopIDs]);
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
      grouping: true,
      data: {
        nodes: initialGraphData.nodes,
        edges: initialGraphData.edges,
        combos: initialGraphData.combos,
      },
      layout: undefined,
      node: {
        type: 'rect',
        style: (node: { data?: StepNodeData | LoopGroupNodeData | VariableNodeData; style?: Record<string, unknown> & { x?: number; y?: number } }) => {
          const data = node.data;
          if (!data) return {};
          if (data.kind === 'variable') {
            return {
              ...variableNodeStyle(data, isDark),
              ...(node.style || {}),
              x: node.style?.x ?? data.layoutX,
              y: node.style?.y ?? data.layoutY,
            };
          }
          if (!('step' in data)) return {};
          return {
            ...stepNodeStyle(data.step, isDark),
            ...(node.style || {}),
            x: node.style?.x ?? data.layoutX,
            y: node.style?.y ?? data.layoutY,
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
        type: 'cubic-vertical',
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
        {
          type: 'click-select',
          multiple: false,
        },
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
    const pulsedNodeIDs = new Set<string>();

    const applyRunningPulse = () => {
      const { nodes } = graphDataRef.current;
      const runningIDs = new Set(
        nodes
          .filter(node => node.data.kind !== 'variable' && node.data.step.status === 'running')
          .map(node => node.id),
      );
      const pulse = (Math.sin(pulseFrame / 2) + 1) / 2;
      pulseFrame += 1;

      const updates = nodes
        .filter(node => {
          if (node.data.kind === 'variable') return false;
          return runningIDs.has(node.id) || pulsedNodeIDs.has(node.id);
        })
        .map(node => {
          const isRunning = runningIDs.has(node.id);
          if (isRunning) pulsedNodeIDs.add(node.id);
          else pulsedNodeIDs.delete(node.id);
          return {
            id: node.id,
            style: stepStatusBorderStyle(node.data.step, isDark, isRunning ? pulse : 0),
          };
        });

      if (!updates.length) return;
      graph.updateNodeData(updates);
      graph.draw();
    };

    const pulseTimer = window.setInterval(applyRunningPulse, 520);

    const applyHoverState = (nodeID: string | undefined) => {
      const { nodes, edges, combos } = graphDataRef.current;
      const activeIDs = nodeID ? relatedPathIDs(nodeID, edges) : new Set<string>();
      graph.updateNodeData(nodes.map(node => ({
        id: node.id,
        style: activeIDs.has(node.id)
          ? node.data.kind === 'variable'
            ? {
                stroke: isDark ? 'rgba(103, 232, 249, 0.56)' : 'rgba(8, 145, 178, 0.48)',
                lineWidth: 2.4,
                shadowBlur: 10,
                shadowColor: isDark ? 'rgba(34, 211, 238, 0.20)' : 'rgba(8, 145, 178, 0.16)',
              }
            : {
                ...stepStatusBorderStyle(node.data.step, isDark),
                lineWidth: Math.max(3.2, Number(stepStatusBorderStyle(node.data.step, isDark).lineWidth) + 0.9),
                shadowBlur: Math.max(12, Number(stepStatusBorderStyle(node.data.step, isDark).shadowBlur) + 4),
              }
          : node.data.kind === 'variable'
            ? {
                stroke: isDark ? 'rgba(103, 232, 249, 0.56)' : 'rgba(8, 145, 178, 0.48)',
                lineWidth: 1.4,
              }
          : stepStatusBorderStyle(node.data.step, isDark),
      })));
      graph.updateEdgeData(edges.map(edge => ({
        id: edge.id,
        style: activeIDs.has(edge.id)
          ? { ...edgeStyle(edge.data, isDark), lineWidth: Math.max(2.2, Number(edgeStyle(edge.data, isDark).lineWidth) + 0.6) }
          : edgeStyle(edge.data, isDark),
      })));
      graph.updateComboData(combos.map(combo => ({
        id: combo.id,
        style: activeIDs.has(combo.id)
          ? {
              stroke: combo.data?.step ? (STATUS_COLORS[combo.data.step.status] || '#a855f7') : '#a855f7',
              lineWidth: 2.1,
            }
          : {
              stroke: combo.data?.step ? (STATUS_COLORS[combo.data.step.status] || '#a855f7') : '#a855f7',
              lineWidth: 1.4,
            },
      })));
      graph.draw();
    };

    graph.on('node:pointerenter', (evt: { target?: { id?: string } }) => applyHoverState(evt.target?.id));
    graph.on('node:pointerleave', () => applyHoverState(undefined));

    graph.on('node:click', (evt: { target?: { id?: string } }) => {
      const nodeID = evt.target?.id;
      if (!nodeID) return;

      const datum = graph.getElementData(nodeID) as { data?: StepNodeData } | undefined;
      const step = resolveClickedStep(nodeID, datum?.data, snapshotRef.current);
      if (!step) return;

      if (datum?.data?.kind === 'loop-body' && datum.data.step.loop?.body?.length) {
        toggleLoop(nodeID);
      } else if (shouldToggleLoopOnClick(nodeID, step)) {
        toggleLoop(step.id);
      }
      onSelectRef.current(step);
    });

    void Promise.resolve(graph.render()).then(applyRunningPulse);

    return () => {
      window.clearInterval(pulseTimer);
      graph.destroy();
      graphRef.current = null;
    };
  }, [isDark]);

  useEffect(() => {
    const graph = graphRef.current;
    if (!graph) return;
    graph.setData({
      nodes: graphData.nodes,
      edges: graphData.edges,
      combos: graphData.combos,
    });
    graph.render();
  }, [graphData]);

  const running = snapshot.steps.find(step => step.status === 'running');
  const loopSteps = snapshot.steps.filter(step => !!step.loop?.body?.length).length;
  const expandedLoopCount = expandedLoopIDs.size;
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
      <div className="graph-canvas g6-canvas">
        <div ref={containerRef} className="g6-container" />
      </div>
    </div>
  );
}
