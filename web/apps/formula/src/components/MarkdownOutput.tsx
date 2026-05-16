import { useMemo } from 'react';
import { Modal } from 'antd';
import { marked } from 'marked';

function normalizeMarkdownHTML(html: string) {
  return html.replace(/<table>([\s\S]*?)<\/table>/g, '<div class="markdown-table-wrap"><table>$1</table></div>');
}

function renderMarkdown(markdown: string) {
  return normalizeMarkdownHTML(marked.parse(markdown || '') as string);
}

export function MarkdownOutput({ content, className = '' }: { content: string; className?: string }) {
  const html = useMemo(() => renderMarkdown(content), [content]);

  return (
    <article
      className={`markdown-body formula-markdown-output ${className}`.trim()}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

export function OutputSurface({
  content,
  className = '',
}: {
  content: string;
  className?: string;
}) {
  return (
    <div className={`output-surface ${className}`.trim()}>
      <MarkdownOutput content={content} />
    </div>
  );
}

export function OutputModal({
  open,
  title,
  content,
  className,
  onClose,
}: {
  open: boolean;
  title: string;
  content: string;
  className?: string;
  onClose: () => void;
}) {
  return (
    <Modal open={open} onCancel={onClose} footer={null} width={1040} title={title} className={className}>
      <div className="final-output-shell">
        <div className="final-output-kicker">Rendered report</div>
        <OutputSurface content={content} />
      </div>
    </Modal>
  );
}
