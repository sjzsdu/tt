import { useEffect, useRef, useState } from 'react';

type D2Module = typeof import('@terrastruct/d2');
type D2Instance = InstanceType<D2Module['D2']>;

let d2Promise: Promise<D2Instance> | null = null;
let renderQueue: Promise<unknown> = Promise.resolve();

function loadD2() {
  if (!d2Promise) {
    d2Promise = import('@terrastruct/d2').then(({ D2 }) => new D2());
  }
  return d2Promise;
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

async function renderD2(id: string, code: string, theme: 'light' | 'dark') {
  const source = code.trim();
  if (!source) throw new Error('D2 diagram is empty');
  const d2 = await loadD2();
  const compiled = await d2.compile(source, {
    layout: 'dagre',
    sketch: false,
    themeID: theme === 'dark' ? 200 : 0,
    darkThemeID: 200,
  });
  return d2.render(compiled.diagram, {
    ...compiled.renderOptions,
    noXMLTag: true,
    salt: id,
  });
}

function enqueueD2Render(id: string, code: string, theme: 'light' | 'dark') {
  const task = renderQueue.then(() => renderD2(id, code, theme));
  renderQueue = task.catch(() => undefined);
  return task;
}

export function useD2(code: string, index: number, theme: 'light' | 'dark') {
  const [svg, setSvg] = useState('');
  const [err, setErr] = useState('');
  const idRef = useRef(`d2-${index}-${Math.random().toString(36).slice(2)}`);
  const renderVersionRef = useRef(0);

  useEffect(() => {
    let alive = true;
    const renderVersion = ++renderVersionRef.current;
    setSvg('');
    setErr('');
    loadD2()
      .then(() => enqueueD2Render(idRef.current, code, theme))
      .then(nextSvg => {
        if (alive && renderVersionRef.current === renderVersion) setSvg(nextSvg);
      })
      .catch(e => {
        if (alive && renderVersionRef.current === renderVersion) {
          setErr(['D2 render failed', errorMessage(e), '', code].join('\n'));
        }
      });
    return () => {
      alive = false;
    };
  }, [code, index, theme]);

  return { svg, err };
}
