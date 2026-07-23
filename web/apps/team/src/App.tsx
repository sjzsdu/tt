import {
  BulbOutlined,
  CrownOutlined,
  DatabaseOutlined,
  MoonOutlined,
  SafetyCertificateOutlined,
  SunOutlined,
  TeamOutlined,
} from '@ant-design/icons';
import { Alert, Avatar, Badge, Button, Card, Empty, Skeleton, Space, Tag, Tooltip, Typography } from 'antd';
import { useEffect, useMemo, useRef } from 'react';
import { useTeamState } from './api';
import type { AppTheme } from './main';
import type { TeamAgent, TeamDashboardState, TeamEvent } from './types';

const { Paragraph, Text, Title } = Typography;

const VISIBLE_EVENT_TYPES = new Set([
  'user_message',
  'agent_message',
  'agent_yield',
  'agent_error',
  'final_answer',
  'memory_updated',
  'memory_update_failed',
  'round_started',
  'round_resumed',
  'round_interrupted',
  'round_failed',
  'round_completed',
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
  if (agent.finalizer) return { label: 'finalizer', color: 'green', icon: <CrownOutlined /> };
  if (agent.facilitator) return { label: 'facilitator', color: 'blue', icon: <SafetyCertificateOutlined /> };
  if (agent.memory_maintainer) return { label: 'memory', color: 'purple', icon: <DatabaseOutlined /> };
  return { label: 'member', color: 'default', icon: <TeamOutlined /> };
}

function MemberCard({ agent }: { agent: TeamAgent }) {
  const badge = agentLabel(agent);
  return (
    <div className="member-row">
      <Avatar className="member-avatar" shape="square">{initials(agent.id)}</Avatar>
      <div className="member-copy">
        <Text strong>@{agent.id}</Text>
        <Text type="secondary" ellipsis>{agent.role || agent.agent || 'Team member'}</Text>
        {agent.model && <Text className="member-model" type="secondary" ellipsis>{agent.model}</Text>}
      </div>
      <Tag icon={badge.icon} color={badge.color}>{badge.label}</Tag>
    </div>
  );
}

function eventKind(event: TeamEvent) {
  if (event.type === 'user_message') return 'user';
  if (event.type === 'final_answer') return 'final';
  if (event.type.includes('failed') || event.type === 'agent_error') return 'error';
  if (event.type === 'agent_message' || event.type === 'agent_yield') return 'agent';
  return 'system';
}

function speakerLabel(event: TeamEvent, kind: string) {
  const speaker = event.from || (kind === 'system' ? 'tt' : 'team');
  if (speaker === 'user') return 'You';
  if (speaker === 'tt') return 'Runtime';
  return `@${speaker}`;
}

function EventCard({ event }: { event: TeamEvent }) {
  const kind = eventKind(event);
  const speaker = event.from || (kind === 'system' ? 'tt' : 'team');
  const meta = [
    event.phase,
    event.wave ? `wave ${event.wave}` : '',
    formatTime(event.at),
  ].filter(Boolean);
  const content = event.content || event.error || event.type.replaceAll('_', ' ');

  return (
    <article className={`event event-${kind}`}>
      <Avatar className="event-avatar" shape="square">{initials(speaker)}</Avatar>
      <div className="event-content">
        <Space size={8} wrap className="event-heading">
          <Text strong>{speakerLabel(event, kind)}</Text>
          {event.type === 'final_answer' && <Tag color="green">final answer</Tag>}
          <Text className="event-meta" type="secondary">{meta.join(' · ')}</Text>
        </Space>
        <div className="event-message">{content}</div>
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
          <Text className="question-label"><BulbOutlined /> Current question</Text>
          <Paragraph className="question-text">{state.round.question}</Paragraph>
        </div>
      )}
      {events.length
        ? events.map(event => <EventCard key={event.id} event={event} />)
        : <Empty description="Waiting for team activity…" />}
    </div>
  );
}

function MemoryPanel({ state }: { state: TeamDashboardState }) {
  return (
    <Card
      className="side-card memory-card"
      title={<Space><DatabaseOutlined />Team memory</Space>}
      extra={<Tag>v{state.memory.version || 0}</Tag>}
    >
      <div className="memory-meta">
        <Text type="secondary">
          {state.memory.updated_at
            ? `Updated ${new Date(state.memory.updated_at).toLocaleString()}`
            : 'No durable update yet'}
        </Text>
        {state.memory.source_round
          ? <Text type="secondary">Source: round {state.memory.source_round}</Text>
          : null}
      </div>
      <pre className="memory-content">{state.memory.content || 'No team memory yet.'}</pre>
    </Card>
  );
}

export function App({ theme, onThemeChange }: Props) {
  const { state, error } = useTeamState();
  const runtimeError = state?.round?.error || state?.thread.error || '';
  const status = state?.thread.status || 'loading';
  const running = status === 'running';

  return (
    <main className="app-shell">
      <header className="app-header">
        <div>
          <div className="eyebrow"><span className="brand-dot" />tt collaborative runtime</div>
          <Title className="team-title" level={1}>{state?.team.title || state?.team.team || 'Team'}</Title>
          <Paragraph className="team-subtitle">
            {state?.team.description || state?.thread.id || 'Connecting to the team thread…'}
          </Paragraph>
          {state?.team.description && <Text className="thread-id" type="secondary">{state.thread.id}</Text>}
        </div>
        <Space align="start">
          <Tooltip title={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}>
            <Button
              aria-label="Toggle color theme"
              shape="circle"
              icon={theme === 'dark' ? <SunOutlined /> : <MoonOutlined />}
              onClick={() => onThemeChange(theme === 'dark' ? 'light' : 'dark')}
            />
          </Tooltip>
          <div className={`runtime-status status-${status}`}>
            <Badge status={running ? 'processing' : status === 'failed' ? 'error' : 'default'} />
            <Text strong>{status}</Text>
            {state?.round?.phase && <Text type="secondary">· {state.round.phase}</Text>}
          </div>
        </Space>
      </header>

      {(error || runtimeError) && (
        <Alert
          className="connection-alert"
          type="error"
          showIcon
          message={error ? `Dashboard disconnected: ${error}` : runtimeError}
        />
      )}

      {!state ? (
        <Card className="loading-card"><Skeleton active paragraph={{ rows: 10 }} /></Card>
      ) : (
        <div className="dashboard-grid">
          <Card
            className="room-card"
            title={<Space><TeamOutlined />Public room</Space>}
            extra={
              <Text type="secondary">
                {state.round ? `round ${state.round.number} · ${state.round.phase || state.round.status}` : 'no active round'}
              </Text>
            }
          >
            <Discussion state={state} />
          </Card>

          <aside className="side-column">
            <Card
              className="side-card"
              title={<Space><TeamOutlined />Team members</Space>}
              extra={<Tag>{state.agents.length}</Tag>}
            >
              <div className="member-list">
                {state.agents.map(agent => <MemberCard key={agent.id} agent={agent} />)}
              </div>
            </Card>
            <MemoryPanel state={state} />
          </aside>
        </div>
      )}
    </main>
  );
}
