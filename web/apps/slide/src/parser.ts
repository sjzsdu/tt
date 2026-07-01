import { marked, type Token } from 'marked';
import hljs from 'highlight.js';
import type { SlideData, SlidePart, SlideMeta, SlideLayout } from './types';

marked.setOptions({
  highlight: function (code: string, lang: string) {
    if (lang && hljs.getLanguage(lang)) {
      try {
        return hljs.highlight(code, { language: lang }).value;
      } catch (_) {}
    }
    return code;
  },
});

type MarkdownRole = Extract<SlidePart, { type: 'markdown' }>['role'];

type MarkdownSegment = {
  markdown: string;
  role?: MarkdownRole;
};

type ParseOptions = {
  assetBasePath?: string;
};

type SlideDirective = {
  markdown: string;
  layoutHint?: SlideLayout;
  classNames: string[];
  hasDirective: boolean;
};

const slideDirectivePattern = /^\.(center|logo|brand|split|two-column|columns|cover|closing|end|final)\s*$/i;

function isExternalOrSpecialUrl(value: string) {
  return /^(?:[a-z][a-z0-9+.-]*:|#|\/)/i.test(value);
}

function joinRawAssetPath(basePath: string, value: string) {
  const match = value.match(/^([^?#]*)([?#].*)?$/);
  const pathPart = match?.[1] || '';
  const suffix = match?.[2] || '';
  const base = basePath.replace(/^\/+|\/+$/g, '');
  const combined = base ? `${base}/${pathPart}` : pathPart;
  const normalized: string[] = [];
  for (const segment of combined.split('/')) {
    if (!segment || segment === '.') continue;
    if (segment === '..') {
      normalized.pop();
      continue;
    }
    normalized.push(segment);
  }
  return `/raw/${normalized.map(encodeURIComponent).join('/')}${suffix}`;
}

function rewriteRelativeUrls(html: string, assetBasePath = '') {
  if (!assetBasePath) return html;
  return html.replace(/\b(src|href)=(['"])([^'"]+)\2/g, (match, attr: string, quote: string, value: string) => {
    if (isExternalOrSpecialUrl(value)) return match;
    return `${attr}=${quote}${joinRawAssetPath(assetBasePath, value)}${quote}`;
  });
}

function extractSlideDirectives(markdown: string): SlideDirective {
  const classNames: string[] = [];
  let layoutHint: SlideLayout | undefined;
  let hasDirective = false;
  const lines: string[] = [];

  for (const line of markdown.split('\n')) {
    const match = line.trim().match(slideDirectivePattern);
    if (!match) {
      lines.push(line);
      continue;
    }

    hasDirective = true;
    const directive = match[1].toLowerCase();
    if (directive === 'center') {
      layoutHint = 'center';
    } else if (directive === 'logo' || directive === 'brand') {
      layoutHint = 'logo';
    } else if (directive === 'closing' || directive === 'end' || directive === 'final') {
      layoutHint = 'closing';
    } else if (directive === 'split') {
      layoutHint = 'split';
    } else if (directive === 'two-column' || directive === 'columns') {
      layoutHint = 'two-column';
    }
    classNames.push(`slide-${directive}`);
  }

  return { markdown: lines.join('\n').trim(), layoutHint, classNames, hasDirective };
}

function splitMarkedParts(markdown: string, role?: MarkdownRole, options: ParseOptions = {}): SlidePart[] {
  const source = markdown.trim();
  if (!source) return [];

  const tokens = marked.lexer(markdown);
  const parts: SlidePart[] = [];
  let buffer: Token[] = [];

  const flush = () => {
    if (buffer.length) {
      parts.push({ type: 'markdown', html: rewriteRelativeUrls(marked.parser(buffer), options.assetBasePath), role });
      buffer = [];
    }
  };

  for (const token of tokens) {
    const codeLang = token.type === 'code'
      ? String((token as Token & { lang?: string }).lang || '').trim().split(/\s+/)[0].toLowerCase()
      : '';
    if (token.type === 'code' && (codeLang === 'mermaid' || codeLang === 'd2')) {
      flush();
      parts.push({ type: codeLang, code: String((token as Token & { text?: string }).text || '') });
    } else {
      buffer.push(token);
    }
  }

  flush();
  return parts;
}

function splitColumnSegments(markdown: string): MarkdownSegment[] | null {
  const lines = markdown.split('\n');
  const segments: MarkdownSegment[] = [];
  let buffer: string[] = [];
  let inColumn = false;
  let sawColumn = false;

  const flush = (role?: MarkdownRole) => {
    const text = buffer.join('\n').trim();
    if (text) {
      segments.push({ markdown: text, role });
    }
    buffer = [];
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (!inColumn && /^:::\s*columns\s*$/i.test(trimmed)) {
      flush();
      inColumn = true;
      sawColumn = true;
      continue;
    }
    if (inColumn && /^:::\s*$/.test(trimmed)) {
      flush('column');
      inColumn = false;
      continue;
    }
    buffer.push(line);
  }

  if (inColumn) return null;
  flush();

  return sawColumn ? segments : null;
}

function splitParts(markdown: string, options: ParseOptions = {}): SlidePart[] {
  const segments = splitColumnSegments(markdown);
  if (!segments) return splitMarkedParts(markdown, undefined, options);

  return segments.flatMap(segment => splitMarkedParts(segment.markdown, segment.role, options));
}

function parseSlideMeta(line: string): { key: string; value: string } | null {
  const match = line.match(/^(\w[\w-]*):\s*(.+)$/);
  if (!match) return null;
  return { key: match[1].toLowerCase(), value: match[2].trim() };
}

function parseLayoutFromParts(parts: SlidePart[]): SlideLayout {
  if (parts.some(part => part.type === 'markdown' && part.role === 'column')) return 'two-column';
  for (const part of parts) {
    if (part.type !== 'markdown') continue;
    const html = part.html;
    if (html.includes('class="two-column"') || html.includes(':::columns')) return 'two-column';
    if (html.includes('class="split"')) return 'split';
    if (html.includes('class="center"')) return 'center';
  }
  return 'default';
}

function classForLayout(layout: SlideLayout): string {
  if (layout === 'center') return 'slide-center';
  if (layout === 'two-column') return 'slide-two-column';
  if (layout === 'split') return 'slide-split';
  if (layout === 'logo') return 'slide-logo';
  if (layout === 'closing') return 'slide-closing';
  return '';
}

function plainTextLength(html: string): number {
  return html.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim().length;
}

function classesForSlideDensity(parts: SlidePart[], layout: SlideLayout, index: number): string[] {
  const diagramCount = parts.filter(part => part.type === 'mermaid' || part.type === 'd2').length;
  const markdownParts = parts.filter((part): part is Extract<SlidePart, { type: 'markdown' }> => part.type === 'markdown');
  const textLength = markdownParts.reduce((total, part) => total + plainTextLength(part.html), 0);
  const classes: string[] = [];
  if (diagramCount > 0) {
    classes.push('slide-diagram-heavy');
  }
  if (diagramCount > 0 && textLength <= 80) {
    classes.push('slide-diagram-only');
  }
  if (layout === 'default' && index > 0 && diagramCount === 0 && textLength > 0 && textLength <= 140 && markdownParts.length <= 1) {
    classes.push('slide-sparse');
  }
  return classes;
}

export function parseSlides(markdown: string, options: ParseOptions = {}): { slides: SlideData[]; meta: SlideMeta } {
  const lines = markdown.split('\n');
  let title = '';
  let layout: SlideLayout = 'default';
  let transition = '';

  let bodyStart = 0;
  if (lines[0]?.trim() === '---') {
    let endIdx = -1;
    for (let i = 1; i < lines.length; i++) {
      if (lines[i].trim() === '---') {
        endIdx = i;
        break;
      }
    }
    if (endIdx > 0) {
      for (let i = 1; i < endIdx; i++) {
        const parsed = parseSlideMeta(lines[i].trim());
        if (!parsed) continue;
        switch (parsed.key) {
          case 'title': title = parsed.value; break;
          case 'layout': layout = parsed.value as SlideLayout; break;
          case 'transition': transition = parsed.value; break;
        }
      }
      bodyStart = endIdx + 1;
    }
  }

  const body = lines.slice(bodyStart).join('\n');
  const rawSlides = body.split(/\n---\n/);

  const slides: SlideData[] = [];
  let idx = 0;
  for (const raw of rawSlides) {
    const trimmed = raw.trim();
    if (!trimmed) continue;

    const directives = extractSlideDirectives(trimmed);
    const parts = splitParts(directives.markdown, options);
    if (parts.length === 0 && !directives.hasDirective) continue;

    const inferredLayout = parseLayoutFromParts(parts);
    const slideLayout = inferredLayout === 'default' && directives.layoutHint ? directives.layoutHint : inferredLayout;
    const slideClass = [idx === 0 ? 'slide-cover' : '', classForLayout(slideLayout), ...directives.classNames, ...classesForSlideDensity(parts, slideLayout, idx)]
      .filter(Boolean)
      .filter((value, index, values) => values.indexOf(value) === index)
      .join(' ');

    slides.push({
      index: idx++,
      parts,
      layout: slideLayout,
      class: slideClass,
    });
  }

  if (!title && slides.length > 0) {
    const firstPart = slides[0].parts[0];
    if (firstPart?.type === 'markdown') {
      const text = firstPart.html.replace(/<[^>]+>/g, '').trim();
      title = text.slice(0, 60) || 'Presentation';
    }
  }

  return {
    slides,
    meta: { title, layout, total: slides.length, transition },
  };
}
