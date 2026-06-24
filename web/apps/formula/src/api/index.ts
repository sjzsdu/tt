import type { FormulaDashboardMessage, FormulaDashboardSnapshot, FormulaDashboardStep } from '../types';

export type AgentSessionTranscript = {
  session: string;
  agent?: string;
  path?: string;
  content?: string;
  missing?: boolean;
  message?: string;
};

function normalizeStep(step: FormulaDashboardStep): FormulaDashboardStep {
  return {
    ...step,
    labels: Array.isArray(step.labels) ? step.labels : [],
    input_ctx: Array.isArray(step.input_ctx) ? step.input_ctx : [],
    var_refs: Array.isArray(step.var_refs) ? step.var_refs : [],
    depends_on: Array.isArray(step.depends_on) ? step.depends_on : [],
    activities: Array.isArray(step.activities) ? step.activities : [],
    metadata: step.metadata || {},
    loop: step.loop ? {
      ...step.loop,
      body: Array.isArray(step.loop.body)
        ? step.loop.body.map(body => ({
            ...body,
            input_ctx: Array.isArray(body.input_ctx) ? body.input_ctx : [],
            var_refs: Array.isArray(body.var_refs) ? body.var_refs : [],
          }))
        : [],
    } : undefined,
  };
}

export function normalizeSnapshot(snapshot: FormulaDashboardSnapshot): FormulaDashboardSnapshot {
  return {
    ...snapshot,
    final_report_chat: snapshot.final_report_chat ? {
      ...snapshot.final_report_chat,
      messages: Array.isArray(snapshot.final_report_chat.messages) ? snapshot.final_report_chat.messages : [],
    } : undefined,
    repairs: Array.isArray(snapshot.repairs) ? snapshot.repairs : [],
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
  async retryStep(stepID: string, advice?: string): Promise<void> {
    const r = await fetch('/api/retry-step', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ step_id: stepID, advice: advice || '' }),
    });
    if (!r.ok) throw new Error(await r.text());
  },
  async stopRun(): Promise<void> {
    const r = await fetch('/api/stop', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!r.ok) throw new Error(await r.text());
  },
  async confirmRepair(stepID: string, attempt: number): Promise<void> {
    const r = await fetch('/api/repairs/confirm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ step_id: stepID, attempt }),
    });
    if (!r.ok) throw new Error(await r.text());
  },
  async ensureFinalReportChat(): Promise<void> {
    const r = await fetch('/api/final-report-chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!r.ok) throw new Error(await r.text());
  },
  async sendFinalReportChatMessage(message: string): Promise<void> {
    const r = await fetch('/api/final-report-chat/message', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message }),
    });
    if (!r.ok) throw new Error(await r.text());
  },
  async promoteFinalReportChatResponse(): Promise<void> {
    const r = await fetch('/api/final-report-chat/promote', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!r.ok) throw new Error(await r.text());
  },
  async agentSession(session: string, agent?: string): Promise<AgentSessionTranscript> {
    const params = new URLSearchParams({ session });
    if (agent) params.set('agent', agent);
    const r = await fetch(`/api/agent-session?${params.toString()}`);
    if (!r.ok) throw new Error(await r.text());
    return await r.json() as AgentSessionTranscript;
  },
};
