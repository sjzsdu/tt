import { useEffect, useRef, useState } from 'react';
import { createMermaidConfig, type AppTheme } from '../utils/mermaidConfig';

type MermaidModule = typeof import('mermaid').default;

let mermaidPromise: Promise<MermaidModule> | null = null;
let renderQueue: Promise<unknown> = Promise.resolve();

function loadMermaid() {
  mermaidPromise ||= import('mermaid').then(module => {
    const mermaid = module.default;
    mermaid.startOnLoad = false;
    return mermaid;
  });
  return mermaidPromise;
}

function normalizeMermaidCode(code: string) {
  return code
    .trim()
    .replace(/^```\s*mermaid[^\n]*\n?/i, '')
    .replace(/\n?```\s*$/i, '')
    .trim();
}

function errorMessage(error: unknown) {
  if (error instanceof Error) return error.stack || error.message;
  return String(error);
}

function isMermaidInternalErrorSvg(svg: string) {
  return /Syntax error in text|mermaid version \d/i.test(svg);
}

async function renderMermaid(id: string, code: string, theme: AppTheme) {
  const normalizedCode = normalizeMermaidCode(code);
  if (!normalizedCode) throw new Error('Mermaid diagram is empty');
  const mermaid = await loadMermaid();
  const config = createMermaidConfig(theme);
  mermaid.initialize(config);
  await mermaid.parse(normalizedCode, { suppressErrors: false });
  mermaid.initialize(config);
  const result = await mermaid.render(id, normalizedCode);
  if (isMermaidInternalErrorSvg(result.svg)) {
    throw new Error('Mermaid returned internal syntax-error SVG');
  }
  return result;
}

function wait(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function renderMermaidWithRetry(id: string, code: string, theme: AppTheme) {
  let lastError: unknown;
  for (let attempt = 1; attempt <= 4; attempt++) {
    try {
      return await renderMermaid(`${id}-${attempt}`, code, theme);
    } catch (error) {
      lastError = error;
      if (attempt < 4) await wait(120 * attempt);
    }
  }
  throw lastError;
}

function enqueueMermaidRender(id: string, code: string, theme: AppTheme) {
  const task = renderQueue.then(() => renderMermaidWithRetry(id, code, theme));
  renderQueue = task.catch(() => undefined);
  return task;
}

export function useMermaid(code: string, index: number, theme: AppTheme) {
  const [svg, setSvg] = useState('');
  const [err, setErr] = useState('');
  const renderVersionRef = useRef(0);

  useEffect(() => {
    let alive = true;
    const renderVersion = ++renderVersionRef.current;
    setErr('');
    setSvg('');
    const id = `slide-mermaid-${index}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    enqueueMermaidRender(id, code, theme)
      .then(r => {
        if (alive && renderVersionRef.current === renderVersion) {
          setSvg(r.svg);
        }
      })
      .catch(e => {
        if (alive && renderVersionRef.current === renderVersion) {
          setErr(errorMessage(e));
        }
      });
    return () => {
      alive = false;
    };
  }, [code, index, theme]);

  return { svg, err };
}
