import { useEffect, useState } from 'react';
import { message } from 'antd';
import type { DocumentResponse, Route, TocItem } from '../types';
import { api } from '../api';

interface UseMarkdownDocumentOptions {
  route: Route;
  navigate: (href: string) => void;
  onListRoute: () => void;
  onError: (error: string) => void;
}

export function useMarkdownDocument({ route, navigate, onListRoute, onError }: UseMarkdownDocumentOptions) {
  const [doc, setDoc] = useState<DocumentResponse | null>(null);
  const [content, setContent] = useState('');
  const [fm, setFm] = useState<Record<string, string>>({});
  const [toc, setToc] = useState<TocItem[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!route.file) {
      setDoc(null);
      setToc([]);
      onListRoute();
      return;
    }

    api.document(route.file).then(next => {
      setDoc(next);
      setContent(next.contentText);
      if (route.mode === 'edit') setFm(frontmatterMap(next));
    }).catch(e => onError(String(e)));
  }, [route.file, route.mode]);

  const save = async () => {
    if (!doc) return;
    setSaving(true);
    try {
      const body = new URLSearchParams();
      body.set('content', content);
      for (const [key, value] of Object.entries(fm)) body.set('fm_' + key, value);
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

  const deleteDoc = async () => {
    if (!doc || !confirm('Delete this markdown file? This cannot be undone.')) return;
    try {
      const res = await fetch('/delete' + doc.filePath, { method: 'POST', redirect: 'follow' });
      if (!res.ok) throw new Error(await res.text());
      message.success('Deleted');
      navigate('/');
    } catch (err) {
      message.error('Delete failed: ' + String(err));
    }
  };

  return { doc, content, setContent, fm, setFm, toc, setToc, saving, save, deleteDoc };
}

function frontmatterMap(doc: DocumentResponse) {
  const initial: Record<string, string> = {};
  for (const field of doc.frontmatterFields || []) initial[field.Key] = field.Value;
  return initial;
}
