import { useCallback, useEffect, useRef, useState } from 'react';
import type { PointerEvent, WheelEvent } from 'react';

type DiagramViewportProps = {
  svg: string;
  label: string;
};

const MIN_SCALE = 0.25;
const MAX_SCALE = 4;
const SCALE_STEP = 0.18;

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

export function DiagramViewport({ svg, label }: DiagramViewportProps) {
  const [scale, setScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const dragRef = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null);

  const reset = useCallback(() => {
    setScale(1);
    setPan({ x: 0, y: 0 });
  }, []);

  useEffect(() => {
    reset();
  }, [svg, reset]);

  const zoomBy = useCallback((delta: number) => {
    setScale(current => clamp(Number((current + delta).toFixed(3)), MIN_SCALE, MAX_SCALE));
  }, []);

  const onWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    zoomBy(event.deltaY < 0 ? SCALE_STEP : -SCALE_STEP);
  }, [zoomBy]);

  const onPointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { x: event.clientX, y: event.clientY, panX: pan.x, panY: pan.y };
    event.preventDefault();
    event.stopPropagation();
  }, [pan]);

  const onPointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag) return;
    setPan({ x: drag.panX + event.clientX - drag.x, y: drag.panY + event.clientY - drag.y });
    event.preventDefault();
    event.stopPropagation();
  }, []);

  const onPointerUp = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (dragRef.current) {
      dragRef.current = null;
      event.preventDefault();
      event.stopPropagation();
    }
  }, []);

  return (
    <div className="diagram-viewport" aria-label={label} onWheel={onWheel} onDoubleClick={reset}>
      <div className="diagram-toolbar" onPointerDown={e => e.stopPropagation()} onClick={e => e.stopPropagation()}>
        <button type="button" onClick={() => zoomBy(-SCALE_STEP)} title="Zoom out">−</button>
        <span>{Math.round(scale * 100)}%</span>
        <button type="button" onClick={() => zoomBy(SCALE_STEP)} title="Zoom in">＋</button>
        <button type="button" onClick={reset} title="Reset pan and zoom">Reset</button>
      </div>
      <div
        className={`diagram-panzoom ${dragRef.current ? 'dragging' : ''}`}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <div
          className="diagram-svg"
          style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${scale})` }}
          dangerouslySetInnerHTML={{ __html: svg }}
        />
      </div>
    </div>
  );
}
