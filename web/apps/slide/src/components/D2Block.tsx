import { useD2 } from '../hooks/useD2';
import { DiagramViewport } from './DiagramViewport';

export function D2Block({ code, index, theme = 'dark', interactive = true }: { code: string; index: number; theme?: 'light' | 'dark'; interactive?: boolean }) {
  const { svg, err } = useD2(code, index, theme);

  if (err) {
    return (
      <div className="diagram-error">
        <div className="diagram-error-title">D2 render failed</div>
        <pre>{err}</pre>
      </div>
    );
  }

  if (svg) {
    return <DiagramViewport svg={svg} label="D2 diagram" interactive={interactive} />;
  }

  return (
    <div className="diagram-loading">
      <div className="diagram-loading-spinner" />
      <span>Rendering D2 diagram...</span>
    </div>
  );
}
