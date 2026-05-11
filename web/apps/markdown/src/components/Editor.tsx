import { useState } from 'react';
import { Button, Input, message } from 'antd';
import { DeleteOutlined, SaveOutlined } from '@ant-design/icons';
import type { DocumentResponse } from '../types';

interface EditorProps {
  doc: DocumentResponse;
  content: string;
  setContent: (s: string) => void;
  fm: Record<string, string>;
  setFm: (v: Record<string, string>) => void;
  navigate: (href: string) => void;
}

export function Editor({ doc, content, setContent, fm, setFm, navigate }: EditorProps) {
  const [saving, setSaving] = useState(false);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
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

  return (
    <form className="editor-form" onSubmit={submit}>
      <div className="editor-actions">
        <Button onClick={() => navigate('/view' + doc.filePath)}>Cancel</Button>
        <Button type="primary" htmlType="submit" loading={saving} icon={<SaveOutlined />}>
          Save
        </Button>
        <Button
          danger
          htmlType="button"
          icon={<DeleteOutlined />}
          onClick={() => deleteDoc(doc.filePath, navigate)}
        >
          Delete
        </Button>
      </div>

      {doc.frontmatterFields?.length ? (
        <div className="fm-edit-panel">
          <div className="fm-edit-header">Frontmatter</div>
          <div className="fm-edit-fields">
            {doc.frontmatterFields.map(f => (
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
      ) : null}

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
