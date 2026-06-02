import { Splitter } from 'antd';
import type { ReactNode } from 'react';
import type { MdFile, TocItem } from '../types';
import { FileSidebar } from './FileSidebar';
import { TocSidebar } from './TocSidebar';

interface ShellProps {
  theme: 'light' | 'dark';
  onThemeChange: (theme: 'light' | 'dark') => void;
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
  fileMode: 'tree' | 'flat';
  setFileMode: (m: 'tree' | 'flat') => void;
  fileQuery: string;
  setFileQuery: (q: string) => void;
  toc: TocItem[];
  contentPaneRef: React.RefObject<HTMLElement | null>;
  onContentScroll?: () => void;
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
  onContentScroll,
  children,
  theme,
  onThemeChange,
}: ShellProps) {
  return (
    <Splitter className="layout">
      <Splitter.Panel defaultSize={280} min={200}>
        <FileSidebar
          theme={theme}
          onThemeChange={onThemeChange}
          files={files}
          current={current}
          navigate={navigate}
          fileMode={fileMode}
          setFileMode={setFileMode}
          fileQuery={fileQuery}
          setFileQuery={setFileQuery}
        />
      </Splitter.Panel>

      <Splitter.Panel min={400}>
        <main className="content-pane" ref={contentPaneRef} onScroll={onContentScroll}>{children}</main>
      </Splitter.Panel>

      <Splitter.Panel defaultSize={260} min={180} max={350}>
        <TocSidebar toc={toc} contentPaneRef={contentPaneRef} />
      </Splitter.Panel>
    </Splitter>
  );
}
