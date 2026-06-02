import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Alert, Button, Empty, Input, Modal, Tag } from 'antd';
import type { FinalReportChat } from '../types';
import { marked, type TokensList } from 'marked';

type AppTheme = 'light' | 'dark';

type MarkdownPart =
  | { type: 'html'; html: string }
  | { type: 'table'; headers: string[]; rows: string[][] }
  | { type: 'mermaid'; code: string };

type OutputKind = 'json' | 'markdown' | 'text' | 'empty';

type ParsedOutput =
  | { kind: 'json'; value: unknown; source: string }
  | { kind: 'markdown' | 'text' | 'empty'; content: string };

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
    const lang = String(token.lang || '').trim().toLowerCase();
    if (token.type === 'code' && lang === 'mermaid') {
      flush();
      parts.push({ type: 'mermaid', code: String(token.text || token.raw || '') });
      continue;
    }
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

function mermaidConfig(theme: AppTheme) {
  const dark = theme === 'dark';
  return {
    startOnLoad: false,
    securityLevel: 'loose' as const,
    theme: 'base' as const,
    themeVariables: dark ? {
      darkMode: true,
      background: 'transparent',
      mainBkg: '#0f2238',
      secondBkg: '#172b45',
      tertiaryBkg: '#1e3553',
      primaryColor: '#0f2238',
      primaryBorderColor: '#67e8f9',
      primaryTextColor: '#e0f2fe',
      secondaryColor: '#172b45',
      secondaryBorderColor: '#a78bfa',
      secondaryTextColor: '#f5f3ff',
      tertiaryColor: '#1e3553',
      tertiaryBorderColor: '#34d399',
      tertiaryTextColor: '#ecfdf5',
      lineColor: '#93c5fd',
      edgeLabelBackground: '#071322',
      clusterBkg: '#081827',
      clusterBorder: '#38bdf8',
      defaultLinkColor: '#93c5fd',
      titleColor: '#e0f2fe',
      nodeTextColor: '#e0f2fe',
      textColor: '#e5e7eb',
      labelTextColor: '#e5e7eb',
    } : {
      darkMode: false,
      background: 'transparent',
      mainBkg: '#ffffff',
      secondBkg: '#f8fafc',
      tertiaryBkg: '#eef4ff',
      primaryColor: '#ffffff',
      primaryBorderColor: '#38bdf8',
      primaryTextColor: '#0f172a',
      secondaryColor: '#f8fafc',
      secondaryBorderColor: '#a78bfa',
      secondaryTextColor: '#0f172a',
      tertiaryColor: '#eef4ff',
      tertiaryBorderColor: '#22c55e',
      tertiaryTextColor: '#0f172a',
      lineColor: '#64748b',
      edgeLabelBackground: '#f8fafc',
      clusterBkg: '#f8fafc',
      clusterBorder: '#38bdf8',
      defaultLinkColor: '#64748b',
      titleColor: '#0f172a',
      nodeTextColor: '#0f172a',
      textColor: '#0f172a',
      labelTextColor: '#0f172a',
    },
  };
}

function MermaidDiagram({ code, theme }: { code: string; theme: AppTheme }) {
  const [svg, setSvg] = useState('');
  const [error, setError] = useState('');
  const idRef = useRef(`formula-mermaid-${Math.random().toString(36).slice(2)}`);

  useEffect(() => {
    let cancelled = false;
    setError('');
    setSvg('');
    void import('mermaid')
      .then(module => {
        const mermaid = module.default;
        mermaid.initialize(mermaidConfig(theme));
        return mermaid.render(idRef.current, code);
      })
      .then(result => {
        if (!cancelled) setSvg(result.svg);
      })
      .catch(err => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => { cancelled = true; };
  }, [code, theme]);

  if (error) {
    return <pre className="code-block error-block">Mermaid render failed: {error}\n\n{code}</pre>;
  }
  if (!svg) {
    return <div className="mermaid-diagram mermaid-loading">Rendering diagram…</div>;
  }
  return <div className="mermaid-diagram" dangerouslySetInnerHTML={{ __html: svg }} />;
}

function parseOutput(content: string): ParsedOutput {
  const trimmed = (content || '').trim();
  if (!trimmed) return { kind: 'empty', content: '' };

  const jsonCandidate = extractJsonCandidate(trimmed);
  if (jsonCandidate) {
    try {
      const parsed = normalizeJsonValue(JSON.parse(jsonCandidate));
      return { kind: 'json', value: parsed, source: JSON.stringify(parsed, null, 2) };
    } catch {
    }
  }

  const parsedStringJson = parseJsonString(trimmed);
  if (parsedStringJson.ok) {
    const parsed = normalizeJsonValue(parsedStringJson.value);
    return { kind: 'json', value: parsed, source: JSON.stringify(parsed, null, 2) };
  }

  if (looksLikeMarkdown(trimmed)) {
    return { kind: 'markdown', content };
  }
  return { kind: 'text', content };
}

function extractJsonCandidate(input: string) {
  const fenced = input.match(/^```(?:json|javascript|js)?\s*([\s\S]*?)\s*```$/i);
  const candidate = fenced ? fenced[1].trim() : input;
  if (!candidate) return '';
  if ((candidate.startsWith('{') && candidate.endsWith('}')) || (candidate.startsWith('[') && candidate.endsWith(']'))) {
    return candidate;
  }
  return '';
}

function parseJsonString(input: string): { ok: true; value: unknown } | { ok: false } {
  const unwrapped = unwrapStringLiteral(input);
  if (!unwrapped) return { ok: false };
  const nestedCandidate = extractJsonCandidate(unwrapped.trim());
  if (!nestedCandidate) return { ok: false };
  try {
    return { ok: true, value: JSON.parse(nestedCandidate) };
  } catch {
    return { ok: false };
  }
}

function unwrapStringLiteral(input: string) {
  try {
    const parsed = JSON.parse(input);
    return typeof parsed === 'string' ? parsed : '';
  } catch {
    if ((input.startsWith('"') && input.endsWith('"')) || (input.startsWith("'") && input.endsWith("'"))) {
      return input.slice(1, -1).replace(/\\n/g, '\n').replace(/\\"/g, '"').replace(/\\'/g, "'");
    }
    return '';
  }
}

function normalizeJsonValue(value: unknown): unknown {
  if (typeof value === 'string') {
    const nestedCandidate = extractJsonCandidate(value.trim());
    if (nestedCandidate) {
      try {
        return normalizeJsonValue(JSON.parse(nestedCandidate));
      } catch {
        return value;
      }
    }
    return value;
  }
  if (Array.isArray(value)) return value.map(normalizeJsonValue);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value as Record<string, unknown>).map(([key, item]) => [key, normalizeJsonValue(item)]));
  }
  return value;
}

function looksLikeMarkdown(input: string) {
  return /(^#{1,6}\s)|(^[-*+]\s)|(^\d+\.\s)|(```)|(`[^`]+`)|(\[[^\]]+\]\([^)]+\))|(^>\s)|(^\|.*\|$)/m.test(input);
}

function outputLabel(kind: OutputKind) {
  switch (kind) {
    case 'json':
      return 'JSON';
    case 'markdown':
      return 'Markdown';
    case 'text':
      return 'Text';
    default:
      return 'Empty';
  }
}

export function MarkdownOutput({ content, className = '', theme }: { content: string; className?: string; theme: AppTheme }) {
  const parts = useMemo(() => splitMarkdownParts(content), [content]);

  return (
    <article className={`markdown-body formula-markdown-output ${className}`.trim()}>
      {parts.map((part, index) => {
        if (part.type === 'html') {
          return <div key={index} className="markdown-html-block" dangerouslySetInnerHTML={{ __html: part.html }} />;
        }

        if (part.type === 'mermaid') {
          return <MermaidDiagram key={index} code={part.code} theme={theme} />;
        }

        return (
          <div key={index} className="markdown-table-block">
            <div className="markdown-table-scroll">
              <table className="formula-markdown-table">
                <thead>
                  <tr>
                    {part.headers.map((header, columnIndex) => (
                      <th key={`head_${columnIndex}`}>
                        <span dangerouslySetInnerHTML={{ __html: renderInlineMarkdown(header) }} />
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {part.rows.map((row, rowIndex) => (
                    <tr key={`row_${rowIndex}`}>
                      {row.map((cell, columnIndex) => (
                        <td key={`cell_${rowIndex}_${columnIndex}`}>
                          <div dangerouslySetInnerHTML={{ __html: renderInlineMarkdown(cell) }} />
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        );
      })}
    </article>
  );
}

function renderJsonKeyLabel(name: string) {
  const segments = name.split(/([._\-/]+)/);
  return segments.map((segment, index) => (
    <Fragment key={`${segment}-${index}`}>
      {segment}
      {index < segments.length - 1 ? <wbr /> : null}
    </Fragment>
  ));
}

function JsonOutput({ value, source }: { value: unknown; source: string }) {
  const copyJson = async () => {
    await navigator.clipboard?.writeText(source);
  };

  return (
    <div className="json-output-shell">
      <div className="json-output-actions">
        <span>Structured JSON</span>
        <Button size="small" type="text" onClick={copyJson}>Copy pretty JSON</Button>
      </div>
      <JsonValue value={value} depth={0} name="root" />
    </div>
  );
}

function JsonValue({ value, name, depth }: { value: unknown; name?: string; depth: number }): ReactNode {
  const type = jsonType(value);
  const isContainer = type === 'object' || type === 'array';
  const label = name ? <span className="json-key">{renderJsonKeyLabel(name)}</span> : null;

  if (value === null || !isContainer) {
    return (
      <div className="json-row" style={{ paddingLeft: depth * 16 }}>
        {label ? <span className="json-key-label">{label}</span> : null}
        {label && <span className="json-separator">:</span>}
        <span className="json-value-slot"><PrimitiveValue value={value} /></span>
      </div>
    );
  }

  const entries = Array.isArray(value)
    ? value.map((item, index) => [String(index), item] as const)
    : Object.entries(value as Record<string, unknown>);

  if (Array.isArray(value) && value.every(item => item === null || !isJsonContainer(item))) {
    return (
      <div className="json-array-row" style={{ paddingLeft: depth * 16 }}>
        {label ? <span className="json-key-label">{label}</span> : null}
        {label && <span className="json-separator">:</span>}
        <div className="json-array-chips">
          {value.length === 0 && <span className="json-empty-array">empty array</span>}
          {value.map((item, index) => (
            <span key={index} className="json-array-chip"><PrimitiveValue value={item} /></span>
          ))}
        </div>
      </div>
    );
  }

  return (
    <details className="json-node" open={depth < 2} style={{ marginLeft: depth * 12 }}>
      <summary className="json-summary">
        {label ? <span className="json-key-label">{label}</span> : null}
        {label && <span className="json-separator">:</span>}
        <Tag className="json-type-tag">{Array.isArray(value) ? 'array' : 'object'}</Tag>
        <span className="json-count">{entries.length} item{entries.length === 1 ? '' : 's'}</span>
      </summary>
      <div className="json-children">
        {entries.map(([entryKey, entryValue]) => (
          <JsonValue key={entryKey} name={Array.isArray(value) ? `#${Number(entryKey) + 1}` : entryKey} value={entryValue} depth={depth + 1} />
        ))}
      </div>
    </details>
  );
}

function PrimitiveValue({ value }: { value: unknown }) {
  const type = jsonType(value);
  if (value === null) return <span className="json-primitive null">null</span>;
  if (typeof value === 'string') return <span className="json-primitive string">{JSON.stringify(value)}</span>;
  if (typeof value === 'number') return <span className="json-primitive number">{value}</span>;
  if (typeof value === 'boolean') return <span className="json-primitive boolean">{String(value)}</span>;
  return <span className={`json-primitive ${type}`}>{String(value)}</span>;
}

function jsonType(value: unknown) {
  if (value === null) return 'null';
  if (Array.isArray(value)) return 'array';
  return typeof value;
}

function isJsonContainer(value: unknown) {
  return value !== null && (Array.isArray(value) || typeof value === 'object');
}

export function AutoOutput({ content, className = '' }: { content: string; className?: string }) {
  const parsed = useMemo(() => parseOutput(content), [content]);

  return (
    <div className={`auto-output ${className}`.trim()}>
      <div className="auto-output-toolbar">
        <span>Auto rendered</span>
        <Tag>{outputLabel(parsed.kind)}</Tag>
      </div>
      {parsed.kind === 'json' && <JsonOutput value={parsed.value} source={parsed.source} />}
      {parsed.kind === 'markdown' && <MarkdownOutput content={parsed.content} theme={(document.documentElement.dataset.theme as AppTheme) || 'dark'} />}
      {parsed.kind === 'text' && <pre className="code-block auto-output-text">{parsed.content}</pre>}
      {parsed.kind === 'empty' && <div className="auto-output-empty">No output</div>}
    </div>
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
      <AutoOutput content={content} />
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
    <Modal open={open} onCancel={onClose} footer={null} width="88vw" title={title} className={className}>
      <div className="final-output-shell">
        <div className="final-output-kicker">Rendered report</div>
        <OutputSurface content={content} />
      </div>
    </Modal>
  );
}

export function FinalReportModal({
  open,
  title,
  content,
  className,
  chat,
  chatBusy,
	chatError,
	onClose,
	onStartChat,
	onSendMessage,
	onPromoteLatest,
}: {
  open: boolean;
  title: string;
  content: string;
  className?: string;
  chat?: FinalReportChat;
  chatBusy?: boolean;
  chatError?: string;
	onClose: () => void;
	onStartChat: () => void;
	onSendMessage: (message: string) => Promise<void> | void;
	onPromoteLatest: () => Promise<void> | void;
}) {
  const [draft, setDraft] = useState('');
  const messages = chat?.messages || [];
  const reportAvailable = !!content?.trim();
	const canPromote = messages.some(item => item.role === 'assistant' && item.content?.trim());

  useEffect(() => {
    if (!open) setDraft('');
  }, [open]);

  const submit = async () => {
    const next = draft.trim();
    if (!next || chatBusy || !reportAvailable) return;
    setDraft('');
    await onSendMessage(next);
  };

  return (
    <Modal open={open} onCancel={onClose} footer={null} width="92vw" title={title} className={className}>
      <div className="final-report-modal-layout">
        <section className="final-report-pane">
          <div className="final-output-kicker">Final report</div>
          <OutputSurface content={content} />
        </section>
        <aside className="final-report-chat-pane">
          <div className="final-report-chat-header">
            <div>
              <div className="final-output-kicker">Chat with coder</div>
              <strong>Ask coder to revise, extend, or validate this final report.</strong>
              <p>Uses a separate session from the formula run.</p>
            </div>
			{chat?.session_id ? <Tag>{chat.session_id}</Tag> : null}
		  </div>
		  {canPromote ? (
			<div className="final-report-chat-promote">
			  <Button onClick={onPromoteLatest} disabled={chatBusy}>Use latest assistant reply as final report</Button>
			</div>
		  ) : null}
		  {chatError ? <Alert type="error" showIcon message={chatError} style={{ marginBottom: 12 }} /> : null}
          {!reportAvailable ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Chat is unavailable because no final report was produced." />
          ) : messages.length === 0 ? (
            <div className="final-report-chat-empty">
              <p>Replies use the report as context.</p>
              <Button onClick={onStartChat} loading={chatBusy}>Start chat</Button>
            </div>
          ) : (
            <div className="final-report-chat-messages">
              {messages.map((item, index) => (
                <div key={`${item.at || index}-${index}`} className={`final-report-chat-message ${item.role}`}>
                  <div className="final-report-chat-role">{item.role}</div>
                  <div className="final-report-chat-content"><AutoOutput content={item.content} /></div>
                </div>
              ))}
              {chatBusy ? <div className="final-report-chat-thinking">Coder is thinking…</div> : null}
            </div>
          )}
          <div className="final-report-chat-composer">
            <Input.TextArea value={draft} onChange={e => setDraft(e.target.value)} rows={4} placeholder="Ask coder to improve or modify this report..." disabled={!reportAvailable || chatBusy} />
            <div className="final-report-chat-actions">
              {!messages.length ? <Button onClick={onStartChat} disabled={!reportAvailable || chatBusy}>Start chat</Button> : null}
              <Button type="primary" onClick={submit} loading={chatBusy} disabled={!draft.trim() || !reportAvailable}>Send</Button>
            </div>
          </div>
        </aside>
      </div>
    </Modal>
  );
}
