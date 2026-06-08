import { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Tooltip } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { Graph, type GraphOptions } from '@antv/g6';
import type { FormulaDashboardSnapshot, FormulaDashboardStep } from '../../types';
import { graphShortId, statusLabel } from '../../utils/status';
import { stepExecutionKind, stepExecutionLabel } from '../../utils/steps';
import { computeGraphData, resolveClickedStep, shouldToggleLoopOnClick, type LoopGroupNodeData, type StepNodeData } from './graphModel';

const STATUS_COLORS: Record<string, string> = {
  pending: 'rgba(148, 163, 184, 0.82)',
  running: '#67e8f9',
  completed: '#34d399',
  failed: '#fb7185',
  skipped: '#fbbf24',
  waiting_input: '#fbbf24',
};

const STATUS_BG: Record<string, string> = {
  pending: 'rgba(30, 41, 59, 0.78)',
  running: 'rgba(8, 47, 73, 0.46)',
  completed: 'rgba(6, 78, 59, 0.42)',
  failed: 'rgba(88, 28, 46, 0.45)',
  skipped: 'rgba(113, 63, 18, 0.34)',
  waiting_input: 'rgba(113, 63, 18, 0.40)',
};

const STATUS_BG_LIGHT: Record<string, string> = {
  pending: 'rgba(248, 250, 252, 0.94)',
  running: 'rgba(236, 254, 255, 0.95)',
  completed: 'rgba(236, 253, 245, 0.95)',
  failed: 'rgba(255, 241, 242, 0.96)',
  skipped: 'rgba(255, 251, 235, 0.95)',
  waiting_input: 'rgba(255, 251, 235, 0.96)',
};

const KIND_COLORS: Record<string, string> = {
  agent: '#1677ff',
  external_agent: '#eb2f96',
  script: '#fa541c',
  loop: '#a855f7',
  tool: '#13c2c2',
  human_input: '#faad14',
  aggregate: '#65a30d',
  noop: '#8c8c8c',
  step: '#1677ff',
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

const KIND_LINE_DASH: Record<string, number[] | undefined> = {
  agent: undefined,
  external_agent: [10, 3, 2, 3],
  script: [8, 5],
  loop: [12, 4],
  tool: [2, 4],
  human_input: [6, 3, 2, 3],
  aggregate: [14, 3],
  noop: [3, 6],
  step: undefined,
};

const STATUS_LEGEND = ['pending', 'running', 'completed', 'failed', 'skipped'];
const KIND_LEGEND = ['agent', 'script', 'tool', 'loop', 'human_input'];


function edgeMarkerColor(status?: string) {
  return STATUS_COLORS[status || 'pending'] || STATUS_COLORS.pending;
}

function compactText(text: string | undefined, maxLength: number) {
  const clean = (text || '').replace(/\s+/g, ' ').trim();
  if (!clean) return '';
  return clean.length > maxLength ? `${clean.slice(0, maxLength - 1)}…` : clean;
}

function stepNodeLabel(step: FormulaDashboardStep, expanded = false) {
  const executionKind = stepExecutionKind(step);
  const mark = KIND_MARKS[executionKind] || KIND_MARKS.step;
  const prefix = step.loop?.body?.length ? (expanded ? '▾ loop' : '▸ loop') : graphShortId(step.id);
  const description = compactText(step.description || step.notes, 88);
  const meta = compactText(stepMetaText(step), 72);
  return [
    `${mark} ${prefix} · ${stepExecutionLabel(executionKind)}`,
    compactText(step.title, 72),
    description,
    meta ? `• ${meta}` : '',
  ].filter(Boolean).join('\n');
}

function stepMetaText(step: FormulaDashboardStep) {
  const metaParts: string[] = [];
  if (step.loop?.body?.length) metaParts.push(`${step.loop.body.length} body`);
  if (step.agent) metaParts.push(`agent ${step.agent}`);
  if (step.human_input_request) metaParts.push('input required');
  if (step.depends_on?.length) metaParts.push(`${step.depends_on.length} deps`);
  if (step.activities?.length) metaParts.push(`${step.activities.length} events`);
  return metaParts.join(' · ');
}

function stepNodeStyle(step: FormulaDashboardStep, isDark: boolean, expanded = false) {
  const status = step.status || 'pending';
  const executionKind = stepExecutionKind(step);
  const statusColor = STATUS_COLORS[status] || STATUS_COLORS.pending;
  const kindColor = KIND_COLORS[executionKind] || KIND_COLORS.step;
  const kindDash = KIND_LINE_DASH[executionKind];
  const isLoop = !!step.loop?.body?.length;
  const isLoopBody = step.type === 'loop-body';
  const width = isLoopBody ? 300 : 340;
  const height = isLoop ? 178 : 162;

  return {
    size: [width, height],
    radius: 14,
    fill: isDark ? (STATUS_BG[status] || STATUS_BG.pending) : (STATUS_BG_LIGHT[status] || STATUS_BG_LIGHT.pending),
    stroke: statusColor,
    lineDash: kindDash,
    lineWidth: status === 'running' ? 3 : isLoop ? 2.4 : 1.7,
    shadowColor: status === 'running' ? `${statusColor}66` : isDark ? 'rgba(0, 0, 0, 0.22)' : 'rgba(15, 23, 42, 0.08)',
    shadowBlur: status === 'running' ? 18 : 8,
    shadowOffsetY: 4,
    cursor: 'pointer',
    labelText: stepNodeLabel(step, expanded),
    labelFill: isDark ? '#e2e8f0' : '#1e293b',
    labelFontSize: 12,
    labelFontWeight: 650,
    labelLineHeight: 18,
    labelWordWrap: true,
    labelMaxWidth: width - 34,
    labelPlacement: 'center',
    badge: true,
    badges: [
      { text: statusLabel(status), placement: 'right-top', fill: `${statusColor}24`, stroke: statusColor, color: statusColor },
      { text: `${KIND_MARKS[executionKind] || KIND_MARKS.step} ${stepExecutionLabel(executionKind)}`, placement: 'left-top', fill: `${kindColor}22`, stroke: kindColor, color: kindColor },
    ],
    ports: [
      { key: 'left-top', placement: [0, 0.24], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'left', placement: [0, 0.5], r: 3.2, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'left-bottom', placement: [0, 0.76], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'right-top', placement: [1, 0.24], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'right', placement: [1, 0.5], r: 3.2, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
      { key: 'right-bottom', placement: [1, 0.76], r: 2.8, fill: statusColor, stroke: isDark ? '#0f172a' : '#fff' },
    ],
  };
}

function edgeStyle(edgeData: { status?: string; kind?: string; sourcePort?: string; targetPort?: string; laneOffset?: number } | undefined, isDark: boolean) {
  const laneOffset = Math.max(-42, Math.min(42, edgeData?.laneOffset || 0));
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


function GraphHelpTooltip() {
  return (
    <Tooltip
      placement="bottom"
      title={(
        <div className="graph-help-tooltip">
          <strong>How to read this graph</strong>
          <ul>
            <li><b>Arrow direction</b>: dependency / execution order flows left to right.</li>
            <li><b>Node color</b>: status. Gray pending, cyan running, green completed, red failed, yellow skipped or waiting.</li>
            <li><b>Node border style</b>: execution type. Solid agent, dashed script, dotted tool, long dashed loop.</li>
            <li><b>Edge color</b>: the target step status. Purple dashed edges are expanded loop-body structure.</li>
            <li><b>Separated lanes</b>: multiple dependencies use different ports and curves so overlapping lines remain readable.</li>
            <li><b>Click a node</b>: opens details. Click a loop node to expand/collapse its body.</li>
          </ul>
        </div>
      )}
    >
      <button type="button" className="graph-help-button" aria-label="How to read this graph">
        <QuestionCircleOutlined />
      </button>
    </Tooltip>
  );
}

export function GraphPanel({ snapshot, onSelect, theme }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void; theme: 'light' | 'dark' }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const [expandedLoopIDs, setExpandedLoopIDs] = useState<Set<string>>(() => new Set());

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

  useEffect(() => {
    if (!containerRef.current) return;

    const { nodes, edges, combos } = graphData;

    const config: GraphOptions = {
      container: containerRef.current,
      autoFit: 'view',
      padding: 40,
      grouping: true,
      data: {
        nodes,
        edges,
        combos,
      },
      layout: {
        type: 'dagre',
        rankdir: 'LR',
        nodesep: 72,
        ranksep: 180,
        controlPoints: true,
        edgeSep: 36,
        ranker: 'tight-tree',
      },
      node: {
        type: 'rect',
        style: (node: { data?: StepNodeData | LoopGroupNodeData }) => {
          const data = node.data;
          if (!data || !('step' in data)) return {};
          return stepNodeStyle(data.step, isDark, 'expanded' in data ? data.expanded : false);
        },
      },
      edge: {
        type: 'cubic-horizontal',
        style: (edge: { data?: { status?: string; kind?: string; sourcePort?: string; targetPort?: string; laneOffset?: number } }) => edgeStyle(edge.data, isDark),
      },
      combo: {
        type: 'rect',
        style: (combo: { data?: { step?: FormulaDashboardStep; bodyCount?: number } }) => ({
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
        }),
      },
      behaviors: [
        'zoom-canvas',
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

    graph.on('node:click', (evt: { target?: { id?: string } }) => {
      const nodeID = evt.target?.id;
      if (!nodeID) return;

      const datum = graph.getElementData(nodeID) as { data?: StepNodeData } | undefined;
      const step = resolveClickedStep(nodeID, datum?.data, snapshot);
      if (!step) return;

      if (shouldToggleLoopOnClick(nodeID, step)) {
        toggleLoop(step.id);
      }
      onSelect(step);
    });

    graph.render();

    return () => {
      graph.destroy();
      graphRef.current = null;
    };
  }, [graphData, isDark, onSelect, snapshot.steps]);

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
          <div className="graph-title-row">
            <h3>Execution graph</h3>
            <GraphHelpTooltip />
          </div>
          <p>Overview-first workflow map. Loops stay collapsed until you expand them, with details available in the inspector.</p>
          <div className="graph-header-metrics">
            <span>{snapshot.steps.length} nodes</span>
            <span>{snapshot.edges.length} edges</span>
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
          <div className="graph-legend-block" aria-label="Graph status legend">
            <span className="graph-legend-title">Status</span>
            <div className="graph-legend">
              {STATUS_LEGEND.map(status => (
                <span className={`legend-pill status ${status}`} key={status}>{status}</span>
              ))}
            </div>
          </div>
          <div className="graph-legend-block" aria-label="Graph execution type legend">
            <span className="graph-legend-title">Type</span>
            <div className="graph-legend kind-legend">
              {KIND_LEGEND.map(kind => (
                <span className={`legend-pill kind ${kind}`} key={kind}>
                  <i aria-hidden="true" /> {KIND_MARKS[kind]} {stepExecutionLabel(kind as ReturnType<typeof stepExecutionKind>)}
                </span>
              ))}
            </div>
          </div>
        </div>
      </div>
      <div className="graph-canvas g6-canvas">
        <div ref={containerRef} className="g6-container" />
      </div>
    </div>
  );
}
