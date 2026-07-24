import type { TeamDashboardState } from './types';

type StateListener = (event: MessageEvent<string>) => void;

export type TeamEventSource = {
  readyState: number;
  onopen: (() => void) | null;
  onerror: (() => void) | null;
  addEventListener(type: 'state', listener: StateListener): void;
  removeEventListener(type: 'state', listener: StateListener): void;
  close(): void;
};

export type TeamEventSourceFactory = (url: string) => TeamEventSource;

export function connectTeamStateStream(
  onState: (state: TeamDashboardState) => void,
  onConnectionError: (message: string) => void,
  factory: TeamEventSourceFactory = url => new EventSource(url) as TeamEventSource,
) {
  const source = factory('/api/events');
  const handleState: StateListener = event => {
    try {
      onState(JSON.parse(event.data) as TeamDashboardState);
    } catch {
      onConnectionError('实时状态数据无效');
    }
  };
  source.addEventListener('state', handleState);
  source.onopen = () => onConnectionError('');
  source.onerror = () => onConnectionError('实时连接已断开，正在重连…');
  return () => {
    source.removeEventListener('state', handleState);
    source.close();
  };
}
