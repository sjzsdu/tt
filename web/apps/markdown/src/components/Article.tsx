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

function headingTopInPane(heading: HTMLHeadingElement, pane: HTMLElement) {
  let top = 0;
  let node: HTMLElement | null = heading;

  while (node && node !== pane) {
    top += node.offsetTop;
    node = node.offsetParent as HTMLElement | null;
  }

  return top;
}

function logHeadingDiagnostics(headings: HTMLHeadingElement[], pane: HTMLElement, active: string) {
  if (localStorage.getItem('md-toc-debug') !== '1') return;
  console.debug('[markdown toc]', {
    active,
    scrollTop: Math.round(pane.scrollTop),
    clientHeight: pane.clientHeight,
    headings: headings.map(heading => {
      const rect = heading.getBoundingClientRect();
      const style = getComputedStyle(heading);
      return {
        id: heading.id,
        text: (heading.textContent || '').trim().slice(0, 60),
        topInPane: Math.round(headingTopInPane(heading, pane)),
        offsetTop: heading.offsetTop,
        offsetParent: heading.offsetParent instanceof HTMLElement ? heading.offsetParent.className || heading.offsetParent.tagName : null,
        rect: {
          top: Math.round(rect.top),
          bottom: Math.round(rect.bottom),
          height: Math.round(rect.height),
        },
        offsetHeight: heading.offsetHeight,
        clientRects: heading.getClientRects().length,
        display: style.display,
        visibility: style.visibility,
        position: style.position,
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
    const setActive = (id: string) => {
      if (!id || id === activeId) return;
      activeId = id;
      setActiveToc(id);
      logHeadingDiagnostics(headings, pane, id);
    };

    const updateByScrollPosition = () => {
      if (!headings.length) return;
      const target = pane.scrollTop + 96;
      let next = headings[0].id;
      for (const heading of headings) {
        if (headingTopInPane(heading, pane) <= target) next = heading.id;
        else break;
      }
      setActive(next);
    };

    const observer = new IntersectionObserver(
      entries => {
        const visible = entries
          .filter(entry => entry.isIntersecting)
          .map(entry => entry.target as HTMLHeadingElement)
          .sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top);
        if (visible[0]) setActive(visible[0].id);
        else updateByScrollPosition();
      },
      { root: pane, rootMargin: '-80px 0px -65% 0px', threshold: [0, 1] }
    );

    headings.forEach(heading => observer.observe(heading));
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
      observer.disconnect();
      pane.removeEventListener('scroll', updateByScrollPosition);
      window.removeEventListener('hashchange', updateByScrollPosition);
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
