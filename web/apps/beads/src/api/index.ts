import type { Issue, GraphResult, HealthResponse, WorkspaceResponse } from '../types';

async function fetchJSON<T>(url: string): Promise<T> {
  const r = await fetch(url);
  if (!r.ok) {
    const text = await r.text().catch(() => r.statusText);
    throw new Error(`API ${r.status}: ${text}`);
  }
  return r.json() as Promise<T>;
}

async function mutateJSON<T>(url: string, body: unknown, method = 'POST'): Promise<T> {
  const r = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!r.ok) {
    const text = await r.text().catch(() => r.statusText);
    throw new Error(`API ${r.status}: ${text}`);
  }
  return r.json() as Promise<T>;
}

export type CreateIssueRequest = {
  title: string;
  description?: string;
  priority?: number;
  issue_type?: string;
  labels?: string[];
};

export type UpdateIssueRequest = {
  title?: string;
  description?: string;
  status?: string;
  priority?: number;
  issue_type?: string;
  labels?: string[];
  assignee?: string;
  acceptance_criteria?: string;
};

export type AddDependencyRequest = {
  depends_on_id: string;
  type?: string;
};

export const api = {
  health(): Promise<HealthResponse> {
    return fetchJSON('/api/health');
  },

  workspace(): Promise<WorkspaceResponse> {
    return fetchJSON('/api/workspace');
  },

  listIssues(params?: { status?: string; labels?: string[]; limit?: number }): Promise<Issue[]> {
    const qs = new URLSearchParams();
    if (params?.status) qs.set('status', params.status);
    if (params?.labels?.length) qs.set('labels', params.labels.join(','));
    if (params?.limit) qs.set('limit', String(params.limit));
    const query = qs.toString();
    return fetchJSON(query ? `/api/issues?${query}` : '/api/issues');
  },

  issueDetail(id: string): Promise<Issue> {
    return fetchJSON(`/api/issues/${encodeURIComponent(id)}`);
  },

  search(query: string): Promise<Issue[]> {
    if (!query.trim()) return Promise.resolve([]);
    return fetchJSON(`/api/search?q=${encodeURIComponent(query)}`);
  },

  graph(rootID: string): Promise<GraphResult> {
    return fetchJSON(`/api/graph/${encodeURIComponent(rootID)}`);
  },

  // Mutations
  createIssue(req: CreateIssueRequest): Promise<Issue[]> {
    return mutateJSON('/api/issues', req);
  },

  updateIssue(id: string, req: UpdateIssueRequest): Promise<Issue> {
    return mutateJSON(`/api/issues/${encodeURIComponent(id)}`, req, 'PATCH');
  },

  addDependency(issueId: string, req: AddDependencyRequest): Promise<unknown> {
    return mutateJSON(`/api/issues/${encodeURIComponent(issueId)}/dependencies`, req);
  },
};
