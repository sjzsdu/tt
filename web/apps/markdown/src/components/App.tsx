import { useEffect, useRef, useState } from 'react';
import { Button, message } from 'antd';
import { DeleteOutlined, EditOutlined, SaveOutlined } from '@ant-design/icons';
import type { DocumentResponse, ListResponse, Route, TocItem } from '../types';
import { api } from '../api';
import { Shell } from './Shell';
import { Article } from './Article';
import { Editor } from './Editor';
import { FileLanding } from './FileLanding';
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';

function currentRoute(): Route {
  const path = window.location.pathname;
  if (path.startsWith('/edit/')) return { mode: 'edit', file: path.slice('/edit'.length) };
  if (path.startsWith('/view/')) return { mode: 'view', file: path.slice('/view'.length) };
  return { mode: 'list', file: '' };
}

export function App() {
  const [route, setRoute] = useState<Route>(currentRoute());
  const [list, setList] = useState<ListResponse | null>(null);
  const [doc, setDoc] = useState<DocumentResponse | null>(null);
  const [content, setContent] = useState('');
  const [fm, setFm] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [toc, setToc] = useState<TocItem[]>([]);
  const [activeToc, setActiveToc] = useState('');
  const [fileMode, setFileModeState] = useState<'tree' | 'flat'>(() =>
    localStorage.getItem('md-file-view-mode') === 'flat' ? 'flat' : 'tree'
  );
  const [saving, setSaving] = useState(false);
  const contentPaneRef = useRef<HTMLElement | null>(null);

  const save = async () => {
    setSaving(true);
    try {
      const body = new URLSearchParams();
      body.set('content', content);
      for (const [key, value] of Object.entries(fm)) {
        body.set('fm_' + key, value);
      }
      const res = await fetch('/save' + doc!.filePath, {
        method: 'POST',
        body,
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      });
      if (!res.ok) throw new Error(await res.text());
      message.success('Saved');
      navigate('/view' + doc!.filePath);
    } catch (err) {
      message.error('Save failed: ' + String(err));
    } finally {
      setSaving(false);
    }
  };

  const setFileMode = (m: 'tree' | 'flat') => {
    localStorage.setItem('md-file-view-mode', m);
    setFileModeState(m);
  };

  const navigate = (href: string) => {
    history.pushState(null, '', href);
    setRoute(currentRoute());
  };

  const connectWs = () => {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${location.host}/ws`);
    ws.onmessage = () => {
      api.list().then(setList).catch(e => console.error('[WS list]', e));
    };
    ws.onclose = () => setTimeout(connectWs, 3000);
    ws.onerror = e => console.error('[WS]', e);
    return ws;
  };

  useEffect(() => {
    const onPop = () => setRoute(currentRoute());
    addEventListener('popstate', onPop);
    return () => removeEventListener('popstate', onPop);
  }, []);

  useEffect(() => {
    api.list().then(setList).catch(e => setError(String(e)));
  }, []);

  useEffect(() => {
    const ws = connectWs();
    return () => ws.close();
  }, [route.file, route.mode]);

  const loadedFileRef = useRef<string | null>(null);

  useEffect(() => {
    if (!route.file) return;
    if (loadedFileRef.current !== route.file) {
      loadedFileRef.current = route.file;
      api.document(route.file).then(next => {
        if (loadedFileRef.current === route.file) {
          setDoc(next);
          setContent(next.contentText);
          if (route.mode === 'edit') {
            const initial: Record<string, string> = {};
            for (const f of next.frontmatterFields || []) {
              initial[f.Key] = f.Value;
            }
            setFm(initial);
          }
        }
      }).catch(e => setError(String(e)));
    }
  }, [route.file, route.mode]);

  useKeyboardShortcuts({
    onEdit: () => {
      if (route.mode !== 'edit' && doc && !list?.contentMode) {
        navigate('/edit' + doc.filePath);
      }
    },
    onSave: () => {
      if (route.mode === 'edit') save();
    },
    onEscape: () => {
      if (route.mode === 'edit' && doc) {
        navigate('/view' + doc.filePath);
      }
    },
    onPrev: () => {
      const idx = files.findIndex(f => f.Relative === doc?.filePath);
      if (idx > 0) navigate('/view' + files[idx - 1].Relative);
    },
    onNext: () => {
      const idx = files.findIndex(f => f.Relative === doc?.filePath);
      if (idx >= 0 && idx < files.length - 1) navigate('/view' + files[idx + 1].Relative);
    },
  });

  const files = doc?.files || list?.files || [];
  const shellProps = {
    files,
    current: doc?.filePath || route.file,
    navigate,
    fileMode,
    setFileMode,
    toc,
    activeToc,
    contentPaneRef,
  };

  if (error) return <Shell {...shellProps}><div className="empty error">{error}</div></Shell>;
  if (!doc && route.mode === 'list') return <Shell {...shellProps}><FileLanding list={list} navigate={navigate} /></Shell>;
  if (!doc) return <Shell {...shellProps}><div className="empty">Loading...</div></Shell>;

  return (
    <Shell {...shellProps}>
      <div className="toolbar">
        <div className="toolbar-title">
          <strong>{doc.filePath}</strong>
          <span>{route.mode === 'edit' ? 'Editing Markdown' : 'Preview'}</span>
        </div>
        <div className="toolbar-actions">
          <Button onClick={() => navigate('/')}>Files</Button>
          <Button href={doc.rawPath}>Raw</Button>
          {route.mode === 'edit' && (
            <>
              <Button onClick={() => navigate('/view' + doc.filePath)}>Preview</Button>
              <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={save}>Save</Button>
              <Button danger icon={<DeleteOutlined />} onClick={() => deleteDoc(doc.filePath, navigate)}>Delete</Button>
            </>
          )}
          {!list?.contentMode && route.mode !== 'edit' && (
            <Button icon={<EditOutlined />} onClick={() => navigate('/edit' + doc.filePath)}>Edit</Button>
          )}
        </div>
      </div>
      {route.mode === 'edit' ? (
        <Editor doc={doc} content={content} setContent={setContent} fm={fm} setFm={setFm} />
      ) : (
        <Article doc={doc} setToc={setToc} setActiveToc={setActiveToc} contentPaneRef={contentPaneRef} />
      )}
    </Shell>
  );
}

async function deleteDoc(path: string, navigate: (href: string) => void) {
  if (!confirm('Delete this markdown file? This cannot be undone.')) return;
  try {
    const res = await fetch('/delete' + path, { method: 'POST', redirect: 'follow' });
    if (!res.ok) throw new Error(await res.text());
    message.success('Deleted');
    navigate('/');
  } catch (err) {
    message.error('Delete failed: ' + String(err));
  }
}
