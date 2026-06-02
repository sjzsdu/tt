import { useState, useEffect, useRef } from 'react';

import { createMermaidConfig, type AppTheme } from '../utils/mermaidConfig';

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
      void import('mermaid')
        .then(module => {
          const mermaid = module.default;
          mermaid.initialize(createMermaidConfig(theme));
          return mermaid.render(idRef.current, code);
        })
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
