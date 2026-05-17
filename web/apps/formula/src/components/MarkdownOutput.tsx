import { useMemo } from 'react';
import { Modal, Table } from 'antd';
import { marked, type TokensList } from 'marked';

type MarkdownPart =
  | { type: 'html'; html: string }
  | { type: 'table'; headers: string[]; rows: string[][] };

function renderInlineMarkdown(markdown: string) {
  return marked.parseInline(markdown || '') as string;
}

function splitMarkdownParts(markdown: string): MarkdownPart[] {
  const tokens = marked.lexer(markdown || '') as TokensList;
  const parts: MarkdownPart[] = [];
  let buffer: any[] = [];

  const flush = () => {
    if (!buffer.length) return;
    parts.push({ type: 'html', html: marked.parser(buffer as any) as string });
    buffer = [];
  };

  for (const token of tokens as any[]) {
    if (token.type === 'table') {
      flush();
      parts.push({
        type: 'table',
        headers: (token.header || []).map((cell: any) => String(cell.text || cell.raw || '')),
        rows: (token.rows || []).map((row: any) => (row || []).map((cell: any) => String(cell.text || cell.raw || ''))),
      });
      continue;
    }
    buffer.push(token);
  }

  flush();
  return parts;
}

export function MarkdownOutput({ content, className = '' }: { content: string; className?: string }) {
  const parts = useMemo(() => splitMarkdownParts(content), [content]);

  return (
    <article className={`markdown-body formula-markdown-output ${className}`.trim()}>
      {parts.map((part, index) => {
        if (part.type === 'html') {
          return <div key={index} className="markdown-html-block" dangerouslySetInnerHTML={{ __html: part.html }} />;
        }

        const columns = part.headers.map((header, columnIndex) => ({
          title: <span dangerouslySetInnerHTML={{ __html: renderInlineMarkdown(header) }} />,
          dataIndex: `col_${columnIndex}`,
          key: `col_${columnIndex}`,
          render: (value: string) => <div dangerouslySetInnerHTML={{ __html: renderInlineMarkdown(value) }} />,
        }));
        const dataSource = part.rows.map((row, rowIndex) => {
          const record: Record<string, string> = { key: `row_${rowIndex}` };
          row.forEach((cell, columnIndex) => {
            record[`col_${columnIndex}`] = cell;
          });
          return record;
        });

        return (
          <div key={index} className="markdown-table-block">
            <Table
              size="small"
              pagination={false}
              columns={columns}
              dataSource={dataSource}
              scroll={{ x: 'max-content' }}
              className="formula-ant-table"
            />
          </div>
        );
      })}
    </article>
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
