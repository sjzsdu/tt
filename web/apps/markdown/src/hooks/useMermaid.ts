import { useState, useEffect, useRef } from 'react';
import mermaid from 'mermaid';

mermaid.initialize({
  startOnLoad: false,
  theme: 'default',
  securityLevel: 'loose',
  logLevel: 'error',
  flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' },
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
    .label {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      color: #0f172a;
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
