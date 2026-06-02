import type { Dispatch, SetStateAction } from 'react';
import type { DocumentResponse, Route, TocItem } from '../types';
import { Article } from './Article';
import { DocumentToolbar } from './DocumentToolbar';
import { Editor } from './Editor';

interface DocumentWorkspaceProps {
  doc: DocumentResponse;
  route: Route;
  content: string;
  setContent: Dispatch<SetStateAction<string>>;
  fm: Record<string, string>;
  setFm: Dispatch<SetStateAction<Record<string, string>>>;
  contentMode?: boolean;
  saving: boolean;
  contentPaneRef: React.RefObject<HTMLElement | null>;
  navigate: (href: string) => void;
  onSave: () => void;
  onDelete: () => void;
  setToc: (toc: TocItem[]) => void;
  theme: 'light' | 'dark';
}

export function DocumentWorkspace({
  doc,
  route,
  content,
  setContent,
  fm,
  setFm,
  contentMode,
  saving,
  contentPaneRef,
  navigate,
  onSave,
  onDelete,
  setToc,
  theme,
}: DocumentWorkspaceProps) {
  return (
    <>
      <DocumentToolbar
        doc={doc}
        route={route}
        contentMode={contentMode}
        saving={saving}
        navigate={navigate}
        onSave={onSave}
        onDelete={onDelete}
      />
      {route.mode === 'edit' ? (
        <Editor doc={doc} content={content} setContent={setContent} fm={fm} setFm={setFm} />
      ) : (
        <Article doc={doc} setToc={setToc} contentPaneRef={contentPaneRef} theme={theme} />
      )}
    </>
  );
}
