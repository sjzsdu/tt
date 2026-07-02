import type { SlideWidgetRegistry } from '../types';

type WidgetContext = Record<string, unknown>;

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

function enrichData(data: Record<string, unknown>): WidgetContext {
  const context: WidgetContext = { ...data };
  for (const [key, value] of Object.entries(data)) {
    if (typeof value === 'string') {
      context[`${key}Chars`] = value.split('').map((char, index) => ({ value: char, index, isYang: char === '1', isYin: char === '0' }));
    }
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
