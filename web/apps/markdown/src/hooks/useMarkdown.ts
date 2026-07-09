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

function markdownTableColumnWeights(markdown: string): number[][] {
  const tables: number[][] = [];
  const lines = markdown.split('\n');
  for (let i = 1; i < lines.length; i++) {
    const prev = lines[i - 1].trim();
    const current = lines[i].trim();
    if (!prev.includes('|') || !current.includes('|')) continue;
    const cells = current
      .replace(/^\|/, '')
      .replace(/\|$/, '')
      .split('|')
      .map(cell => cell.trim());
    if (cells.length < 2) continue;
    if (!cells.every(cell => /^:?-{1,}:?$/.test(cell))) continue;
    tables.push(cells.map(cell => Math.max(1, (cell.match(/-/g) || []).length)));
  }
  return tables;
}

function applyMarkdownTableColumnWidths(markdown: string, html: string) {
  const weightsList = markdownTableColumnWeights(markdown);
  if (!weightsList.length || !html.includes('<table')) return html;
  let tableIndex = 0;
  return html.replace(/<table(\s[^>]*)?>/g, (match, attrs = '') => {
    const weights = weightsList[tableIndex++];
    if (!weights?.length) return match;
    const total = weights.reduce((sum, value) => sum + value, 0) || weights.length;
    const colgroup = `<colgroup>${weights.map(weight => `<col style="width:${(weight / total * 100).toFixed(4)}%">`).join('')}</colgroup>`;
    const nextAttrs = /\bstyle=/.test(attrs)
      ? attrs.replace(/style=(['"])(.*?)\1/, (_: string, quote: string, value: string) => `style=${quote}table-layout:fixed;${value}${quote}`)
      : `${attrs || ''} style="table-layout:fixed"`;
    return `<table${nextAttrs}>${colgroup}`;
  });
}

export function useMarkdownParts(markdown: string): MdPart[] {
  return useMemo(() => splitMarkdownParts(markdown), [markdown]);
}

export function splitMarkdownParts(markdown: string): MdPart[] {
  const tokens = marked.lexer(markdown || '');
  const parts: MdPart[] = [];
  let buffer: Token[] = [];

  const flush = () => {
    if (buffer.length) {
      parts.push({ type: 'markdown', html: applyMarkdownTableColumnWidths(buffer.map(token => token.raw || '').join(''), marked.parser(buffer)) });
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
