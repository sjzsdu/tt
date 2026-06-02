import { Anchor } from 'antd';
import type { TocItem } from '../types';

interface TocSidebarProps {
  toc: TocItem[];
  contentPaneRef: React.RefObject<HTMLElement | null>;
}

export function TocSidebar({ toc, contentPaneRef }: TocSidebarProps) {
  const anchorItems = toc.map(item => ({
    key: item.id,
    href: `#${item.id}`,
    title: <span className={`toc-anchor-title level-${item.level}`}>{item.text}</span>,
  }));

  return (
    <aside className="toc-pane section">
      <h2 className="toc-title">On this page</h2>
      {toc.length ? (
        <Anchor
          affix={false}
          className="toc-anchor"
          getContainer={() => contentPaneRef.current || window}
          items={anchorItems}
          targetOffset={80}
          onClick={(event, link) => {
            event.preventDefault();
            scrollToHeading(link.href, contentPaneRef.current);
          }}
        />
      ) : (
        <p className="toc-empty">No headings found.</p>
      )}
    </aside>
  );
}

function scrollToHeading(href: string, pane: HTMLElement | null) {
  const id = href.replace(/^#/, '');
  const el = document.getElementById(id);
  if (!el || !pane) return;
  const paneRect = pane.getBoundingClientRect();
  const elRect = el.getBoundingClientRect();
  pane.scrollTo({ top: elRect.top - paneRect.top + pane.scrollTop - 20, behavior: 'smooth' });
  history.replaceState(null, '', href);
}
