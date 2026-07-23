import { useEffect, useState } from 'react';
import type { TeamDashboardState } from './types';

const POLL_INTERVAL_MS = 800;

export function useTeamState() {
  const [state, setState] = useState<TeamDashboardState | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let stopped = false;
    let timer: number | undefined;
    let controller: AbortController | undefined;

    const refresh = async () => {
      controller?.abort();
      controller = new AbortController();
      try {
        const response = await fetch('/api/state', {
          cache: 'no-store',
          signal: controller.signal,
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const next = await response.json() as TeamDashboardState;
        if (!stopped) {
          setState(next);
          setError('');
          document.title = `${next.team.team || 'team'} · tt`;
        }
      } catch (cause) {
        if (!stopped && !(cause instanceof DOMException && cause.name === 'AbortError')) {
          setError(cause instanceof Error ? cause.message : String(cause));
        }
      } finally {
        if (!stopped) timer = window.setTimeout(refresh, POLL_INTERVAL_MS);
      }
    };

    void refresh();
    return () => {
      stopped = true;
      controller?.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, []);

  return { state, error };
}
