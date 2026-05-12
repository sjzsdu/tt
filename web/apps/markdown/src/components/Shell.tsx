import { Anchor, Input, Segmented, Splitter } from 'antd';
import type { ReactNode } from 'react';
import type { MdFile, TocItem } from '../types';
import { FileList } from './FileList';
import { filterFiles } from '../utils/fileSearch';

interface ShellProps {
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
  fileMode: 'tree' | 'flat';
  setFileMode: (m: 'tree' | 'flat') => void;
  fileQuery: string;
  setFileQuery: (q: string) => void;
  toc: TocItem[];
  contentPaneRef: React.RefObject<HTMLElement | null>;
  children: ReactNode;
}

export function Shell({
  files,
  current,
  navigate,
  fileMode,
  setFileMode,
  fileQuery,
  setFileQuery,
  toc,
  contentPaneRef,
  children,
}: ShellProps) {
  const filteredFiles = filterFiles(files, fileQuery);
  const anchorItems = toc.map(item => ({
    key: item.id,
    href: `#${item.id}`,
    title: <span className={`toc-anchor-title level-${item.level}`}>{item.text}</span>,
  }));

  return (
    <Splitter className="layout">
      <Splitter.Panel defaultSize="280px" min="200px" max="400px">
        <aside className="files-pane section">
          <h1 className="section-title">Markdown Files</h1>
          <p className="section-subtitle">Browse and edit local Markdown files.</p>
          <Input.Search
            className="file-search"
            allowClear
            size="small"
            placeholder="Search files"
            value={fileQuery}
            onChange={event => setFileQuery(event.target.value)}
          />
          <div className="file-toolbar">
            <span>{filteredFiles.length}/{files.length} files</span>
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
          <FileList files={filteredFiles} current={current} navigate={navigate} mode={fileMode} searchActive={Boolean(fileQuery.trim())} />
        </aside>
      </Splitter.Panel>

      <Splitter.Panel min="400px">
        <main className="content-pane" ref={contentPaneRef}>{children}</main>
      </Splitter.Panel>

      <Splitter.Panel defaultSize="260px" min="180px" max="350px">
        <aside className="toc-pane section">
          <h2 className="toc-title">On this page</h2>
          {toc.length ? (
            <Anchor
              affix={false}
              className="toc-anchor"
              getContainer={() => contentPaneRef.current || window}
              items={anchorItems}
              targetOffset={80}
              onClick={(event, link) => {
                event.preventDefault();
                const id = link.href.replace(/^#/, '');
                const el = document.getElementById(id);
                const pane = contentPaneRef.current;
                if (!el || !pane) return;
                const paneRect = pane.getBoundingClientRect();
                const elRect = el.getBoundingClientRect();
                pane.scrollTo({ top: elRect.top - paneRect.top + pane.scrollTop - 20, behavior: 'smooth' });
                history.replaceState(null, '', link.href);
              }}
            />
          ) : (
            <p className="toc-empty">No headings found.</p>
          )}
        </aside>
      </Splitter.Panel>
    </Splitter>
  );
}
