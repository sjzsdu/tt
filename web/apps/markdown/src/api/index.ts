import type { ListResponse, DocumentResponse } from '../types';

export const api = {
  async list(): Promise<ListResponse> {
    const r = await fetch('/api/list');
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  },

  async content(): Promise<DocumentResponse> {
    const r = await fetch('/api/content');
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  },

  async document(path: string): Promise<DocumentResponse> {
    const r = await fetch(`/api/document${path}`);
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  },
};
