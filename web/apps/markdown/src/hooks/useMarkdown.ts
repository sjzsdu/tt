import { useMemo } from 'react';
import { marked, type Token } from 'marked';
import type { MdPart } from '../types';

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
    if (
      token.type === 'code' &&
      String((token as Token & { lang?: string }).lang || '')
        .trim()
        .split(/\s+/)[0] === 'mermaid'
    ) {
      flush();
      parts.push({
        type: 'mermaid',
        code: String((token as Token & { text?: string }).text || ''),
      });
    } else {
      buffer.push(token);
    }
  }

  flush();
  return parts;
}
