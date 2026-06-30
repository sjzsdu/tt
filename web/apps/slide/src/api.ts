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
  return fetch(`/raw${filePath}`).then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    return r.text();
  });
}

export function fetchRawContent(): Promise<string> {
  return fetch('/raw-content').then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    return r.text();
  });
}

export function fetchTemplate(name: string): Promise<ExternalTemplate> {
  return apiFetch(`/api/template/${encodeURIComponent(name)}`);
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
