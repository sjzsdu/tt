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

type TocMarker = { id: string; text: string; marker: HTMLElement; heading: HTMLHeadingElement };

function topInPane(element: HTMLElement, pane: HTMLElement) {
  let top = 0;
  let node: HTMLElement | null = element;

  while (node && node !== pane) {
    top += node.offsetTop;
    node = node.offsetParent as HTMLElement | null;
  }

  return top;
}

function logHeadingDiagnostics(markers: TocMarker[], pane: HTMLElement, active: string) {
  if (localStorage.getItem('md-toc-debug') !== '1') return;
  console.debug('[markdown toc]', {
    active,
    scrollTop: Math.round(pane.scrollTop),
    clientHeight: pane.clientHeight,
    markers: markers.map(({ id, text, marker, heading }) => {
      const markerRect = marker.getBoundingClientRect();
      const headingRect = heading.getBoundingClientRect();
      return {
        id,
        text: text.slice(0, 60),
        topInPane: Math.round(topInPane(marker, pane)),
        markerRect: { top: Math.round(markerRect.top), height: Math.round(markerRect.height) },
        headingRect: { top: Math.round(headingRect.top), height: Math.round(headingRect.height) },
        markerOffsetTop: marker.offsetTop,
        markerOffsetParent: marker.offsetParent instanceof HTMLElement ? marker.offsetParent.className || marker.offsetParent.tagName : null,
      };
    }),
  });
}

function createTocMarkers(headings: HTMLHeadingElement[], items: TocItem[]): TocMarker[] {
  return headings.map((heading, index) => {
    const item = items[index];
    let marker = heading.previousElementSibling as HTMLElement | null;
    if (!marker || marker.dataset.tocMarker !== item.id) {
      marker = document.createElement('span');
      marker.className = 'toc-scroll-marker';
      marker.dataset.tocMarker = item.id;
      marker.setAttribute('aria-hidden', 'true');
      heading.before(marker);
    }
    return { id: item.id, text: item.text, marker, heading };
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

    const markers = createTocMarkers(headings, items);

    let activeId = items[0]?.id || '';
    const setActive = (id: string) => {
      if (!id || id === activeId) return;
      activeId = id;
      setActiveToc(id);
      logHeadingDiagnostics(markers, pane, id);
    };

    const updateByScrollPosition = () => {
      if (!markers.length) return;
      const target = pane.scrollTop + 96;
      let next = markers[0].id;
      for (const item of markers) {
        if (topInPane(item.marker, pane) <= target) next = item.id;
        else break;
      }
      setActive(next);
    };

    pane.addEventListener('scroll', updateByScrollPosition, { passive: true });
    window.addEventListener('hashchange', updateByScrollPosition);
    requestAnimationFrame(updateByScrollPosition);

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
      pane.removeEventListener('scroll', updateByScrollPosition);
      window.removeEventListener('hashchange', updateByScrollPosition);
      markers.forEach(({ marker }) => marker.remove());
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
