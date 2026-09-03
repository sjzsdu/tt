import { useEffect, useRef, type MouseEvent } from 'react';
import type { DocumentResponse, TocItem } from '../types';
import { useMarkdownParts } from '../hooks/useMarkdown';
import { MarkdownContent } from './MarkdownContent';

interface ArticleProps {
  doc: DocumentResponse;
  setToc: (items: TocItem[]) => void;
  contentPaneRef: React.RefObject<HTMLElement | null>;
  theme: 'light' | 'dark';
}

function slugify(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replace(/[^\w\u4e00-\u9fa5\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-');
}

function isLocalHTMLPath(pathname: string): boolean {
  const lower = pathname.toLowerCase();
  return lower.endsWith('.html') || lower.endsWith('.htm');
}

function handleArticleLinkClick(event: MouseEvent<HTMLElement>, currentFilePath: string) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const target = event.target;
  if (!(target instanceof Element)) return;
  const anchor = target.closest('a');
  if (!anchor) return;

  const href = anchor.getAttribute('href')?.trim();
  if (!href || href.startsWith('#')) return;

  let url: URL;
  try {
    url = new URL(href, `${window.location.origin}${currentFilePath}`);
  } catch {
    return;
  }
  if (url.origin !== window.location.origin || !isLocalHTMLPath(url.pathname)) return;

  event.preventDefault();
  window.location.assign(`/html${url.pathname}${url.search}${url.hash}`);
}

export function Article({ doc, setToc, contentPaneRef, theme }: ArticleProps) {
  const articleRef = useRef<HTMLElement | null>(null);
  const parts = useMarkdownParts(doc.contentText);

  useEffect(() => {
    const article = articleRef.current;
    if (!article) return;

    const used = new Map<string, number>();
    const headings = [...article.querySelectorAll<HTMLHeadingElement>('h1,h2,h3,h4')];
    const items: TocItem[] = headings.map(h => {
      const text = h.textContent?.trim() || '';
      const base = slugify(text) || 'section';
      const count = used.get(base) || 0;
      used.set(base, count + 1);
      const id = count ? `${base}-${count}` : base;
      h.id = id;
      const level = Number(h.tagName.slice(1));
      return { id, text, level };
    });

    setToc(items);

    const pane = contentPaneRef.current;
    const hash = window.location.hash.slice(1);
    if (pane && hash) {
      requestAnimationFrame(() => {
        const el = headings.find(h => h.id === hash);
        if (!el) return;
        const paneTop = pane.getBoundingClientRect().top;
        const elTop = el.getBoundingClientRect().top - paneTop + pane.scrollTop - 20;
        pane.scrollTo({ top: elTop, behavior: 'instant' });
      });
    }
  }, [parts, setToc, contentPaneRef]);

  const fmPanel = doc.hasFrontmatter && doc.frontmatterFields?.length ? (
    <div className="fm-panel">
      <div className="fm-header">Frontmatter</div>
      <div className="fm-fields">
        {doc.frontmatterFields.map(f => (
          <div className="fm-field" key={f.Key}>
            <div className="fm-key">{f.Key}</div>
            <div className="fm-value">{f.Value}</div>
          </div>
        ))}
      </div>
    </div>
  ) : null;

  return (
    <div className="doc-wrap">
      {fmPanel}
      <article
        ref={articleRef}
        className="markdown-body"
        onClick={event => handleArticleLinkClick(event, doc.filePath)}
      >
        <MarkdownContent parts={parts} theme={theme} />
      </article>
    </div>
  );
}
