import { useState, useEffect, useRef } from 'react';
import mermaid from 'mermaid';

mermaid.initialize({
  startOnLoad: false,
  theme: 'default',
  securityLevel: 'loose',
  logLevel: 'error',
  flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' },
});

export function useMermaid(code: string, index: number) {
  const [svg, setSvg] = useState('');
  const [err, setErr] = useState('');
  const idRef = useRef(`mermaid-${index}-${Date.now()}`);

  useEffect(() => {
    let cancelled = false;
    idRef.current = `mermaid-${index}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    mermaid
      .render(idRef.current, code)
      .then(r => {
        if (!cancelled) {
          setSvg(r.svg);
          setErr('');
        }
      })
      .catch(e => {
        if (!cancelled) setErr(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [code, index]);

  return { svg, err };
}
