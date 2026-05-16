import type { FormulaDashboardMessage, FormulaDashboardSnapshot } from '../types';

export const api = {
  async state(): Promise<FormulaDashboardSnapshot> {
    const r = await fetch('/api/state');
    if (!r.ok) throw new Error(await r.text());
    const msg = await r.json() as FormulaDashboardMessage;
    return msg.state;
  },
};
