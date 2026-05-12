import { useEffect, useRef } from 'react';
import type { DocumentResponse, TocItem } from '../types';
import { useMarkdownParts } from '../hooks/useMarkdown';
import { MarkdownContent } from './MarkdownContent';

interface ArticleProps {
  doc: DocumentResponse;
  setToc: (items: TocItem[]) => void;
  setActiveToc: (id: string) => void;
  contentPaneRef: React.RefObject<HTMLElement | null>;
}

function slugify(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replace(/[^\w\u4e00-\u9fa5\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-');
}

function closestHeadingForVisibleElement(element: Element, headings: HTMLHeadingElement[]) {
  const ownHeading = element.closest('h1,h2,h3,h4') as HTMLHeadingElement | null;
  if (ownHeading?.id) return ownHeading.id;

  let active = headings[0]?.id || '';
  for (const heading of headings) {
    if (heading === element || heading.contains(element)) return heading.id;
    const position = heading.compareDocumentPosition(element);
    if (position & Node.DOCUMENT_POSITION_FOLLOWING) active = heading.id;
    if (position & Node.DOCUMENT_POSITION_PRECEDING) break;
  }
  return active;
}

export function Article({ doc, setToc, setActiveToc, contentPaneRef }: ArticleProps) {
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
    if (items[0]) setActiveToc(items[0].id);

    const pane = contentPaneRef.current;
    if (!pane) return;

    const update = () => {
      let active = '';
      const paneRect = pane.getBoundingClientRect();
      const sampleXs = [0.5, 0.35, 0.65].map(ratio => paneRect.left + paneRect.width * ratio);
      const sampleRows: Array<{ x: number; y: number; tag: string; text: string; active: string }> = [];

      for (let y = paneRect.top + 24; y <= paneRect.bottom - 24 && !active; y += 48) {
        for (const x of sampleXs) {
          const element = document.elementFromPoint(x, y);
          if (!element || !article.contains(element)) continue;
          active = closestHeadingForVisibleElement(element, headings);
          if (localStorage.getItem('md-toc-debug') === '1') {
            sampleRows.push({
              x: Math.round(x),
              y: Math.round(y),
              tag: element.tagName.toLowerCase(),
              text: (element.textContent || '').trim().slice(0, 60),
              active,
            });
          }
          break;
        }
      }

      if (!active) active = headings[0]?.id || '';
      if (localStorage.getItem('md-toc-debug') === '1') {
        console.debug('[markdown toc]', {
          active,
          scrollTop: Math.round(pane.scrollTop),
          clientHeight: pane.clientHeight,
          samples: sampleRows,
        });
      }
      if (active) setActiveToc(active);
    };

    pane.addEventListener('scroll', update, { passive: true });
    window.addEventListener('hashchange', update);
    update();

    const hash = window.location.hash.slice(1);
    if (hash) {
      requestAnimationFrame(() => {
        const el = headings.find(h => h.id === hash);
        if (el) {
          const paneTop = pane.getBoundingClientRect().top;
          const elTop = el.getBoundingClientRect().top - paneTop + pane.scrollTop - 20;
          pane.scrollTo({ top: elTop, behavior: 'instant' });
        }
      });
    }

    return () => {
      pane.removeEventListener('scroll', update);
      window.removeEventListener('hashchange', update);
    };
  }, [parts, setToc, setActiveToc, contentPaneRef]);

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
      <article ref={articleRef} className="markdown-body">
        <MarkdownContent parts={parts} />
      </article>
    </div>
  );
}
