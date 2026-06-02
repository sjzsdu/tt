import { Empty } from 'antd';
import { TagOutlined } from '@ant-design/icons';
import type { MdFile } from '../types';

export type LandingView = 'card' | 'list';

interface FileLandingResultsProps {
  allFiles: MdFile[];
  files: MdFile[];
  view: LandingView;
  navigate: (href: string) => void;
}

export function FileLandingResults({ allFiles, files, view, navigate }: FileLandingResultsProps) {
  if (!allFiles.length) return <LandingEmpty description="No markdown files" hint />;
  if (!files.length) return <LandingEmpty description="No files match your search" />;
  if (view === 'card') return <LandingCardGrid files={files} navigate={navigate} />;
  return <LandingList files={files} navigate={navigate} />;
}

function LandingEmpty({ description, hint }: { description: string; hint?: boolean }) {
  return (
    <div className="files-home-empty">
      <Empty description={description} />
      {hint && <p className="hint">Use <code>tt markdown</code> to open a specific file or folder.</p>}
    </div>
  );
}

function LandingCardGrid({ files, navigate }: Pick<FileLandingResultsProps, 'files' | 'navigate'>) {
  return (
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
  );
}

function LandingList({ files, navigate }: Pick<FileLandingResultsProps, 'files' | 'navigate'>) {
  return (
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
  );
}

function FileMeta({ file }: { file: MdFile }) {
  return (
    <>
      <span>{file.Relative}</span>
      <span>{file.Size} bytes</span>
      {file.HasFrontmatter && <TagOutlined className="fm-icon" title="Has frontmatter" />}
    </>
  );
}

function fileTitle(file: MdFile) {
  return file.Title || file.Name;
}
