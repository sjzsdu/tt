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

function splitParts(markdown: string): SlidePart[] {
  const tokens = marked.lexer(markdown);
  const parts: SlidePart[] = [];
  let buffer: Token[] = [];

  const flush = () => {
    if (buffer.length) {
      parts.push({ type: 'markdown', html: marked.parser(buffer) });
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

function parseSlideMeta(line: string): { key: string; value: string } | null {
  const match = line.match(/^(\w[\w-]*):\s*(.+)$/);
  if (!match) return null;
  return { key: match[1].toLowerCase(), value: match[2].trim() };
}

function parseLayoutFromParts(parts: SlidePart[]): SlideLayout {
  for (const part of parts) {
    if (part.type !== 'markdown') continue;
    const html = part.html;
    if (html.includes('class="two-column"') || html.includes(':::columns')) return 'two-column';
    if (html.includes('class="split"')) return 'split';
    if (html.includes('class="center"')) return 'center';
  }
  return 'default';
}

export function parseSlides(markdown: string): { slides: SlideData[]; meta: SlideMeta } {
  const lines = markdown.split('\n');
  let title = '';
  let template = 'dark';
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
          case 'template': template = parsed.value; break;
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

    const parts = splitParts(trimmed);
    const slideLayout = parseLayoutFromParts(parts);

    let slideClass = '';
    if (slideLayout === 'center') slideClass = 'slide-center';
    else if (slideLayout === 'two-column') slideClass = 'slide-two-column';
    else if (slideLayout === 'split') slideClass = 'slide-split';

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
    meta: { title, template, layout, total: slides.length, transition },
  };
}
