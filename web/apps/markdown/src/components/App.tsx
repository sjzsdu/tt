import { useEffect, useState } from 'react';
import { message } from 'antd';
import type { DocumentResponse, ListResponse, TocItem } from '../types';
import { api } from '../api';
import { Shell } from './Shell';
import { FileLanding } from './FileLanding';
import { DocumentWorkspace } from './DocumentWorkspace';
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';
import { useLiveFileList } from '../hooks/useLiveFileList';
import { usePersistentState } from '../hooks/usePersistentState';
import { useScrollMemory } from '../hooks/useScrollMemory';
import { currentRoute } from '../store/route';

export function App({ theme, onThemeChange }: { theme: 'light' | 'dark'; onThemeChange: (theme: 'light' | 'dark') => void }) {
  const [route, setRoute] = useState(currentRoute);
  const [list, setList] = useState<ListResponse | null>(null);
  const [doc, setDoc] = useState<DocumentResponse | null>(null);
  const [content, setContent] = useState('');
  const [fm, setFm] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [toc, setToc] = useState<TocItem[]>([]);
  const [saving, setSaving] = useState(false);
  const [fileMode, setFileMode] = usePersistentState<'tree' | 'flat'>(
    'md-file-view-mode',
    'tree',
    value => (value === 'flat' ? 'flat' : 'tree')
  );
  const [fileQuery, setFileQuery] = usePersistentState('md-file-query', '');
  const { contentPaneRef, rememberCurrentScroll, activateScrollKey } = useScrollMemory();

  const navigate = (href: string) => {
    rememberCurrentScroll();
    history.pushState(null, '', href);
    setRoute(currentRoute());
  };

  const save = async () => {
    if (!doc) return;
    setSaving(true);
    try {
      const body = new URLSearchParams();
      body.set('content', content);
      for (const [key, value] of Object.entries(fm)) {
        body.set('fm_' + key, value);
      }
      const res = await fetch('/save' + doc.filePath, {
        method: 'POST',
        body,
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      });
      if (!res.ok) throw new Error(await res.text());
      message.success('Saved');
      navigate('/view' + doc.filePath);
    } catch (err) {
      message.error('Save failed: ' + String(err));
    } finally {
      setSaving(false);
    }
  };

  useEffect(() => {
    const onPop = () => {
      rememberCurrentScroll();
      setRoute(currentRoute());
    };
    addEventListener('popstate', onPop);
    return () => removeEventListener('popstate', onPop);
  }, []);

  useEffect(() => {
    api.list().then(setList).catch(e => setError(String(e)));
  }, []);

  useLiveFileList(setList);

  useEffect(() => {
    if (!route.file) {
      setDoc(null);
      setToc([]);
      return;
    }
    api.document(route.file).then(next => {
      setDoc(next);
      setContent(next.contentText);
      if (route.mode === 'edit') {
        const initial: Record<string, string> = {};
        for (const f of next.frontmatterFields || []) {
          initial[f.Key] = f.Value;
        }
        setFm(initial);
      }
    }).catch(e => setError(String(e)));
  }, [route.file, route.mode]);

  useEffect(() => {
    activateScrollKey(doc && route.mode !== 'list' ? doc.filePath : '');
  }, [route.mode, doc?.filePath]);

  const files = doc?.files || list?.files || [];

  useKeyboardShortcuts({
    onEdit: () => {
      if (route.mode !== 'edit' && doc && !list?.contentMode) navigate('/edit' + doc.filePath);
    },
    onSave: () => {
      if (route.mode === 'edit') save();
    },
    onEscape: () => {
      if (route.mode === 'edit' && doc) navigate('/view' + doc.filePath);
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

  const shellProps = {
    files,
    current: route.mode === 'list' ? '' : doc?.filePath || route.file,
    navigate,
    fileMode,
    setFileMode,
    fileQuery,
    setFileQuery,
    toc,
    contentPaneRef,
    onContentScroll: rememberCurrentScroll,
    theme,
    onThemeChange,
  };

  if (error) return <Shell {...shellProps}><div className="empty error">{error}</div></Shell>;
  if (route.mode === 'list') return <FileLanding list={list} navigate={navigate} query={fileQuery} setQuery={setFileQuery} />;
  if (!doc) return <Shell {...shellProps}><div className="empty">Loading...</div></Shell>;

  return (
    <Shell {...shellProps}>
      <DocumentWorkspace
        doc={doc}
        route={route}
        content={content}
        setContent={setContent}
        fm={fm}
        setFm={setFm}
        contentMode={list?.contentMode}
        saving={saving}
        contentPaneRef={contentPaneRef}
        navigate={navigate}
        onSave={save}
        onDelete={() => deleteDoc(doc.filePath, navigate)}
        setToc={setToc}
      />
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
