import { lazy, Suspense, useMemo, type ReactNode } from 'react';
import { Button, Tag } from 'antd';

const MarkdownOutput = lazy(() => import('./MarkdownOutput').then(module => ({ default: module.MarkdownOutput })));

type OutputKind = 'json' | 'markdown' | 'text' | 'empty';

type ParsedOutput =
  | { kind: 'json'; value: unknown; source: string }
  | { kind: 'markdown' | 'text' | 'empty'; content: string };

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

  if (looksLikeMarkdown(trimmed)) return { kind: 'markdown', content };
  return { kind: 'text', content };
}

function extractJsonCandidate(input: string) {
  const fenced = input.match(/^```(?:json|javascript|js)?\s*([\s\S]*?)\s*```$/i);
  const candidate = fenced ? fenced[1].trim() : input;
  if (!candidate) return '';
  if ((candidate.startsWith('{') && candidate.endsWith('}')) || (candidate.startsWith('[') && candidate.endsWith(']'))) return candidate;
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
    case 'json': return 'JSON';
    case 'markdown': return 'Markdown';
    case 'text': return 'Text';
    default: return 'Empty';
  }
}

function JsonOutput({ value, source }: { value: unknown; source: string }) {
  const copyJson = async () => navigator.clipboard?.writeText(source);
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
  const label = name ? <span className="json-key">{name}</span> : null;

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
  if (typeof value === 'string') {
    const isLongString = value.length > 80 || value.includes('\n');
    return <span className={`json-primitive string ${isLongString ? 'long' : ''}`}>{isLongString ? value : JSON.stringify(value)}</span>;
  }
  if (typeof value === 'number') return <span className="json-primitive number">{value}</span>;
  if (typeof value === 'boolean') return <span className="json-primitive boolean">{String(value)}</span>;
  return <span className={`json-primitive ${type}`}>{String(value)}</span>;
}

function jsonType(value: unknown) {
  if (value === null) return 'null';
  if (Array.isArray(value)) return 'array';
  return typeof value;
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
      {parsed.kind === 'markdown' && (
        <Suspense fallback={<pre className="code-block auto-output-text">{parsed.content}</pre>}>
          <MarkdownOutput content={parsed.content} theme={(document.documentElement.dataset.theme as 'light' | 'dark') || 'dark'} />
        </Suspense>
      )}
      {parsed.kind === 'text' && <pre className="code-block auto-output-text">{parsed.content}</pre>}
      {parsed.kind === 'empty' && <div className="auto-output-empty">No output</div>}
    </div>
  );
}

export function OutputSurface({ content, className = '' }: { content: string; className?: string }) {
  return (
    <div className={`output-surface ${className}`.trim()}>
      <AutoOutput content={content} />
    </div>
  );
}
