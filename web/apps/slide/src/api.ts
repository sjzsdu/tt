export type SlideFile = {
  path: string;
  name: string;
};

export type ListResponse = {
  files: SlideFile[];
  total: number;
};

export type ExternalTemplate = {
  name: string;
  revealTheme: string;
  css: string;
  defaults: {
    theme: 'light' | 'dark';
    transition: string;
    center: boolean;
    margin?: number;
  };
};

export type ExternalWidgetTemplate = {
  type: string;
  html: string;
  css?: string;
  source?: 'project' | 'global';
};

export type WidgetResponse = {
  widgets: Record<string, ExternalWidgetTemplate>;
};

export function apiFetch<T>(path: string): Promise<T> {
  return fetch(path).then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    return r.json();
  });
}

export function fetchSlideList(): Promise<ListResponse> {
  return apiFetch('/api/list');
}

export function fetchSlideContent(filePath: string): Promise<string> {
  return fetch(`/raw${filePath}`, { cache: 'no-store' }).then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    return r.text();
  });
}

export function fetchRawContent(): Promise<string> {
  return fetch('/raw-content', { cache: 'no-store' }).then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    return r.text();
  });
}

export function saveSlideContent(filePath: string, content: string): Promise<{ ok: boolean; file: string }> {
  return fetch('/api/slide/content', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ file: filePath, content }),
  }).then(async r => {
    if (!r.ok) throw new Error(await r.text() || `${r.status} ${r.statusText}`);
    return r.json();
  });
}

export type RewriteSlideRequest = {
  file: string;
  slideIndex: number;
  slideSource: string;
  instruction: string;
  previousSlide?: string;
  nextSlide?: string;
};

export type RewriteSlideResponse = {
  updatedSlideSource?: string;
  summary?: string;
  error?: string;
};

export function rewriteSlide(request: RewriteSlideRequest): Promise<RewriteSlideResponse> {
  return fetch('/api/slide/rewrite', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  }).then(async r => {
    const data = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(data.error || `${r.status} ${r.statusText}`);
    return data;
  });
}

export function fetchTemplate(name: string): Promise<ExternalTemplate> {
  return apiFetch(`/api/template/${encodeURIComponent(name)}`);
}

export function fetchWidgets(): Promise<WidgetResponse> {
  return apiFetch('/api/widgets');
}

export function createWS(onMessage: (data: any) => void): WebSocket {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${proto}//${location.host}/ws`);
  ws.onmessage = (e) => {
    try { onMessage(JSON.parse(e.data)); } catch (_) {}
  };
  ws.onclose = () => {
    setTimeout(() => createWS(onMessage), 2000);
  };
  return ws;
}
