import { CheckCircleOutlined, ClockCircleOutlined, LoadingOutlined, WarningOutlined } from '@ant-design/icons';

export const statusOrder: Record<string, number> = {
  running: 0,
  ready: 1,
  waiting_input: 2,
  waiting_dependency: 3,
  blocked: 4,
  failed: 5,
  interrupted: 6,
  pending: 7,
  skipped: 8,
  completed: 9,
};

export const statusTone: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  ready: 'cyan',
  waiting_input: 'warning',
  waiting_dependency: 'default',
  blocked: 'error',
  completed: 'success',
  failed: 'error',
  interrupted: 'error',
  skipped: 'warning',
};

export function formatDuration(ms?: number) {
  if (!ms) return '—';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`;
}

export function statusIcon(status: string) {
  switch (status) {
    case 'completed':
      return <CheckCircleOutlined />;
    case 'running':
      return <LoadingOutlined />;
    case 'waiting_input':
    case 'waiting_dependency':
    case 'ready':
      return <ClockCircleOutlined />;
    case 'failed':
    case 'interrupted':
    case 'blocked':
      return <WarningOutlined />;
    default:
      return <ClockCircleOutlined />;
  }
}

export function graphShortId(id: string) {
  return id.includes('.') ? id.slice(id.lastIndexOf('.') + 1) : id;
}

export function statusLabel(status: string) {
  return status.replace(/_/g, ' ');
}

export function activityShortId(id: string) {
  const iter = id.match(/\.iter(\d+)\.([^.]*)$/);
  if (iter) return `iter ${iter[1]} · ${iter[2]}`;
  return graphShortId(id);
}
