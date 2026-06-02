import { useEffect, useState } from 'react';
import type { ListResponse } from '../types';
import { api } from '../api';
import { Shell } from './Shell';
import { FileLanding } from './FileLanding';
import { DocumentWorkspace } from './DocumentWorkspace';
import { useDocumentShortcuts } from '../hooks/useDocumentShortcuts';
import { useLiveFileList } from '../hooks/useLiveFileList';
import { useMarkdownDocument } from '../hooks/useMarkdownDocument';
import { usePersistentState } from '../hooks/usePersistentState';
import { useScrollMemory } from '../hooks/useScrollMemory';
import { currentRoute } from '../store/route';

export function App({ theme, onThemeChange }: { theme: 'light' | 'dark'; onThemeChange: (theme: 'light' | 'dark') => void }) {
  const [route, setRoute] = useState(currentRoute);
  const [list, setList] = useState<ListResponse | null>(null);
  const [error, setError] = useState('');
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

  const documentState = useMarkdownDocument({
    route,
    navigate,
    onListRoute: () => activateScrollKey(''),
    onError: setError,
  });
  const { doc, toc, setToc, save, deleteDoc } = documentState;
  const files = doc?.files || list?.files || [];

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
    activateScrollKey(doc && route.mode !== 'list' ? doc.filePath : '');
  }, [route.mode, doc?.filePath]);

  useDocumentShortcuts({ route, doc, list, files, navigate, onSave: save });

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
        content={documentState.content}
        setContent={documentState.setContent}
        fm={documentState.fm}
        setFm={documentState.setFm}
        contentMode={list?.contentMode}
        saving={documentState.saving}
        contentPaneRef={contentPaneRef}
        navigate={navigate}
        onSave={save}
        onDelete={deleteDoc}
        setToc={setToc}
      />
    </Shell>
  );
}
