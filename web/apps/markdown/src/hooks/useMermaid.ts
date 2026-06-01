import { useState, useEffect, useRef } from 'react';
import mermaid from 'mermaid';

type AppTheme = 'light' | 'dark';

function createMermaidConfig(theme: AppTheme) {
  const dark = theme === 'dark';
  return {
    startOnLoad: false,
    theme: 'base' as const,
    securityLevel: 'loose' as const,
    logLevel: 'error' as const,
    flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' as const },
    themeVariables: dark ? {
      background: '#0f172a',
      primaryColor: '#111827',
      primaryBorderColor: '#38bdf8',
      primaryTextColor: '#e5eefc',
      secondaryColor: '#172033',
      secondaryBorderColor: '#64748b',
      secondaryTextColor: '#e5eefc',
      tertiaryColor: '#1e293b',
      tertiaryBorderColor: '#334155',
      tertiaryTextColor: '#e5eefc',
      lineColor: '#94a3b8',
      textColor: '#e5eefc',
      mainBkg: '#111827',
      nodeBorder: '#38bdf8',
      clusterBkg: '#111827',
      clusterBorder: '#334155',
    } : {
      background: '#ffffff',
      primaryColor: '#ffffff',
      primaryBorderColor: '#cbd5e1',
      primaryTextColor: '#0f172a',
      secondaryColor: '#f8fafc',
      secondaryBorderColor: '#cbd5e1',
      secondaryTextColor: '#0f172a',
      tertiaryColor: '#eef4ff',
      tertiaryBorderColor: '#cbd5e1',
      tertiaryTextColor: '#0f172a',
      lineColor: '#94a3b8',
      textColor: '#0f172a',
      mainBkg: '#ffffff',
      nodeBorder: '#cbd5e1',
      clusterBkg: '#f8fafc',
      clusterBorder: '#e2e8f0',
    },
    themeCSS: dark ? `
      .node rect, .node circle, .node ellipse, .node polygon, .node path { fill: #111827; stroke: #38bdf8; stroke-width: 1.5px; rx: 6px; ry: 6px; }
      .edgePath .path { stroke: #94a3b8; stroke-width: 1.5px; }
      .edgeLabel, .edgeLabel rect { background-color: #0f172a; fill: #0f172a; color: #cbd5e1; font-size: 12px; }
      .cluster rect { fill: #111827; stroke: #334155; stroke-width: 1px; }
      .label, .label text, .label span, .nodeLabel, .nodeLabel p, .nodeLabel span, .edgeLabel, .edgeLabel text, .sectionTitle, .actor, .messageText, .loopText, .noteText, .taskText, .taskTextOutsideRight, .taskTextOutsideLeft, .mindmap-node .label, .mindmap-node .label text, .mindmap-node .label span, svg text, svg tspan { fill: #e5eefc !important; color: #e5eefc !important; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
    ` : `
      .node rect, .node circle, .node ellipse, .node polygon, .node path { fill: #fff; stroke: #cbd5e1; stroke-width: 1.5px; rx: 6px; ry: 6px; }
      .edgePath .path { stroke: #94a3b8; stroke-width: 1.5px; }
      .edgeLabel { background-color: #fff; color: #475569; font-size: 12px; }
      .cluster rect { fill: #f8fafc; stroke: #e2e8f0; stroke-width: 1px; }
      .label, .label text, .label span, .nodeLabel, .nodeLabel p, .nodeLabel span, .edgeLabel, .edgeLabel text, .sectionTitle, .actor, .messageText, .loopText, .noteText, .taskText, .taskTextOutsideRight, .taskTextOutsideLeft, .mindmap-node .label, .mindmap-node .label text, .mindmap-node .label span, svg text, svg tspan { fill: #0f172a !important; color: #0f172a !important; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
    `,
  };
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
      mermaid.initialize(createMermaidConfig(theme));
      mermaid
        .render(idRef.current, code)
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
