import type { SlideWidgetRegistry } from '../types';

type WidgetContext = Record<string, unknown>;

const yaoPositionNames = ['初', '二', '三', '四', '五', '上'];

export function safeWidgetClassName(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'unknown';
}

function escapeHTML(value: unknown): string {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function readPath(context: WidgetContext, path: string): unknown {
  const key = path.trim();
  if (key === 'this') return context.this;
  if (key === '@index') return context['@index'];
  return key.split('.').reduce<unknown>((value, segment) => {
    if (value && typeof value === 'object') return (value as Record<string, unknown>)[segment];
    return undefined;
  }, context);
}

function yaoLabelForLine(char: string, index: number): string {
  const number = char === '0' ? '六' : '九';
  const position = yaoPositionNames[index];
  if (!position) return `${number}${index + 1}`;
  if (index === 0) return `${position}${number}`;
  if (index === 5) return `${position}${number}`;
  return `${number}${position}`;
}

function stripYaoPrefix(text: string, index: number): string {
  const patterns = [
    /^初(?:九|六)?[：:、，,\s]*/,
    /^(?:九|六)?二[：:、，,\s]*/,
    /^(?:九|六)?三[：:、，,\s]*/,
    /^(?:九|六)?四[：:、，,\s]*/,
    /^(?:九|六)?五[：:、，,\s]*/,
    /^上(?:九|六)?[：:、，,\s]*/,
  ];
  return text.replace(patterns[index] || /^\s*/, '').trim();
}

function splitNoteSegments(note: string): string[] {
  return note.split(/[｜|]/).map(segment => segment.trim()).filter(Boolean);
}

function extractYaoTexts(note: unknown): string[] {
  if (typeof note !== 'string') return [];
  const yaoSegment = splitNoteSegments(note).find(segment => /^爻\s*[:：]/.test(segment));
  if (!yaoSegment) return [];
  return yaoSegment
    .replace(/^爻\s*[:：]\s*/, '')
    .split(/[；;]/)
    .map(segment => segment.trim())
    .filter(Boolean)
    .map((segment, index) => stripYaoPrefix(segment, index));
}

function noteWithoutYao(note: unknown): string {
  if (typeof note !== 'string') return '';
  const segments = splitNoteSegments(note);
  if (!segments.some(segment => /^爻\s*[:：]/.test(segment))) return note;
  return segments.filter(segment => !/^爻\s*[:：]/.test(segment)).join('｜');
}

function flipLine(lines: string, index: number): string {
  if (index < 0 || index >= lines.length) return lines;
  const next = lines[index] === '1' ? '0' : '1';
  return `${lines.slice(0, index)}${next}${lines.slice(index + 1)}`;
}

function renderLineHtml(char: string, className: string): string {
  const polarity = char === '1' ? 'yang' : 'yin';
  return `<div class="${className} ${polarity}"><span></span><span></span></div>`;
}

function renderMiniLinesHtml(lines: string, movingIndex: number): string {
  return lines.split('').map((char, index) => {
    const movingClass = index === movingIndex ? ' is-moving' : '';
    return renderLineHtml(char, `bianyao-mini-line${movingClass}`);
  }).join('');
}

function buildBianyaoChangesHtml(data: Record<string, unknown>): string {
  const lines = String(data.lines || '').trim();
  if (!/^[01]{6}$/.test(lines)) return '';
  const changes: string[] = [];
  for (let index = 1; index <= 6; index += 1) {
    const value = data[`change${index}`];
    if (typeof value === 'string' && value.trim()) changes.push(value.trim());
  }

  return changes.map((change, index) => {
    const [label = yaoLabelForLine(lines[index] || '', index), yao = '', target = '', targetSubtitle = '', insight = ''] = change.split('||').map(part => part.trim());
    const targetLines = flipLine(lines, index);
    return `<article class="bianyao-change" style="--i:${index}">
      <div class="bianyao-mini-lines" aria-label="${escapeHTML(target)} ${escapeHTML(targetLines)}">${renderMiniLinesHtml(targetLines, index)}</div>
      <div class="bianyao-change-copy">
        <div class="bianyao-change-top">
          <span class="bianyao-change-label">${escapeHTML(label)}</span>
          <span class="bianyao-change-target">${escapeHTML(target)}</span>
        </div>
        <div class="bianyao-change-yao">${escapeHTML(yao)}</div>
        <div class="bianyao-change-insight">${escapeHTML(targetSubtitle ? `${targetSubtitle}。${insight}` : insight)}</div>
      </div>
    </article>`;
  }).join('');
}

function enrichData(data: Record<string, unknown>): WidgetContext {
  const context: WidgetContext = { ...data };
  const yaoTexts = extractYaoTexts(data.note);
  if (typeof data.note === 'string') {
    context.noteSummary = noteWithoutYao(data.note);
  }
  for (const [key, value] of Object.entries(data)) {
    if (typeof value === 'string') {
      context[`${key}Chars`] = value.split('').map((char, index) => ({
        value: char,
        index,
        isYang: char === '1',
        isYin: char === '0',
        ...(key === 'lines' ? {
          yaoLabel: yaoLabelForLine(char, index),
          yaoText: yaoTexts[index] || '',
        } : {}),
      }));
    }
  }
  if (data.type === 'bianyao') {
    context.changesHtml = buildBianyaoChangesHtml(data);
  }
  return context;
}

function renderBlocks(template: string, context: WidgetContext): string {
  let html = template.replace(/{{#each\s+([\w.-]+)}}([\s\S]*?){{\/each}}/g, (_match, key: string, body: string) => {
    const value = readPath(context, key);
    const items = Array.isArray(value) ? value : [];
    return items.map((item, index) => renderBlocks(body, { ...context, this: item, '@index': index, ...(typeof item === 'object' && item ? item as Record<string, unknown> : { value: item }) })).join('');
  });

  html = html.replace(/{{#if\s+([\w.-]+)}}([\s\S]*?){{\/if}}/g, (_match, key: string, body: string) => {
    return readPath(context, key) ? renderBlocks(body, context) : '';
  });

  html = html.replace(/{{{\s*([\w.-]+)\s*}}}/g, (_match, key: string) => String(readPath(context, key) ?? ''));
  html = html.replace(/{{\s*([\w@.-]+)\s*}}/g, (_match, key: string) => escapeHTML(readPath(context, key)));
  return html;
}

export function renderWidgetTemplate(template: string, data: Record<string, unknown>): string {
  return renderBlocks(template, enrichData(data));
}

export function renderWidgetFallback(widgetType: string, raw: string, registry: SlideWidgetRegistry): string {
  const escapedType = escapeHTML(widgetType || 'missing type');
  const escapedRaw = escapeHTML(raw);
  const known = Object.keys(registry).sort().join(', ') || 'none';
  return `<div class="slide-widget-fallback"><strong>Unsupported widget: ${escapedType}</strong><pre>${escapedRaw}</pre><small>Available widgets: ${escapeHTML(known)}</small></div>`;
}
