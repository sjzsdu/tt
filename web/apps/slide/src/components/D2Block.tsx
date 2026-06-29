import { useD2 } from '../hooks/useD2';

export function D2Block({ code, index, theme = 'dark' }: { code: string; index: number; theme?: 'light' | 'dark' }) {
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
    return <div className="diagram-svg" dangerouslySetInnerHTML={{ __html: svg }} />;
  }

  return (
    <div className="diagram-loading">
      <div className="diagram-loading-spinner" />
      <span>Rendering D2 diagram...</span>
    </div>
  );
}
