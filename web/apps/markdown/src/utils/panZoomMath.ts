export interface PanZoomState {
  scale: number;
  position: { x: number; y: number };
}

export function clampScale(scale: number, minScale: number, maxScale: number) {
  return Math.min(maxScale, Math.max(minScale, scale));
}

export function zoomAroundPoint(state: PanZoomState, nextScale: number, focus: { x: number; y: number }): PanZoomState {
  const scaleRatio = nextScale / state.scale;
  return {
    scale: nextScale,
    position: {
      x: focus.x - scaleRatio * (focus.x - state.position.x),
      y: focus.y - scaleRatio * (focus.y - state.position.y),
    },
  };
}

export function panBy(state: PanZoomState, delta: { x: number; y: number }): PanZoomState {
  return {
    ...state,
    position: {
      x: state.position.x + delta.x,
      y: state.position.y + delta.y,
    },
  };
}

export function elementCenter(el: HTMLElement) {
  const rect = el.parentElement?.getBoundingClientRect();
  if (!rect) return null;
  return { x: rect.width / 2, y: rect.height / 2 };
}

export function eventPointInElement(event: MouseEvent, el: HTMLElement) {
  const rect = el.getBoundingClientRect();
  return { x: event.clientX - rect.left, y: event.clientY - rect.top };
}
