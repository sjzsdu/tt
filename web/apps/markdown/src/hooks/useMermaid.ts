import { useState, useEffect, useRef, type RefObject } from 'react';

import { createMermaidConfig, type AppTheme } from '../utils/mermaidConfig';

type MermaidModule = typeof import('mermaid').default;
type MermaidLayoutSnapshot = { width: number; height: number; connected: boolean; display: string };

let mermaidPromise: Promise<MermaidModule> | null = null;
let renderQueue: Promise<unknown> = Promise.resolve();
const MERMAID_RENDER_ATTEMPTS = 4;

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

function layoutSnapshot(hostRef?: RefObject<HTMLElement | null>): MermaidLayoutSnapshot | null {
  const el = hostRef?.current;
  if (!el) return null;
  const rect = el.getBoundingClientRect();
  const style = window.getComputedStyle(el);
  return {
    width: Math.round(rect.width),
    height: Math.round(rect.height),
    connected: el.isConnected,
    display: style.display,
  };
}

function formatMermaidError(error: unknown, id: string, code: string, hostRef?: RefObject<HTMLElement | null>) {
  const normalizedCode = normalizeMermaidCode(code);
  return [
    errorMessage(error),
    '',
    '--- Mermaid debug ---',
    `render_id: ${id}`,
    `layout: ${JSON.stringify(layoutSnapshot(hostRef))}`,
    'normalized_code:',
    normalizedCode,
  ].join('\n');
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
    throw new Error('Mermaid returned its internal syntax-error SVG even though the diagram parsed successfully');
  }
  return result;
}

function wait(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function nextFrame() {
  return new Promise(resolve => requestAnimationFrame(() => resolve(undefined)));
}

async function waitForRenderableElement(hostRef?: RefObject<HTMLElement | null>) {
  if (!hostRef) return;
  for (let i = 0; i < 30; i++) {
    const el = hostRef.current;
    if (el?.isConnected) {
      const rect = el.getBoundingClientRect();
      const visible = rect.width > 0 && rect.height > 0 && window.getComputedStyle(el).display !== 'none';
      if (visible) {
        await nextFrame();
        await nextFrame();
        return;
      }
    }
    await wait(50);
  }
}

async function renderMermaidWithRetry(id: string, code: string, theme: AppTheme) {
  let lastError: unknown;
  for (let attempt = 1; attempt <= MERMAID_RENDER_ATTEMPTS; attempt++) {
    try {
      return await renderMermaid(`${id}-${attempt}`, code, theme);
    } catch (error) {
      lastError = error;
      if (attempt < MERMAID_RENDER_ATTEMPTS) await wait(120 * attempt);
    }
  }
  throw lastError;
}

function enqueueMermaidRender(id: string, code: string, theme: AppTheme) {
  const task = renderQueue.then(() => renderMermaidWithRetry(id, code, theme));
  renderQueue = task.catch(() => undefined);
  return task;
}

export function useMermaid(code: string, index: number, theme: AppTheme, hostRef?: RefObject<HTMLElement | null>) {
  const [svg, setSvg] = useState('');
  const [err, setErr] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const renderVersionRef = useRef(0);
  const idRef = useRef(`mermaid-${index}-${Date.now()}`);

  useEffect(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    const renderVersion = ++renderVersionRef.current;
    setErr('');
    setSvg('');
    timerRef.current = setTimeout(() => {
      idRef.current = `mermaid-${index}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
      void waitForRenderableElement(hostRef)
        .then(() => enqueueMermaidRender(idRef.current, code, theme))
        .then(r => {
          if (renderVersionRef.current === renderVersion) {
            setSvg(r.svg);
            setErr('');
          }
        })
        .catch(e => {
          if (renderVersionRef.current === renderVersion) setErr(formatMermaidError(e, idRef.current, code, hostRef));
        });
    }, 150);
    return () => {
      renderVersionRef.current += 1;
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [code, index, theme]);

  return { svg, err };
}
