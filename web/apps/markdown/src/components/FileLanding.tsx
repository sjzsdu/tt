import type { ListResponse } from '../types';

interface FileLandingProps {
  list: ListResponse | null;
  navigate: (href: string) => void;
}

export function FileLanding({ list, navigate }: FileLandingProps) {
  if (!list) return <div className="empty">Loading...</div>;

  return (
    <div className="doc-wrap">
      <div className="card">
        <h1>Markdown Files</h1>
        <p>Total: {list.total}</p>
        {list.files.map(f => (
          <div className="file-card" key={f.Relative}>
            <h2>
              <button onClick={() => navigate('/view' + f.Relative)}>
                {f.Title || f.Name}
                {f.HasFrontmatter && <span className="fm-badge">FM</span>}
              </button>
            </h2>
            <p>{f.Relative}</p>
            {f.Description && <div className="desc">{f.Description}</div>}
            <small>
              {f.Size} bytes · <a href={'/raw' + f.Relative}>download raw</a>
            </small>
          </div>
        ))}
      </div>
    </div>
  );
}
