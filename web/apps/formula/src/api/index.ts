import type { FormulaDashboardMessage, FormulaDashboardSnapshot, FormulaDashboardStep } from '../types';

function normalizeStep(step: FormulaDashboardStep): FormulaDashboardStep {
  return {
    ...step,
    labels: Array.isArray(step.labels) ? step.labels : [],
    input_ctx: Array.isArray(step.input_ctx) ? step.input_ctx : [],
    depends_on: Array.isArray(step.depends_on) ? step.depends_on : [],
    activities: Array.isArray(step.activities) ? step.activities : [],
    metadata: step.metadata || {},
    loop: step.loop ? {
      ...step.loop,
      body: Array.isArray(step.loop.body) ? step.loop.body : [],
    } : undefined,
  };
}

export function normalizeSnapshot(snapshot: FormulaDashboardSnapshot): FormulaDashboardSnapshot {
  return {
    ...snapshot,
    steps: Array.isArray(snapshot.steps) ? snapshot.steps.map(normalizeStep) : [],
    edges: Array.isArray(snapshot.edges) ? snapshot.edges : [],
    logs: Array.isArray(snapshot.logs) ? snapshot.logs : [],
  };
}

export const api = {
  async state(): Promise<FormulaDashboardSnapshot> {
    const r = await fetch('/api/state');
    if (!r.ok) throw new Error(await r.text());
    const msg = await r.json() as FormulaDashboardMessage;
    return normalizeSnapshot(msg.state);
  },
  async submitHumanInput(stepID: string, response: Record<string, unknown>): Promise<void> {
    const r = await fetch('/api/human-input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ step_id: stepID, response }),
    });
    if (!r.ok) throw new Error(await r.text());
  },
};
