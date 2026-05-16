import { useEffect, useMemo, useState } from 'react';
import {
  App as AntdApp,
  Button,
  Card,
  Empty,
  Modal,
  Segmented,
  Space,
  Statistic,
  Tag,
  Timeline,
  Tooltip,
} from 'antd';
import {
  ApartmentOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  ExpandOutlined,
  LoadingOutlined,
  NodeIndexOutlined,
  PartitionOutlined,
  UnorderedListOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { marked } from 'marked';
import { api } from '../api';
import type {
  DashboardView,
  FormulaDashboardMessage,
  FormulaDashboardSnapshot,
  FormulaDashboardStep,
} from '../types';

const statusOrder: Record<string, number> = {
  running: 0,
  failed: 1,
  pending: 2,
  skipped: 3,
  completed: 4,
};

const statusTone: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  completed: 'success',
  failed: 'error',
  skipped: 'warning',
};

function currentView(): DashboardView {
  return localStorage.getItem('formula-dashboard-view') === 'graph' ? 'graph' : 'list';
}

function formatDate(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
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
    case 'failed':
      return <WarningOutlined />;
    default:
      return <ClockCircleOutlined />;
  }
}

function renderMarkdown(markdown: string) {
	return marked.parse(markdown || '') as string;
}

function graphShortId(id: string) {
  return id.includes('.') ? id.slice(id.lastIndexOf('.') + 1) : id;
}

function computeGraphLayout(snapshot: FormulaDashboardSnapshot) {
  const grouped = new Map<number, FormulaDashboardStep[]>();
  for (const step of snapshot.steps) {
    const depth = step.depth || 0;
    if (!grouped.has(depth)) grouped.set(depth, []);
    grouped.get(depth)!.push(step);
  }

  const depths = Array.from(grouped.keys()).sort((a, b) => a - b);
  for (const depth of depths) {
    grouped.get(depth)!.sort((a, b) => a.index - b.index);
  }

  const nodeWidth = 280;
  const nodeHeight = 112;
  const colGap = 92;
  const rowGap = 42;
  const paddingX = 28;
  const paddingY = 28;

  const nodes = snapshot.steps.map(step => {
    const depth = step.depth || 0;
    const column = grouped.get(depth) || [];
    const row = column.findIndex(candidate => candidate.id === step.id);
    return {
      step,
      x: paddingX + depth * (nodeWidth + colGap),
      y: paddingY + row * (nodeHeight + rowGap),
      width: nodeWidth,
      height: nodeHeight,
    };
  });

  const nodeByID = new Map(nodes.map(node => [node.step.id, node]));
  const lines = snapshot.edges.flatMap(edge => {
    const from = nodeByID.get(edge.from);
    const to = nodeByID.get(edge.to);
    if (!from || !to) return [];
    const startX = from.x + from.width;
    const startY = from.y + from.height / 2;
    const endX = to.x;
    const endY = to.y + to.height / 2;
    const midX = startX + (endX - startX) / 2;
    return [{ edge, path: `M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}` }];
  });

  const width = Math.max(720, paddingX * 2 + Math.max(1, depths.length) * nodeWidth + Math.max(0, depths.length-1) * colGap);
  const maxRows = Math.max(1, ...depths.map(depth => grouped.get(depth)?.length || 0));
  const height = Math.max(360, paddingY * 2 + maxRows * nodeHeight + Math.max(0, maxRows-1) * rowGap);

  return { widths: { nodeWidth, nodeHeight }, depths, nodes, lines, width, height };
}

function GraphPanel({ snapshot, onSelect }: { snapshot: FormulaDashboardSnapshot; onSelect: (step: FormulaDashboardStep) => void }) {
  const layout = useMemo(() => computeGraphLayout(snapshot), [snapshot]);
  const running = snapshot.steps.find(step => step.status === 'running');

  if (!snapshot.steps.length) {
    return <Empty description="No executable steps" />;
  }

  return (
    <div className="graph-panel">
      <div className="graph-header">
        <div>
          <h3>Execution graph</h3>
          <p>Live flow map with luminous stages, dependency paths, and direct click-to-inspect details.</p>
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
      <div className="graph-canvas">
        <div className="graph-stage-rails" aria-hidden="true">
          {layout.depths.map(depth => (
            <div
              key={depth}
              className="graph-stage-rail"
              style={{ left: 28 + depth * (layout.widths.nodeWidth + 92), width: layout.widths.nodeWidth }}
            />
          ))}
        </div>
        <div className="graph-board" style={{ width: layout.width, height: layout.height }}>
          <svg className="graph-svg-layer" width={layout.width} height={layout.height} viewBox={`0 0 ${layout.width} ${layout.height}`}>
            <defs>
              <marker id="graph-arrow" markerWidth="10" markerHeight="10" refX="8" refY="5" orient="auto">
                <path d="M 0 0 L 10 5 L 0 10 z" fill="rgba(148, 163, 184, 0.82)" />
              </marker>
            </defs>
            {layout.lines.map(({ edge, path }) => (
              <path key={`${edge.from}-${edge.to}`} d={path} className={`graph-edge ${edge.type || 'default'}`} markerEnd="url(#graph-arrow)" />
            ))}
          </svg>

          {layout.depths.map(depth => (
            <div
              key={depth}
              className="graph-column-label"
              style={{ left: 28 + depth * (layout.widths.nodeWidth + 92), width: layout.widths.nodeWidth }}
            >
              Stage {depth + 1}
            </div>
          ))}

          {layout.nodes.map(node => (
            <button
              key={node.step.id}
              type="button"
              className={`graph-node ${node.step.status}`}
              style={{ left: node.x, top: node.y, width: node.width, minHeight: node.height }}
              onClick={() => onSelect(node.step)}
            >
              <div className="graph-node-topline">
                <div className="graph-node-id">{graphShortId(node.step.id)}</div>
                <span className={`graph-node-state ${node.step.status}`}>{node.step.status}</span>
              </div>
              <strong>{node.step.title}</strong>
              <p>{node.step.description || node.step.notes || 'Structured execution step in the formula pipeline.'}</p>
              <div className="graph-node-meta">
                <span><PartitionOutlined /> {node.step.agent || 'default agent'}</span>
                {!!node.step.depends_on?.length && <span><ExpandOutlined /> {node.step.depends_on.length} deps</span>}
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function MarkdownOutput({ content }: { content: string }) {
  const html = useMemo(() => renderMarkdown(content), [content]);

  return (
    <article className="markdown-body formula-markdown-output" dangerouslySetInnerHTML={{ __html: html }} />
  );
}

function StepCard({ step, onSelect }: { step: FormulaDashboardStep; onSelect: (step: FormulaDashboardStep) => void }) {
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
      <div className="step-chip-row">
        {step.agent && <span className="step-chip">agent · {step.agent}</span>}
        {step.model && <span className="step-chip">model · {step.model}</span>}
        {!!step.depends_on?.length && <span className="step-chip">deps · {step.depends_on.length}</span>}
        {step.output_key && <span className="step-chip">output · {step.output_key}</span>}
      </div>
    </button>
  );
}

function StepDetailModal({ step, open, onClose }: { step: FormulaDashboardStep | null; open: boolean; onClose: () => void }) {
  if (!step) return null;
  const metadataEntries = Object.entries(step.metadata || {});

  return (
    <Modal open={open} onCancel={onClose} footer={null} width={960} title={step.title} className="step-modal">
      <div className="step-modal-grid">
        <div className="step-modal-main">
          <Space wrap size={[8, 8]}>
            <Tag color={statusTone[step.status] || 'default'}>{step.status}</Tag>
            {step.agent && <Tag>{step.agent}</Tag>}
            {step.model && <Tag>{step.model}</Tag>}
            {step.type && <Tag>{step.type}</Tag>}
            {step.priority && <Tag>P{step.priority}</Tag>}
          </Space>

          {(step.description || step.notes) && (
            <section>
              <h4>Brief</h4>
              {step.description && <p>{step.description}</p>}
              {step.notes && <pre className="code-block">{step.notes}</pre>}
            </section>
          )}

          {step.output && (
            <section>
              <h4>Output</h4>
              <pre className="code-block">{step.output}</pre>
            </section>
          )}

          {step.error && (
            <section>
              <h4>Error</h4>
              <pre className="code-block error-block">{step.error}</pre>
            </section>
          )}
        </div>

        <aside className="step-modal-side">
          <Card size="small" title="Runtime">
            <ul className="meta-list">
              <li><span>ID</span><strong>{step.id}</strong></li>
              <li><span>Started</span><strong>{formatDate(step.started_at)}</strong></li>
              <li><span>Finished</span><strong>{formatDate(step.finished_at)}</strong></li>
              <li><span>Duration</span><strong>{formatDuration(step.duration_ms)}</strong></li>
              <li><span>Session</span><strong>{step.session || '—'}</strong></li>
              <li><span>Execution</span><strong>{step.execution || '—'}</strong></li>
              <li><span>Condition</span><strong>{step.condition || '—'}</strong></li>
            </ul>
          </Card>
          <Card size="small" title="Dependencies">
            {step.depends_on?.length ? (
              <ul className="pill-list">
                {step.depends_on.map(dep => <li key={dep}>{dep}</li>)}
              </ul>
            ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No dependencies" />}
          </Card>
          <Card size="small" title="Inputs & Labels">
            {!!step.input_ctx?.length && (
              <>
                <div className="card-subtitle">Input context</div>
                <ul className="pill-list">{step.input_ctx.map(key => <li key={key}>{key}</li>)}</ul>
              </>
            )}
            {!!step.labels?.length && (
              <>
                <div className="card-subtitle">Labels</div>
                <ul className="pill-list">{step.labels.map(label => <li key={label}>{label}</li>)}</ul>
              </>
            )}
            {!step.input_ctx?.length && !step.labels?.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No labels or inputs" />}
          </Card>
          {metadataEntries.length > 0 && (
            <Card size="small" title="Metadata">
              <ul className="meta-list compact">
                {metadataEntries.map(([key, value]) => (
                  <li key={key}><span>{key}</span><strong>{value}</strong></li>
                ))}
              </ul>
            </Card>
          )}
        </aside>
      </div>
    </Modal>
  );
}

export function App() {
  const { message } = AntdApp.useApp();
  const [snapshot, setSnapshot] = useState<FormulaDashboardSnapshot | null>(null);
  const [view, setView] = useState<DashboardView>(currentView());
  const [selectedStep, setSelectedStep] = useState<FormulaDashboardStep | null>(null);
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
            setSnapshot(msg.state);
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
      failed: counts.failed || 0,
      logs: snapshot.logs.length,
    };
  }, [snapshot]);

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
          <h1>{snapshot.recipe_name}</h1>
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
          <Tooltip title={snapshot.workspace_dir || 'Formula sessions stored under project-local .tt'}>
            <Button icon={<NodeIndexOutlined />}>.tt sessions</Button>
          </Tooltip>
        </div>
      </section>

      <section className="stats-grid">
        <Card><Statistic title="Run status" value={snapshot.status} prefix={statusIcon(snapshot.status)} /></Card>
        <Card><Statistic title="Steps" value={summary.steps} /></Card>
        <Card><Statistic title="Running / Completed" value={`${summary.running} / ${summary.completed}`} /></Card>
        <Card><Statistic title="Failures / Logs" value={`${summary.failed} / ${summary.logs}`} /></Card>
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
          </Card>
          <Card className="console-card" title="Final output">
            {snapshot.final_output ? (
              <div className="final-output-shell">
                <div className="final-output-kicker">Rendered report</div>
                <MarkdownOutput content={snapshot.final_output} />
              </div>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Waiting for final output" />
            )}
          </Card>
        </aside>
      </section>

      <StepDetailModal step={selectedStep} open={!!selectedStep} onClose={() => setSelectedStep(null)} />
    </main>
  );
}
