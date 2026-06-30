import { useMermaid } from '../hooks/useMermaid';
import type { AppTheme } from '../utils/mermaidConfig';
import { DiagramViewport } from './DiagramViewport';

export function MermaidBlock({ code, index, theme = 'dark' }: { code: string; index: number; theme?: AppTheme }) {
  const { svg, err } = useMermaid(code, index, theme);

  if (err) {
    return (
      <div className="diagram-error">
        <div className="diagram-error-title">Mermaid render failed</div>
        <pre>{err}</pre>
      </div>
    );
  }

  if (svg) {
    return <DiagramViewport svg={svg} label="Mermaid diagram" />;
  }

  return (
    <div className="diagram-loading">
      <div className="diagram-loading-spinner" />
      <span>Rendering diagram...</span>
    </div>
  );
}
