import { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Input, Segmented, Select, Space, Spin, Tag, Tooltip, message } from 'antd';
import {
  BugOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  DeploymentUnitOutlined,
  ExclamationCircleOutlined,
  MinusCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';
import type { DashboardView, Issue, IssueStatus } from '../types';
import { api } from '../api';
import type { CreateIssueRequest, UpdateIssueRequest } from '../api';
import { IssueList } from './IssueList';
import { IssueDetail } from './IssueDetail';
import { DependencyGraph } from './DependencyGraph';
import { CreateIssueButton } from './IssueForm';

type Props = {
  theme: 'light' | 'dark';
  onThemeChange: (t: 'light' | 'dark') => void;
};

const STATUS_OPTIONS: { label: string; value: string }[] = [
  { label: 'All', value: '' },
  { label: 'Open', value: 'open' },
  { label: 'In Progress', value: 'in_progress' },
  { label: 'Blocked', value: 'blocked' },
  { label: 'Closed', value: 'closed' },
  { label: 'Deferred', value: 'deferred' },
];

const STATUS_ICONS: Record<string, React.ReactNode> = {
  open: <BugOutlined style={{ color: '#38bdf8' }} />,
  in_progress: <ClockCircleOutlined style={{ color: '#f59e0b' }} />,
  blocked: <ExclamationCircleOutlined style={{ color: '#f43f5e' }} />,
  closed: <CheckCircleOutlined style={{ color: '#22c55e' }} />,
  deferred: <MinusCircleOutlined style={{ color: '#94a3b8' }} />,
};

export function App({ theme, onThemeChange }: Props) {
  const [view, setView] = useState<DashboardView>('list');
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedIssue, setSelectedIssue] = useState<Issue | null>(null);
  const [statusFilter, setStatusFilter] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<Issue[] | null>(null);
  const [workspace, setWorkspace] = useState('');
  const [readOnly, setReadOnly] = useState(true);

  const loadIssues = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params: { status?: string } = {};
      if (statusFilter) params.status = statusFilter;
      const data = await api.listIssues(params);
      setIssues(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load issues');
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);

  const loadWorkspace = useCallback(async () => {
    try {
      const ws = await api.workspace();
      setWorkspace(ws.workspace);
      const health = await api.health();
      setReadOnly(health.read_only);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { loadIssues(); }, [loadIssues]);
  useEffect(() => { loadWorkspace(); }, [loadWorkspace]);

  // Load selected issue detail
  useEffect(() => {
    if (!selectedId) {
      setSelectedIssue(null);
      return;
    }
    api.issueDetail(selectedId)
      .then(setSelectedIssue)
      .catch(() => setSelectedIssue(null));
  }, [selectedId, issues]);

  // Search
  const handleSearch = useCallback(async (q: string) => {
    setSearchQuery(q);
    if (!q.trim()) {
      setSearchResults(null);
      return;
    }
    try {
      const results = await api.search(q);
      setSearchResults(results);
    } catch {
      setSearchResults(null);
    }
  }, []);

  const displayIssues = searchResults ?? issues;

  // Stats
  const stats = useMemo(() => {
    const counts: Record<string, number> = { total: 0, open: 0, in_progress: 0, blocked: 0, closed: 0 };
    for (const issue of issues) {
      counts.total++;
      counts[issue.status] = (counts[issue.status] || 0) + 1;
    }
    return counts;
  }, [issues]);

  const handleRefresh = useCallback(async () => {
    message.loading({ content: 'Refreshing...', key: 'refresh', duration: 0 });
    await loadIssues();
    message.success({ content: 'Refreshed', key: 'refresh' });
  }, [loadIssues]);

  // Mutation handlers
  const handleCreateIssue = useCallback(async (req: CreateIssueRequest) => {
    await api.createIssue(req);
    await loadIssues();
  }, [loadIssues]);

  const handleUpdateIssue = useCallback(async (id: string, req: UpdateIssueRequest) => {
    await api.updateIssue(id, req);
    await loadIssues();
    // Reload selected issue detail
    if (selectedId === id) {
      const updated = await api.issueDetail(id);
      setSelectedIssue(updated);
    }
  }, [loadIssues, selectedId]);

  return (
    <div className="beads-layout">
      {/* Header */}
      <div className="beads-header">
        <div className="beads-header-title">
          <DeploymentUnitOutlined />
          <span>Beads Dashboard</span>
          {workspace && <Tag color="default" style={{ fontSize: 11 }}>{workspace.split('/').pop()}</Tag>}
        </div>
        <div className="beads-header-actions">
          <Segmented
            value={view}
            onChange={(v) => setView(v as DashboardView)}
            options={[
              { label: <Space><UnorderedListOutlined />List</Space>, value: 'list' },
              { label: <Space><DeploymentUnitOutlined />Graph</Space>, value: 'graph' },
            ]}
          />
          <Tooltip title="Refresh">
            <Button icon={<ReloadOutlined />} onClick={handleRefresh} size="small" />
          </Tooltip>
          <Tooltip title={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}>
            <Button
              size="small"
              onClick={() => onThemeChange(theme === 'dark' ? 'light' : 'dark')}
            >
              {theme === 'dark' ? '☀️' : '🌙'}
            </Button>
          </Tooltip>
          <CreateIssueButton onCreate={handleCreateIssue} readOnly={readOnly} />
        </div>
      </div>

      {/* Content */}
      <div className="beads-content">
        <div className="beads-main">
          {/* Stats */}
          <div className="beads-stats">
            <div className="beads-stat-card"><strong>{stats.total}</strong>Total</div>
            <div className="beads-stat-card"><strong>{stats.open || 0}</strong>Open</div>
            <div className="beads-stat-card"><strong>{stats.in_progress || 0}</strong>Active</div>
            <div className="beads-stat-card"><strong>{stats.blocked || 0}</strong>Blocked</div>
            <div className="beads-stat-card"><strong>{stats.closed || 0}</strong>Done</div>
          </div>

          {/* Filters */}
          <div className="beads-filters">
            <Input
              prefix={<SearchOutlined />}
              placeholder="Search issues..."
              value={searchQuery}
              onChange={(e) => handleSearch(e.target.value)}
              allowClear
              style={{ width: 260 }}
            />
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              style={{ width: 150 }}
              options={STATUS_OPTIONS}
              placeholder="Filter by status"
            />
          </div>

          {/* Main content */}
          {loading ? (
            <div className="beads-empty"><Spin size="large" /></div>
          ) : error ? (
            <div className="beads-empty" style={{ color: '#f43f5e' }}>{error}</div>
          ) : view === 'list' ? (
            <IssueList
              issues={displayIssues}
              selectedId={selectedId}
              onSelect={setSelectedId}
              statusIcons={STATUS_ICONS}
            />
          ) : (
            <DependencyGraph
              issues={displayIssues}
              selectedId={selectedId}
              onSelect={setSelectedId}
              theme={theme}
            />
          )}
        </div>

        {/* Sidebar - Issue Detail */}
        <div className="beads-sidebar">
          <IssueDetail
            issue={selectedIssue}
            issues={issues}
            onClose={() => setSelectedId(null)}
            onNavigate={setSelectedId}
            onUpdate={handleUpdateIssue}
            onRefresh={loadIssues}
            readOnly={readOnly}
          />
        </div>
      </div>
    </div>
  );
}
