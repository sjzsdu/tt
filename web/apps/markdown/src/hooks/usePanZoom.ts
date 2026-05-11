import { useState, useCallback, useEffect, useRef } from 'react';

interface PanZoomState {
  scale: number;
  position: { x: number; y: number };
}

interface UsePanZoomOptions {
  minScale?: number;
  maxScale?: number;
  step?: number;
}

export function usePanZoom(
  svgEl: SVGSVGElement | null,
  baseWidth: number,
  baseHeight: number,
  options: UsePanZoomOptions = {}
) {
  const { minScale = 0.4, maxScale = 4, step = 0.2 } = options;
  const svgRef = useRef(svgEl);
  svgRef.current = svgEl;
  const [state, setState] = useState<PanZoomState>({ scale: 1, position: { x: 0, y: 0 } });

  const apply = useCallback(() => {
    const el = svgRef.current;
    if (!el) return;
    if (state.scale === 1) {
      el.setAttribute('viewBox', `0 0 ${baseWidth} ${baseHeight}`);
      el.style.transform = `translate(${state.position.x}px, ${state.position.y}px)`;
    } else {
      const newW = baseWidth / state.scale;
      const newH = baseHeight / state.scale;
      el.setAttribute(
        'viewBox',
        `${-state.position.x / state.scale} ${-state.position.y / state.scale} ${newW} ${newH}`
      );
      el.style.transform = '';
    }
  }, [baseWidth, baseHeight, state]);

  useEffect(() => {
    apply();
  }, [apply]);

  const zoomIn = () =>
    setState(s => ({ ...s, scale: Math.min(maxScale, s.scale + step) }));
  const zoomOut = () =>
    setState(s => ({ ...s, scale: Math.max(minScale, s.scale - step) }));
  const reset = () => setState({ scale: 1, position: { x: 0, y: 0 } });

  const onWheel = (e: WheelEvent) => {
    e.preventDefault();
    const factor = e.deltaY > 0 ? 0.95 : 1.05;
    setState(s => ({
      ...s,
      scale: Math.min(maxScale, Math.max(minScale, s.scale * factor)),
    }));
  };

  const onPointerDown = (e: PointerEvent) => {
    const el = svgRef.current;
    if (!el) return;
    el.style.cursor = 'grabbing';
    el.setPointerCapture(e.pointerId);
  };

  const onPointerMove = (e: PointerEvent) => {
    if (e.buttons !== 1) return;
    setState(s => ({
      ...s,
      position: { x: s.position.x + e.movementX, y: s.position.y + e.movementY },
    }));
  };

  const onPointerUp = (e: PointerEvent) => {
    const el = svgRef.current;
    if (!el) return;
    el.style.cursor = 'grab';
  };

  const setScale = (scale: number) =>
    setState(s => ({ ...s, scale: Math.min(maxScale, Math.max(minScale, scale)) }));

  return {
    scale: state.scale,
    position: state.position,
    zoomIn,
    zoomOut,
    reset,
    setScale,
    onWheel,
    onPointerDown,
    onPointerMove,
    onPointerUp,
  };
}
