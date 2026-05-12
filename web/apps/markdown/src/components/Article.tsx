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

function isHeading(element: Element): element is HTMLHeadingElement {
  return /H[1-4]/.test(element.tagName);
}

function headingForElement(element: Element, headings: HTMLHeadingElement[]) {
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

function firstVisibleContentElement(article: HTMLElement, pane: HTMLElement) {
  const paneRect = pane.getBoundingClientRect();
  const topLimit = paneRect.top + 96;
  const bottomLimit = paneRect.bottom - 24;
  const elements = Array.from(article.querySelectorAll<HTMLElement>('*'));

  for (const element of elements) {
    if (element.classList.contains('toc-scroll-marker')) continue;
    const style = getComputedStyle(element);
    if (style.display === 'none' || style.visibility === 'hidden') continue;

    const rects = Array.from(element.getClientRects());
    const visibleRect = rects.find(rect =>
      rect.width > 0 &&
      rect.height > 0 &&
      rect.bottom >= topLimit &&
      rect.top <= bottomLimit
    );
    if (visibleRect) return { element, rect: visibleRect };
  }

  return null;
}

function logTocDebug(params: {
  article: HTMLElement;
  pane: HTMLElement;
  headings: HTMLHeadingElement[];
  active: string;
  visible: { element: HTMLElement; rect: DOMRect } | null;
}) {
  if (localStorage.getItem('md-toc-debug') !== '1') return;
  const { article, pane, headings, active, visible } = params;
  const paneRect = pane.getBoundingClientRect();
  console.debug('[markdown toc]', {
    active,
    scrollTop: Math.round(pane.scrollTop),
    clientHeight: pane.clientHeight,
    paneRect: {
      top: Math.round(paneRect.top),
      bottom: Math.round(paneRect.bottom),
      height: Math.round(paneRect.height),
    },
    articleConnected: article.isConnected,
    visible: visible ? {
      tag: visible.element.tagName.toLowerCase(),
      id: visible.element.id,
      className: visible.element.className,
      text: (visible.element.textContent || '').trim().slice(0, 80),
      rect: {
        top: Math.round(visible.rect.top),
        bottom: Math.round(visible.rect.bottom),
        height: Math.round(visible.rect.height),
      },
    } : null,
    headings: headings.map(heading => {
      const rect = heading.getBoundingClientRect();
      return {
        id: heading.id,
        text: (heading.textContent || '').trim().slice(0, 60),
        connected: heading.isConnected,
        rect: {
          top: Math.round(rect.top),
          bottom: Math.round(rect.bottom),
          height: Math.round(rect.height),
        },
        offsetParent: heading.offsetParent instanceof HTMLElement ? heading.offsetParent.className || heading.offsetParent.tagName : null,
      };
    }),
  });
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

    let activeId = items[0]?.id || '';
    let raf = 0;

    const updateActive = () => {
      raf = 0;
      if (!headings.length) return;

      const visible = firstVisibleContentElement(article, pane);
      let next = visible ? headingForElement(visible.element, headings) : activeId || headings[0].id;

      if (!next && visible && isHeading(visible.element)) next = visible.element.id;
      if (!next) next = headings[0].id;

      logTocDebug({ article, pane, headings, active: next, visible });

      if (next && next !== activeId) {
        activeId = next;
        setActiveToc(next);
      }
    };

    const scheduleUpdate = () => {
      if (raf) return;
      raf = requestAnimationFrame(updateActive);
    };

    pane.addEventListener('scroll', scheduleUpdate, { passive: true });
    window.addEventListener('hashchange', scheduleUpdate);
    requestAnimationFrame(updateActive);

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
      if (raf) cancelAnimationFrame(raf);
      pane.removeEventListener('scroll', scheduleUpdate);
      window.removeEventListener('hashchange', scheduleUpdate);
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
