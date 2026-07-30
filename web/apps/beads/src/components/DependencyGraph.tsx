import { useEffect, useRef } from 'react';
import { Empty, Spin } from 'antd';
import { Graph } from '@antv/g6';
import type { Issue } from '../types';

const STATUS_COLORS: Record<string, string> = {
  open: '#38bdf8',
  in_progress: '#f59e0b',
  blocked: '#f43f5e',
  closed: '#22c55e',
  deferred: '#94a3b8',
};

type Props = {
  issues: Issue[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  theme: 'light' | 'dark';
};

export function DependencyGraph({ issues, selectedId, onSelect, theme }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);

  useEffect(() => {
    if (!containerRef.current || issues.length === 0) return;

    // Build nodes and edges from issues
    const nodeSet = new Set<string>();
    const edges: { source: string; target: string; type: string }[] = [];

    for (const issue of issues) {
      nodeSet.add(issue.id);
      const deps = issue.dependencies || [];
      for (const dep of deps) {
        // dep.issue_id depends on dep.depends_on_id
        if (nodeSet.has(dep.depends_on_id) || issues.some((i) => i.id === dep.depends_on_id)) {
          edges.push({ source: dep.issue_id, target: dep.depends_on_id, type: dep.type });
          nodeSet.add(dep.depends_on_id);
        }
      }
    }

    // If no edges, just show nodes without graph
    const nodes = Array.from(nodeSet).map((id) => {
      const issue = issues.find((i) => i.id === id);
      return {
        id,
        data: {
          label: id,
          title: issue?.title || id,
          status: issue?.status || 'open',
          priority: issue?.priority || 0,
        },
      };
    });

    const graphEdges = edges.map((e, i) => ({
      id: `e-${i}`,
      source: e.source,
      target: e.target,
      data: { type: e.type },
    }));

    // Destroy previous graph
    if (graphRef.current) {
      graphRef.current.destroy();
      graphRef.current = null;
    }

    const isDark = theme === 'dark';
    const container = containerRef.current;

    const graph = new Graph({
      container,
      width: container.clientWidth,
      height: container.clientHeight || 500,
      autoFit: 'view',
      data: {
        nodes,
        edges: graphEdges,
      },
      node: {
        type: 'rect',
        style: {
          size: (d: any) => {
            const label = d.data?.label || '';
            return [Math.max(80, label.length * 8 + 20), 32];
          },
          fill: (d: any) => STATUS_COLORS[d.data?.status] || '#94a3b8',
          stroke: (d: any) => d.id === selectedId ? '#6366f1' : (isDark ? '#334155' : '#cbd5e1'),
          lineWidth: (d: any) => d.id === selectedId ? 3 : 1,
          radius: 6,
          labelFontSize: 11,
          labelFill: isDark ? '#e2e8f0' : '#1e293b',
          labelPlacement: 'center',
          labelText: (d: any) => d.data?.label || d.id,
          cursor: 'pointer',
        },
      },
      edge: {
        type: 'line',
        style: {
          stroke: isDark ? '#475569' : '#94a3b8',
          lineWidth: 1.5,
          endArrow: true,
          endArrowSize: 6,
          labelFontSize: 10,
          labelFill: isDark ? '#94a3b8' : '#64748b',
          labelText: (d: any) => d.data?.type || '',
        },
      },
      layout: {
        type: 'dagre',
        rankdir: 'TB',
        nodesep: 40,
        ranksep: 60,
      },
      behaviors: ['drag-canvas', 'zoom-canvas'],
    });

    graph.on('node:click', (evt: any) => {
      const nodeId = evt.target?.id;
      if (nodeId) onSelect(nodeId);
    });

    graph.render();
    graphRef.current = graph;

    return () => {
      if (graphRef.current) {
        graphRef.current.destroy();
        graphRef.current = null;
      }
    };
  }, [issues, selectedId, theme, onSelect]);

  if (issues.length === 0) {
    return (
      <div className="beads-empty">
        <Empty description="No issues to display in graph" />
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className="beads-graph-container"
      style={{ position: 'relative' }}
    />
  );
}
