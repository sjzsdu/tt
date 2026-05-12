import { useMemo, useState } from 'react';
import { Empty, Input, Segmented } from 'antd';
import { AppstoreOutlined, UnorderedListOutlined } from '@ant-design/icons';
import type { ListResponse, MdFile } from '../types';

interface FileLandingProps {
  list: ListResponse | null;
  navigate: (href: string) => void;
}

type LandingView = 'card' | 'list';

function matchesQuery(file: MdFile, query: string) {
  if (!query) return true;
  const haystack = [file.Title, file.Name, file.Relative, file.Description]
    .filter(Boolean)
    .join('\n')
    .toLowerCase();
  return haystack.includes(query.toLowerCase());
}

function fileTitle(file: MdFile) {
  return file.Title || file.Name;
}

function FileMeta({ file }: { file: MdFile }) {
  return (
    <>
      <span>{file.Relative}</span>
      <span>{file.Size} bytes</span>
      {file.HasFrontmatter && <span className="fm-badge">FM</span>}
    </>
  );
}

export function FileLanding({ list, navigate }: FileLandingProps) {
  const [view, setView] = useState<LandingView>(() =>
    localStorage.getItem('md-landing-view') === 'list' ? 'list' : 'card'
  );
  const [query, setQuery] = useState('');

  const files = useMemo(() => {
    const normalized = query.trim();
    return (list?.files || []).filter(file => matchesQuery(file, normalized));
  }, [list?.files, query]);

  const setLandingView = (next: LandingView) => {
    localStorage.setItem('md-landing-view', next);
    setView(next);
  };

  if (!list) {
    return (
      <main className="files-home">
        <div className="empty">Loading...</div>
      </main>
    );
  }

  return (
    <main className="files-home">
      <section className="files-home-hero">
        <div>
          <p className="files-home-eyebrow">Markdown workspace</p>
          <h1>Files</h1>
          <p>{list.total ? `${list.total} markdown files found` : 'No .md files found in the current directory.'}</p>
        </div>
        <div className="files-home-actions">
          <Input.Search
            allowClear
            placeholder="Search title, path or description"
            value={query}
            onChange={event => setQuery(event.target.value)}
          />
          <Segmented
            value={view}
            onChange={value => setLandingView(value as LandingView)}
            options={[
              { label: 'Card', value: 'card', icon: <AppstoreOutlined /> },
              { label: 'List', value: 'list', icon: <UnorderedListOutlined /> },
            ]}
          />
        </div>
      </section>

      {!list.files.length ? (
        <div className="files-home-empty">
          <Empty description="No markdown files" />
          <p className="hint">Use <code>tt markdown</code> to open a specific file or folder.</p>
        </div>
      ) : !files.length ? (
        <div className="files-home-empty">
          <Empty description="No files match your search" />
        </div>
      ) : view === 'card' ? (
        <section className="files-home-grid">
          {files.map(file => (
            <article className="landing-file-card" key={file.Relative}>
              <button type="button" onClick={() => navigate('/view' + file.Relative)}>
                <strong>{fileTitle(file)}</strong>
                <span className="landing-file-path">{file.Relative}</span>
              </button>
              {file.Description && <p>{file.Description}</p>}
              <div className="landing-file-meta"><FileMeta file={file} /></div>
            </article>
          ))}
        </section>
      ) : (
        <section className="landing-file-list">
          {files.map(file => (
            <article className="landing-file-row" key={file.Relative}>
              <button type="button" onClick={() => navigate('/view' + file.Relative)}>
                <strong>{fileTitle(file)}</strong>
                <span>{file.Description || file.Relative}</span>
              </button>
              <div className="landing-file-meta"><FileMeta file={file} /></div>
            </article>
          ))}
        </section>
      )}
    </main>
  );
}
