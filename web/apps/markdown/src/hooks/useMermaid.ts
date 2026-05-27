import { useState, useEffect, useRef } from 'react';
import mermaid from 'mermaid';

mermaid.initialize({
  startOnLoad: false,
  theme: 'base',
  securityLevel: 'loose',
  logLevel: 'error',
  flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' },
  themeVariables: {
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
  themeCSS: `
    .node rect, .node circle, .node ellipse, .node polygon, .node path {
      fill: #fff;
      stroke: #cbd5e1;
      stroke-width: 1.5px;
      rx: 6px;
      ry: 6px;
    }
    .edgePath .path {
      stroke: #94a3b8;
      stroke-width: 1.5px;
    }
    .edgeLabel {
      background-color: #fff;
      color: #475569;
      font-size: 12px;
    }
    .cluster rect {
      fill: #f8fafc;
      stroke: #e2e8f0;
      stroke-width: 1px;
    }
    .label,
    .label text,
    .label span,
    .nodeLabel,
    .nodeLabel p,
    .nodeLabel span,
    .edgeLabel,
    .edgeLabel text,
    .sectionTitle,
    .actor,
    .messageText,
    .loopText,
    .noteText,
    .taskText,
    .taskTextOutsideRight,
    .taskTextOutsideLeft,
    .mindmap-node .label,
    .mindmap-node .label text,
    .mindmap-node .label span,
    svg text,
    svg tspan {
      fill: #0f172a !important;
      color: #0f172a !important;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      font-size: 13px;
    }
  `,
});

export function useMermaid(code: string, index: number) {
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
  }, [code, index]);

  return { svg, err };
}
