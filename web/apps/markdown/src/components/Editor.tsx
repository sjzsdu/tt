import { Input } from 'antd';
import type { DocumentResponse } from '../types';

interface EditorProps {
  doc: DocumentResponse;
  content: string;
  setContent: (s: string) => void;
  fm: Record<string, string>;
  setFm: (v: Record<string, string>) => void;
}

export function Editor({ doc, content, setContent, fm, setFm }: EditorProps) {
  return (
    <form className="editor-form" onSubmit={e => e.preventDefault()}>
      {doc.hasFrontmatter && (
        <div className="fm-edit-panel">
          <div className="fm-edit-header">Frontmatter</div>
          <div className="fm-edit-fields">
            {doc.frontmatterFields?.map(f => (
              <label className="fm-edit-field" key={f.Key}>
                <span className="fm-edit-label">{f.Key}</span>
                <Input
                  name={'fm_' + f.Key}
                  value={fm[f.Key] || ''}
                  onChange={e => setFm({ ...fm, [f.Key]: e.target.value })}
                />
              </label>
            ))}
          </div>
        </div>
      )}

      <Input.TextArea
        className="editor-textarea"
        name="content"
        value={content}
        onChange={e => setContent(e.target.value)}
        autoSize={{ minRows: 24 }}
      />

      <div className="editor-hint">Editing body content. Frontmatter fields above are preserved when present.</div>
    </form>
  );
}
