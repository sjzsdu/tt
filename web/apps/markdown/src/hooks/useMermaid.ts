import { useState, useEffect, useRef } from 'react';

import { createMermaidConfig, type AppTheme } from '../utils/mermaidConfig';

type MermaidModule = typeof import('mermaid').default;

let mermaidPromise: Promise<MermaidModule> | null = null;
let initializedTheme: AppTheme | null = null;

function loadMermaid() {
  mermaidPromise ||= import('mermaid').then(module => module.default);
  return mermaidPromise;
}

function normalizeMermaidCode(code: string) {
  return code
    .trim()
    .replace(/^```\s*mermaid[^\n]*\n?/i, '')
    .replace(/\n?```\s*$/i, '')
    .trim();
}

async function renderMermaid(id: string, code: string, theme: AppTheme) {
  const normalizedCode = normalizeMermaidCode(code);
  if (!normalizedCode) throw new Error('Mermaid diagram is empty');
  const mermaid = await loadMermaid();
  if (initializedTheme !== theme) {
    mermaid.initialize(createMermaidConfig(theme));
    initializedTheme = theme;
  }
  return mermaid.render(id, normalizedCode);
}

export function useMermaid(code: string, index: number, theme: AppTheme) {
  const [svg, setSvg] = useState('');
  const [err, setErr] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelledRef = useRef(false);
  const idRef = useRef(`mermaid-${index}-${Date.now()}`);

  useEffect(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    cancelledRef.current = false;
    timerRef.current = setTimeout(() => {
      idRef.current = `mermaid-${index}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
      void renderMermaid(idRef.current, code, theme)
        .then(r => {
          if (!cancelledRef.current) {
            setSvg(r.svg);
            setErr('');
          }
        })
        .catch(e => {
          if (!cancelledRef.current) setErr(String(e));
        });
    }, 150);
    return () => {
      cancelledRef.current = true;
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [code, index, theme]);

  return { svg, err };
}
