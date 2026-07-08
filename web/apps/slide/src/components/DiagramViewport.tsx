import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import type { PointerEvent, WheelEvent } from 'react';

type DiagramViewportProps = {
  svg: string;
  label: string;
  interactive?: boolean;
};

const MIN_SCALE = 0.25;
const MAX_SCALE = 4;
const SCALE_STEP = 0.18;
const VIEWBOX_PADDING = 24;

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function numericSvgLength(value: string | null) {
  if (!value) return 0;
  if (value.includes('%')) return 0;
  const match = value.match(/([0-9]*\.?[0-9]+)/);
  return match ? Number(match[1]) : 0;
}

function normalizeSvgViewBox(svgElement: SVGSVGElement) {
  let bbox: DOMRect | SVGRect | null = null;
  try {
    bbox = svgElement.getBBox();
  } catch (_) {
    bbox = null;
  }

  if (bbox && bbox.width > 0 && bbox.height > 0) {
    const x = bbox.x - VIEWBOX_PADDING;
    const y = bbox.y - VIEWBOX_PADDING;
    const width = bbox.width + VIEWBOX_PADDING * 2;
    const height = bbox.height + VIEWBOX_PADDING * 2;
    svgElement.setAttribute('viewBox', `${x} ${y} ${width} ${height}`);
    return;
  }

  if (!svgElement.getAttribute('viewBox')) {
    const width = numericSvgLength(svgElement.getAttribute('width'));
    const height = numericSvgLength(svgElement.getAttribute('height'));
    if (width > 0 && height > 0) {
      svgElement.setAttribute('viewBox', `0 0 ${width} ${height}`);
    }
  }
}

export function DiagramViewport({ svg, label, interactive = true }: DiagramViewportProps) {
  const [scale, setScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const svgRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null);

  const reset = useCallback(() => {
    setScale(1);
    setPan({ x: 0, y: 0 });
  }, []);

  useEffect(() => {
    reset();
  }, [svg, reset]);

  useLayoutEffect(() => {
    const svgElement = svgRef.current?.querySelector('svg') as SVGSVGElement | null;
    if (!svgElement) return;

    normalizeSvgViewBox(svgElement);

    svgElement.setAttribute('preserveAspectRatio', 'xMidYMid meet');
    svgElement.removeAttribute('width');
    svgElement.removeAttribute('height');
    Object.assign(svgElement.style, {
      display: 'block',
      width: '100%',
      height: '100%',
      maxWidth: '100%',
      maxHeight: '100%',
      overflow: 'visible',
    });
  }, [svg]);

  const zoomBy = useCallback((delta: number) => {
    setScale(current => clamp(Number((current + delta).toFixed(3)), MIN_SCALE, MAX_SCALE));
  }, []);

  const onWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (!interactive) return;
    event.preventDefault();
    event.stopPropagation();
    zoomBy(event.deltaY < 0 ? SCALE_STEP : -SCALE_STEP);
  }, [interactive, zoomBy]);

  const onPointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (!interactive) return;
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { x: event.clientX, y: event.clientY, panX: pan.x, panY: pan.y };
    event.preventDefault();
    event.stopPropagation();
  }, [interactive, pan]);

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
    <div className="slide-diagram">
      <div className={`diagram-viewport ${interactive ? 'diagram-viewport-interactive' : 'diagram-viewport-static'}`} aria-label={label} onWheel={onWheel} onDoubleClick={interactive ? reset : undefined}>
        {interactive && (
          <div className="diagram-toolbar" onPointerDown={e => e.stopPropagation()} onClick={e => e.stopPropagation()}>
            <button type="button" onClick={() => zoomBy(-SCALE_STEP)} title="Zoom out">−</button>
            <span>{Math.round(scale * 100)}%</span>
            <button type="button" onClick={() => zoomBy(SCALE_STEP)} title="Zoom in">＋</button>
            <button type="button" onClick={reset} title="Reset pan and zoom">Reset</button>
          </div>
        )}
        <div
          className={`diagram-panzoom ${interactive ? 'diagram-panzoom-interactive' : 'diagram-panzoom-static'} ${dragRef.current ? 'dragging' : ''}`}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
        >
          <div
            ref={svgRef}
            className="diagram-svg"
            style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${scale})` }}
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        </div>
      </div>
    </div>
  );
}
