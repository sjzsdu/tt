import { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

type Agent = { id: string; name: string; description?: string };
type Defaults = { agent: string; model: string; session: string; debug: boolean };
type StateResponse = { workspace: string; defaults: Defaults; agents: Agent[] };
type SessionInfo = { session: string; agent?: string; path: string; updated_at?: string; size?: number };
type TranscriptMessage = { role: string; content: string };
type TranscriptResponse = { missing?: boolean; message?: string; path?: string; messages?: TranscriptMessage[] };
type StreamEvent = { type: string; delta?: string; error?: string };
type GitFile = { path: string; status?: string };
type GitCommit = { hash: string; subject: string };
type GitContext = { branch?: string; files?: GitFile[]; commits?: GitCommit[]; diff?: string; missing?: boolean; message?: string };

type ChatMessage = { role: string; content: string; error?: boolean };

function escapeHtml(value: string) {
  return String(value || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c] || c));
}

function inlineMd(value: string) {
  return escapeHtml(value)
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>');
}

function renderMarkdown(text: string) {
  const lines = String(text || '').split('\n');
  let html = '';
  let inCode = false;
  let paragraph: string[] = [];
  const flush = () => {
    if (!paragraph.length) return;
    html += `<p>${inlineMd(paragraph.join(' '))}</p>`;
    paragraph = [];
  };
  for (const line of lines) {
    if (line.startsWith('```')) {
      flush();
      inCode = !inCode;
      html += inCode ? '<pre><code>' : '</code></pre>';
      continue;
    }
    if (inCode) {
      html += `${escapeHtml(line)}\n`;
      continue;
    }
    if (/^#{1,3}\s+/.test(line)) {
      flush();
      const level = line.match(/^#+/)?.[0].length || 1;
      html += `<h${level}>${inlineMd(line.replace(/^#{1,3}\s+/, ''))}</h${level}>`;
      continue;
    }
    if (/^[-*]\s+/.test(line)) {
      flush();
      html += `<ul><li>${inlineMd(line.replace(/^[-*]\s+/, ''))}</li></ul>`;
      continue;
    }
    if (!line.trim()) {
      flush();
      continue;
    }
    paragraph.push(line.trim());
  }
  flush();
  if (inCode) html += '</code></pre>';
  return html;
}

function normalizeRole(role: string) {
  return ['user', 'assistant', 'tool', 'system'].includes(role) ? role : 'assistant';
}

async function readSSE(res: Response, onEvent: (event: StreamEvent) => void) {
  if (!res.body) throw new Error('stream response body missing');
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let idx = -1;
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const raw = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      for (const line of raw.split('\n')) {
        if (!line.startsWith('data:')) continue;
        const json = line.slice(5).trim();
        if (json) onEvent(JSON.parse(json));
      }
    }
  }
}

function MessageView({ message }: { message: ChatMessage }) {
  const role = normalizeRole(message.role);
  const className = `msg ${role}${message.error ? ' error' : ''}`;
  return (
    <div className={className}>
      {role === 'tool' ? <div className="msg-role">tool output</div> : null}
      <div
        className="msg-content"
        dangerouslySetInnerHTML={{ __html: role === 'tool' ? escapeHtml(message.content) : renderMarkdown(message.content) }}
      />
    </div>
  );
}

function sessionLabel(session: SessionInfo) {
  const agent = session.agent ? `[${session.agent}] ` : '';
  const size = session.size ? ` · ${Math.round(session.size / 1024)} KiB` : '';
  const at = session.updated_at ? ` · ${new Date(session.updated_at).toLocaleString()}` : '';
  return `${agent}${session.session}${size}${at}`;
}

function App() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [agent, setAgent] = useState('');
  const [session, setSession] = useState('cli:default');
  const [model, setModel] = useState('');
  const [workspace, setWorkspace] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [git, setGit] = useState<GitContext>({});
  const messageRef = useRef<HTMLDivElement>(null);

  const filteredSessions = useMemo(() => sessions.filter(item => !agent || !item.agent || item.agent === agent), [sessions, agent]);

  const scrollToBottom = () => requestAnimationFrame(() => {
    if (messageRef.current) messageRef.current.scrollTop = messageRef.current.scrollHeight;
  });

  const loadSessions = async () => {
    const res = await fetch('/api/sessions');
    const data = await res.json();
    setSessions(data.sessions || []);
  };

  const loadGitContext = async (file = '') => {
    const res = await fetch(`/api/git/context${file ? `?file=${encodeURIComponent(file)}` : ''}`);
    const data = await res.json();
    setGit(data);
  };

  const loadTranscript = async (nextAgent = agent, nextSession = session) => {
    const params = new URLSearchParams({ agent: nextAgent || '', session: nextSession || '' });
    const res = await fetch(`/api/transcript?${params}`);
    const data: TranscriptResponse = await res.json();
    if (data.missing) {
      setMessages([{ role: 'system', content: '没有找到这个 session 的历史记录，可以直接开始新对话。' }]);
      return;
    }
    const history = (data.messages || []).map(item => ({ role: normalizeRole(item.role), content: item.content }));
    setMessages([...history, { role: 'system', content: `已加载历史：${data.path || ''}` }]);
    scrollToBottom();
  };

  useEffect(() => {
    (async () => {
      const res = await fetch('/api/state');
      const data: StateResponse = await res.json();
      setAgents(data.agents || []);
      setWorkspace(data.workspace || '');
      const defaults = data.defaults || {} as Defaults;
      const defaultAgent = defaults.agent || '';
      const defaultSession = defaults.session || 'cli:default';
      setAgent(defaultAgent);
      setSession(defaultSession);
      setModel(defaults.model || '');
      await loadSessions();
      await loadTranscript(defaultAgent, defaultSession);
      await loadGitContext();
    })().catch(err => setMessages([{ role: 'assistant', content: String(err), error: true }]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => scrollToBottom(), [messages]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const text = input.trim();
    if (!text || busy) return;
    setInput('');
    setBusy(true);
    const pendingIndex = messages.length + 1;
    setMessages(prev => [...prev, { role: 'user', content: text }, { role: 'assistant', content: '正在等待 agent 回复...' }]);
    let acc = '';
    try {
      const res = await fetch('/api/chat/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
        body: JSON.stringify({ message: text, agent, session, model }),
      });
      if (!res.ok || !res.body) throw new Error(res.statusText || 'stream request failed');
      await readSSE(res, event => {
        if (event.type === 'delta') {
          acc += event.delta || '';
          setMessages(prev => prev.map((item, idx) => idx === pendingIndex ? { role: 'assistant', content: acc } : item));
        } else if (event.type === 'error') {
          setMessages(prev => prev.map((item, idx) => idx === pendingIndex ? { role: 'assistant', content: event.error || 'agent failed', error: true } : item));
        }
      });
      await loadSessions();
      await loadGitContext();
    } catch (err) {
      setMessages(prev => prev.map((item, idx) => idx === pendingIndex ? { role: 'assistant', content: String(err), error: true } : item));
    } finally {
      setBusy(false);
    }
  };

  const createSession = () => {
    const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
    const value = `web:${agent || 'agent'}:${stamp}`;
    setSession(value);
    setMessages([{ role: 'system', content: `已创建新 session：${value}` }]);
  };

  return (
    <div className="app">
      <aside className="side">
        <h1>tt agent web</h1>
        <label className="label" htmlFor="agent">Agent</label>
        <select id="agent" value={agent} onChange={event => { setAgent(event.target.value); loadTranscript(event.target.value, session).catch(() => undefined); }}>
          {agents.map(item => <option key={item.id} value={item.id}>{item.description ? `${item.id} - ${item.description}` : item.id}</option>)}
        </select>
        <label className="label" htmlFor="session">Session</label>
        <input id="session" value={session} onChange={event => setSession(event.target.value)} onBlur={() => loadTranscript(agent, session).catch(() => undefined)} />
        <label className="label" htmlFor="session-list">History</label>
        <select id="session-list" value="" onChange={event => {
          const selected = sessions.find(item => item.session === event.target.value);
          if (!selected) return;
          setSession(selected.session);
          if (selected.agent) setAgent(selected.agent);
          loadTranscript(selected.agent || agent, selected.session).catch(() => undefined);
        }}>
          <option value="">选择历史 session...</option>
          {filteredSessions.map(item => <option key={`${item.agent}:${item.session}:${item.path}`} value={item.session}>{sessionLabel(item)}</option>)}
        </select>
        <button className="secondary" type="button" onClick={createSession}>新建 session</button>
        <label className="label" htmlFor="model">Model</label>
        <input id="model" value={model} onChange={event => setModel(event.target.value)} placeholder="default" />
        <button className="secondary" type="button" onClick={() => loadTranscript().catch(err => setMessages([{ role: 'assistant', content: String(err), error: true }]))}>加载历史</button>
        <div className="meta">Workspace: {workspace || '(unknown)'}</div>
        <div className="hint">当前使用 SSE 传输层。Ctrl+Enter 发送。</div>
      </aside>
      <main className="main">
        <div className="messages" ref={messageRef}>{messages.map((message, idx) => <MessageView key={idx} message={message} />)}</div>
        <form className="composer" onSubmit={submit}>
          <textarea value={input} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && event.ctrlKey) submit(event); }} placeholder="输入消息，例如：帮我理解这个项目的架构" />
          <button id="send" type="submit" disabled={busy}>{busy ? '等待中' : '发送'}</button>
        </form>
      </main>
      <aside className="context">
        <h1>Context</h1>
        <button className="secondary" type="button" onClick={() => loadGitContext().catch(err => setGit({ missing: true, message: String(err) }))}>刷新 Git 状态</button>
        <div className="meta">{git.missing ? git.message : `Branch: ${git.branch || '(detached/unknown)'}`}</div>
        <label className="label">Changed files</label>
        <div className="ctx-list">
          {(git.files || []).length ? (git.files || []).map(file => (
            <button key={`${file.status}:${file.path}`} type="button" className="ctx-item" onClick={() => loadGitContext(file.path)}><strong>{file.status || '??'}</strong> {file.path}</button>
          )) : <div className="meta">工作区干净</div>}
        </div>
        <label className="label">Recent commits</label>
        <div className="ctx-list">{(git.commits || []).map(commit => <div key={commit.hash} className="ctx-item"><small>{commit.hash}</small><br />{commit.subject}</div>)}</div>
        <label className="label">Diff</label>
        <div className="diff">{git.diff === undefined ? '选择 changed file 查看 diff' : (git.diff || '没有 diff')}</div>
      </aside>
    </div>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
