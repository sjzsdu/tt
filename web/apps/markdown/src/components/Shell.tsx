import { Segmented } from 'antd';
import type { ReactNode } from 'react';
import type { MdFile, TocItem } from '../types';
import { FileList } from './FileList';

interface ShellProps {
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
  fileMode: 'tree' | 'flat';
  setFileMode: (m: 'tree' | 'flat') => void;
  toc: TocItem[];
  activeToc: string;
  contentPaneRef: React.RefObject<HTMLElement | null>;
  children: ReactNode;
}

export function Shell({
  files,
  current,
  navigate,
  fileMode,
  setFileMode,
  toc,
  activeToc,
  children,
}: ShellProps) {
  return (
    <div className="layout">
      <aside className="files-pane section">
        <h1 className="section-title">Markdown Files</h1>
        <p className="section-subtitle">Browse and edit local Markdown files.</p>
        <div className="file-toolbar">
          <span>{files.length} files</span>
          <Segmented
            size="small"
            value={fileMode}
            onChange={v => setFileMode(v as 'tree' | 'flat')}
            options={[
              { label: 'Tree', value: 'tree' },
              { label: 'List', value: 'flat' },
            ]}
          />
        </div>
        <FileList files={files} current={current} navigate={navigate} mode={fileMode} />
      </aside>

      <main className="content-pane">{children}</main>

      <aside className="toc-pane section">
        <h2 className="toc-title">On this page</h2>
        {toc.length ? (
          <ul className="toc-list">
            {toc.map(x => (
              <li key={x.id} className={`toc-item level-${x.level}`}>
                <a
                  className={`toc-link ${activeToc === x.id ? 'active' : ''}`}
                  href={'#' + x.id}
                  onClick={e => {
                    e.preventDefault();
                    document
                      .getElementById(x.id)
                      ?.scrollIntoView({ behavior: 'smooth', block: 'start' });
                    history.replaceState(null, '', '#' + x.id);
                  }}
                >
                  {x.text}
                </a>
              </li>
            ))}
          </ul>
        ) : (
          <p className="toc-empty">No headings found.</p>
        )}
      </aside>
    </div>
  );
}
