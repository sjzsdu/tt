import { useMemo } from 'react';
import { marked, type Token } from 'marked';
import hljs from 'highlight.js';
import type { MdPart } from '../types';

// Configure marked to use highlight.js
marked.setOptions({
  highlight: function (code: string, lang: string) {
    if (lang && hljs.getLanguage(lang)) {
      try {
        return hljs.highlight(code, { language: lang }).value;
      } catch (__) {}
    }
    return code;
  },
});

export function useMarkdownParts(markdown: string): MdPart[] {
  return useMemo(() => splitMarkdownParts(markdown), [markdown]);
}

export function splitMarkdownParts(markdown: string): MdPart[] {
  const tokens = marked.lexer(markdown || '');
  const parts: MdPart[] = [];
  let buffer: Token[] = [];

  const flush = () => {
    if (buffer.length) {
      parts.push({ type: 'markdown', html: marked.parser(buffer) });
      buffer = [];
    }
  };

  for (const token of tokens) {
    const codeLang = token.type === 'code'
      ? String((token as Token & { lang?: string }).lang || '')
        .trim()
        .split(/\s+/)[0]
        .toLowerCase()
      : '';
    if (token.type === 'code' && (codeLang === 'mermaid' || codeLang === 'd2')) {
      flush();
      parts.push({
        type: codeLang,
        code: String((token as Token & { text?: string }).text || ''),
      });
    } else {
      buffer.push(token);
    }
  }

  flush();
  return parts;
}
