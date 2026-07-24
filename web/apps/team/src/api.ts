import { useCallback, useEffect, useState } from 'react';
import { connectTeamStateStream } from './team-stream';
import type { TeamDashboardState } from './types';

export function useTeamState() {
  const [state, setState] = useState<TeamDashboardState | null>(null);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');
  const [pendingAction, setPendingAction] = useState('');

  useEffect(() => {
    let stopped = false;
    let receivedStreamState = false;
    const loadInitial = async () => {
      try {
        const response = await fetch('/api/state', { cache: 'no-store' });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const next = await response.json() as TeamDashboardState;
        if (!stopped && !receivedStreamState) {
          setState(next);
          document.title = `${next.team.team || 'team'} · tt`;
        }
      } catch (cause) {
        if (!stopped) {
          setError(cause instanceof Error ? cause.message : String(cause));
        }
      }
    };
    void loadInitial();
    const disconnect = connectTeamStateStream(next => {
      if (stopped) return;
      receivedStreamState = true;
      setState(next);
      setError('');
      document.title = `${next.team.team || 'team'} · tt`;
    }, message => {
      if (!stopped) setError(message);
    });
    return () => {
      stopped = true;
      disconnect();
    };
  }, []);

  const mutate = useCallback(async (action: string, path: string, body?: unknown) => {
    setPendingAction(action);
    setActionError('');
    try {
      const response = await fetch(path, {
        method: 'POST',
        headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      if (!response.ok) {
        const message = (await response.text()).trim();
        throw new Error(message || `HTTP ${response.status}`);
      }
      return true;
    } catch (cause) {
      setActionError(cause instanceof Error ? cause.message : String(cause));
      return false;
    } finally {
      setPendingAction('');
    }
  }, []);

  const followUp = useCallback(
    (message: string) => mutate('follow-up', '/api/follow-up', { message }),
    [mutate],
  );
  const resume = useCallback(() => mutate('resume', '/api/resume'), [mutate]);
  const stop = useCallback(() => mutate('stop', '/api/stop'), [mutate]);

  return { state, error, actionError, pendingAction, followUp, resume, stop };
}
