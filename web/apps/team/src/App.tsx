import {
  BulbOutlined,
  CheckCircleOutlined,
  CrownOutlined,
  DatabaseOutlined,
  MoonOutlined,
  SafetyCertificateOutlined,
  SendOutlined,
  SnippetsOutlined,
  StopOutlined,
  SunOutlined,
  SyncOutlined,
  TeamOutlined,
} from '@ant-design/icons';
import { Alert, Avatar, Badge, Button, Card, Empty, Input, Skeleton, Space, Tag, Tooltip, Typography } from 'antd';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTeamState } from './api';
import { renderSafeMarkdown } from './markdown';
import type { AppTheme } from './main';
import type { TeamAgent, TeamBlackboardEntry, TeamDashboardState, TeamEvent } from './types';

const { Paragraph, Text, Title } = Typography;

const VISIBLE_EVENT_TYPES = new Set([
  'user_message',
  'agent_message',
  'agent_yield',
  'agent_error',
  'final_answer',
  'memory_updated',
  'memory_proposed',
  'memory_rolled_back',
  'memory_update_failed',
  'round_started',
  'round_resumed',
  'round_interrupted',
  'round_failed',
  'round_completed',
  'convergence_reached',
  'forced_stop',
]);

type Props = {
  theme: AppTheme;
  onThemeChange: (theme: AppTheme) => void;
};

function initials(value?: string) {
  return (value || '?')
    .split(/[-_\s]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map(part => part[0])
    .join('')
    .toUpperCase();
}

function formatTime(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function agentLabel(agent: TeamAgent) {
  if (agent.finalizer) return { label: '总结者', color: 'green', icon: <CrownOutlined /> };
  if (agent.facilitator) return { label: '主持人', color: 'blue', icon: <SafetyCertificateOutlined /> };
  if (agent.memory_maintainer) return { label: '记忆管理', color: 'purple', icon: <DatabaseOutlined /> };
  return { label: '成员', color: 'default', icon: <TeamOutlined /> };
}

function MemberCard({ agent }: { agent: TeamAgent }) {
  const badge = agentLabel(agent);
  return (
    <div className="member-row">
      <Avatar className="member-avatar" shape="square">{initials(agent.id)}</Avatar>
      <div className="member-copy">
        <Text strong>@{agent.id}</Text>
        <Text type="secondary" ellipsis>{agent.role || agent.agent || '团队成员'}</Text>
        {agent.model && <Text className="member-model" type="secondary" ellipsis>{agent.model}</Text>}
      </div>
      <Tag icon={badge.icon} color={badge.color}>{badge.label}</Tag>
    </div>
  );
}

function eventKind(event: TeamEvent) {
  if (event.type === 'user_message') return 'user';
  if (event.type === 'final_answer' || event.type === 'convergence_reached') return 'final';
  if (event.type === 'forced_stop') return 'error';
  if (event.type.includes('failed') || event.type === 'agent_error') return 'error';
  if (event.type === 'agent_message' || event.type === 'agent_yield') return 'agent';
  return 'system';
}

function speakerLabel(event: TeamEvent, kind: string) {
  const speaker = event.from || (kind === 'system' ? '系统' : '团队');
  if (speaker === 'user') return '你';
  if (speaker === 'tt') return '运行时';
  return `@${speaker}`;
}

function signalTag(signal?: string) {
  switch (signal) {
    case 'agree':
      return <Tag color="green">同意</Tag>;
    case 'object':
      return <Tag color="red">异议</Tag>;
    case 'yield':
      return <Tag>让出</Tag>;
    case 'propose_final':
      return <Tag color="blue">建议总结</Tag>;
    case 'resolved':
      return <Tag color="cyan">异议已解决</Tag>;
    default:
      return null;
  }
}

function eventText(event: TeamEvent) {
  if (event.type === 'convergence_reached') return '团队已达到收敛条件，准备生成最终答案。';
  if (event.type === 'forced_stop') {
    if (event.content === 'max_agent_turns') return '已达到 Agent turn 上限，运行时将保留未解决分歧并强制总结。';
    if (event.content === 'max_wall_time') return '已达到最大运行时间，本轮已中断并保存，可稍后恢复。';
    if (event.content === 'unresolved_objections_no_progress') return '仍有未解决异议，但当前没有可继续激活的成员，运行时将强制总结。';
  }
  return event.content || event.error || event.type.replaceAll('_', ' ');
}

function MarkdownContent({ content }: { content: string }) {
  const html = useMemo(() => renderSafeMarkdown(content), [content]);
  return <div className="event-message markdown-body" dangerouslySetInnerHTML={{ __html: html }} />;
}

function EventCard({ event }: { event: TeamEvent }) {
  const kind = eventKind(event);
  const speaker = event.from || (kind === 'system' ? '系统' : '团队');
  const meta = [
    event.phase,
    event.wave ? `第${event.wave}轮` : '',
    event.metrics?.turn ? `turn ${event.metrics.turn}` : '',
    event.metrics?.model || '',
    event.metrics?.duration_ms !== undefined ? `${event.metrics.duration_ms}ms` : '',
    formatTime(event.at),
  ].filter(Boolean);
  const content = eventText(event);

  return (
    <article className={`event event-${kind}`}>
      <Avatar className="event-avatar" shape="square">{initials(speaker)}</Avatar>
      <div className="event-content">
        <Space size={8} wrap className="event-heading">
          <Text strong>{speakerLabel(event, kind)}</Text>
          {event.type === 'final_answer' && <Tag color="green">最终答案</Tag>}
          {signalTag(event.signal)}
          <Text className="event-meta" type="secondary">{meta.join(' · ')}</Text>
        </Space>
        {kind === 'agent' || kind === 'final'
          ? <MarkdownContent content={content} />
          : <div className="event-message">{content}</div>}
      </div>
    </article>
  );
}

function Discussion({ state }: { state: TeamDashboardState }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const events = useMemo(() => state.events.filter(event => {
    if (!VISIBLE_EVENT_TYPES.has(event.type)) return false;
    if (event.type === 'round_started' && state.round?.question) return false;
    if (event.type === 'user_message' && state.round && event.round === state.round.number) return false;
    return true;
  }), [state.events, state.round]);

  useEffect(() => {
    const root = scrollRef.current;
    if (!root) return;
    const nearBottom = root.scrollHeight - root.scrollTop - root.clientHeight < 180;
    if (nearBottom) root.scrollTo({ top: root.scrollHeight, behavior: 'smooth' });
  }, [events.length]);

  return (
    <div ref={scrollRef} className="discussion">
      {state.round?.question && (
        <div className="question-card">
          <Text className="question-label"><BulbOutlined /> 当前问题</Text>
          <Paragraph className="question-text">{state.round.question}</Paragraph>
        </div>
      )}
      {events.length
        ? events.map(event => <EventCard key={event.id} event={event} />)
        : <Empty description="等待团队活动…" />}
    </div>
  );
}

type TeamControlsProps = {
  state: TeamDashboardState;
  pendingAction: string;
  onFollowUp: (message: string) => Promise<boolean>;
  onResume: () => Promise<boolean>;
  onStop: () => Promise<boolean>;
};

function TeamControls({ state, pendingAction, onFollowUp, onResume, onStop }: TeamControlsProps) {
  const [message, setMessage] = useState('');
  const controls = state.controls || {
    busy: false,
    can_follow_up: false,
    can_resume: false,
    can_stop: false,
  };
  const submit = async () => {
    const value = message.trim();
    if (!value || !controls.can_follow_up) return;
    if (await onFollowUp(value)) setMessage('');
  };
  return (
    <div className="team-controls">
      <Input.TextArea
        value={message}
        onChange={event => setMessage(event.target.value)}
        onPressEnter={event => {
          if (!event.shiftKey) {
            event.preventDefault();
            void submit();
          }
        }}
        placeholder={controls.can_follow_up ? '向团队提出后续问题…' : '当前轮次结束后可以继续追问'}
        disabled={!controls.can_follow_up || Boolean(pendingAction)}
        autoSize={{ minRows: 2, maxRows: 5 }}
      />
      <Space wrap>
        <Button
          type="primary"
          icon={<SendOutlined />}
          disabled={!controls.can_follow_up || !message.trim()}
          loading={pendingAction === 'follow-up'}
          onClick={() => void submit()}
        >
          追问
        </Button>
        {controls.can_resume && (
          <Button
            icon={<SyncOutlined />}
            loading={pendingAction === 'resume'}
            onClick={() => void onResume()}
          >
            恢复本轮
          </Button>
        )}
        {controls.can_stop && (
          <Button
            danger
            icon={<StopOutlined />}
            loading={pendingAction === 'stop'}
            onClick={() => void onStop()}
          >
            停止
          </Button>
        )}
        {controls.busy && <Text type="secondary">团队正在协作，进度会实时更新。</Text>}
      </Space>
    </div>
  );
}

function MemoryPanel({
  state,
  pendingAction,
  onRetry,
  onRollback,
}: {
  state: TeamDashboardState;
  pendingAction: string;
  onRetry: () => Promise<boolean>;
  onRollback: (version: number) => Promise<boolean>;
}) {
  const versions = state.memory_review?.versions || [];
  const proposal = state.memory_review?.proposals?.[0];
  return (
    <Card
      className="side-card memory-card"
      title={<Space><DatabaseOutlined />团队记忆</Space>}
      extra={<Tag>v{state.memory.version || 0}</Tag>}
    >
      <div className="memory-meta">
        <Text type="secondary">
          {state.memory.updated_at
            ? `更新于 ${new Date(state.memory.updated_at).toLocaleString()}`
            : '暂无持久化更新'}
        </Text>
        {state.memory.source_round
          ? <Text type="secondary">来源: 第 {state.memory.source_round} 轮 · events {state.memory.source_events?.join(', ') || '—'}</Text>
          : null}
      </div>
      <pre className="memory-content">{state.memory.content || '暂无团队记忆。'}</pre>
      {proposal && (
        <details className="memory-review">
          <summary>最近提案 · {proposal.status} · v{proposal.base_version} → v{proposal.proposed_version}</summary>
          <pre className="memory-diff">{proposal.diff}</pre>
          {proposal.error && <Text type="danger">{proposal.error}</Text>}
        </details>
      )}
      <Space wrap className="memory-actions">
        {state.controls.can_retry_memory && (
          <Button
            size="small"
            icon={<SyncOutlined />}
            loading={pendingAction === 'memory-retry'}
            onClick={() => void onRetry()}
          >
            重试记忆更新
          </Button>
        )}
        {state.controls.can_rollback_memory && versions
          .filter(version => version.version !== state.memory.version)
          .slice(0, 3)
          .map(version => (
            <Button
              key={version.version}
              size="small"
              loading={pendingAction === `memory-rollback-${version.version}`}
              onClick={() => void onRollback(version.version)}
            >
              回滚到 v{version.version}
            </Button>
          ))}
      </Space>
    </Card>
  );
}

const blackboardKindLabel: Record<TeamBlackboardEntry['kind'], string> = {
  fact: '事实',
  proposal: '提案',
  question: '问题',
  decision: '决策',
  objection: '异议',
  artifact: '产物',
};

function BlackboardPanel({ state }: { state: TeamDashboardState }) {
  const entries = state.blackboard?.entries || [];
  return (
    <Card
      className="side-card blackboard-card"
      title={<Space><SnippetsOutlined />工作黑板</Space>}
      extra={<Tag>{entries.length}</Tag>}
    >
      {entries.length ? (
        <div className="blackboard-list">
          {entries.map(entry => (
            <div className={`blackboard-entry blackboard-${entry.status}`} key={`${entry.kind}:${entry.key}`}>
              <Space size={5} wrap>
                <Tag color={entry.status === 'resolved' ? 'default' : 'blue'}>
                  {blackboardKindLabel[entry.kind]}
                </Tag>
                <Text code>{entry.key}</Text>
                {entry.status === 'resolved' && <CheckCircleOutlined className="blackboard-resolved-icon" />}
              </Space>
              <div className="blackboard-content">{entry.content || '已解决'}</div>
              <Text type="secondary" className="blackboard-provenance">
                @{entry.updated_by || 'runtime'} · event {entry.updated_at_event_id} · {entry.revisions.length} 次修订
              </Text>
            </div>
          ))}
        </div>
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本轮暂无结构化条目" />}
    </Card>
  );
}

export function App({ theme, onThemeChange }: Props) {
  const { state, error, actionError, pendingAction, followUp, resume, stop, retryMemory, rollbackMemory } = useTeamState();
  const runtimeError = state?.round?.error || state?.thread.error || '';
  const status = state?.thread.status || 'loading';
  const running = status === 'running';
  const collaboration = state?.round?.collaboration;
  const openObjections = collaboration?.objections?.filter(objection => !objection.resolved).length || 0;

  const statusLabel: Record<string, string> = {
    loading: '连接中',
    idle: '空闲',
    running: '运行中',
    failed: '失败',
    completed: '已完成',
    interrupted: '已中断',
  };

  return (
    <main className="app-shell">
      <header className="app-header">
        <div>
          <div className="eyebrow"><span className="brand-dot" />tt 团队协作运行时</div>
          <Title className="team-title" level={1}>{state?.team.title || state?.team.team || '团队'}</Title>
          <Paragraph className="team-subtitle">
            {state?.team.description || state?.thread.id || '正在连接团队线程…'}
          </Paragraph>
          {state?.team.description && <Text className="thread-id" type="secondary">{state.thread.id}</Text>}
        </div>
        <Space align="start">
          <Tooltip title={`切换到${theme === 'dark' ? '浅色' : '深色'}主题`}>
            <Button
              aria-label="切换主题"
              shape="circle"
              icon={theme === 'dark' ? <SunOutlined /> : <MoonOutlined />}
              onClick={() => onThemeChange(theme === 'dark' ? 'light' : 'dark')}
            />
          </Tooltip>
          <div className={`runtime-status status-${status}`}>
            <Badge status={running ? 'processing' : status === 'failed' ? 'error' : 'default'} />
            <Text strong>{statusLabel[status] || status}</Text>
            {state?.round?.phase && <Text type="secondary">· {state.round.phase}</Text>}
          </div>
        </Space>
      </header>

      {(error || runtimeError) && (
        <Alert
          className="connection-alert"
          type="error"
          showIcon
          message={error ? `仪表盘断开连接: ${error}` : runtimeError}
        />
      )}
      {actionError && (
        <Alert
          className="connection-alert"
          type="warning"
          showIcon
          message={`操作失败: ${actionError}`}
        />
      )}

      {!state ? (
        <Card className="loading-card"><Skeleton active paragraph={{ rows: 10 }} /></Card>
      ) : (
        <div className="dashboard-grid">
          <Card
            className="room-card"
            title={<Space><TeamOutlined />公共讨论区</Space>}
            extra={
              <Space size={6} wrap>
                {collaboration && <Tag>turn {collaboration.turn_count}</Tag>}
                {openObjections > 0 && <Tag color="red">{openObjections} 个未解决异议</Tag>}
                <Text type="secondary">
                  {state.round ? `第 ${state.round.number} 轮 · ${state.round.phase || state.round.status}` : '无活跃轮次'}
                </Text>
              </Space>
            }
          >
            <Discussion state={state} />
            <TeamControls
              state={state}
              pendingAction={pendingAction}
              onFollowUp={followUp}
              onResume={resume}
              onStop={stop}
            />
          </Card>

          <aside className="side-column">
            <Card
              className="side-card"
              title={<Space><TeamOutlined />团队成员</Space>}
              extra={<Tag>{state.agents.length}</Tag>}
            >
              <div className="member-list">
                {state.agents.map(agent => <MemberCard key={agent.id} agent={agent} />)}
              </div>
            </Card>
            <BlackboardPanel state={state} />
            <MemoryPanel
              state={state}
              pendingAction={pendingAction}
              onRetry={retryMemory}
              onRollback={rollbackMemory}
            />
          </aside>
        </div>
      )}
    </main>
  );
}
