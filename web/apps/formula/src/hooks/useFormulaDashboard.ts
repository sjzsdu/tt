import { useEffect, useMemo, useState } from 'react';
import { api, normalizeSnapshot } from '../api';
import type { FormulaDashboardMessage, FormulaDashboardSnapshot, FormulaDashboardStep } from '../types';
import { statusOrder } from '../utils/status';

export function useFormulaDashboard(onError: (error: unknown) => void) {
  const [snapshot, setSnapshot] = useState<FormulaDashboardSnapshot | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    api.state().then(setSnapshot).catch(err => {
      setError(String(err));
      onError(err);
    });
  }, [onError]);

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
            setSnapshot(normalizeSnapshot(msg.state));
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
      skipped: counts.skipped || 0,
      failed: counts.failed || 0,
      logs: snapshot.logs.length,
      repairs: (snapshot.repairs || []).length,
    };
  }, [snapshot]);

  const orderedSteps = useMemo(() => {
    return [...(snapshot?.steps || [])].sort((a: FormulaDashboardStep, b: FormulaDashboardStep) => {
      const statusDelta = (statusOrder[a.status] ?? 99) - (statusOrder[b.status] ?? 99);
      if (statusDelta !== 0) return statusDelta;
      if ((a.depth || 0) !== (b.depth || 0)) return (a.depth || 0) - (b.depth || 0);
      return a.index - b.index;
    });
  }, [snapshot]);

  return { snapshot, setSnapshot, error, summary, orderedSteps };
}
