import { useEffect, useMemo, useRef } from 'react';
import { prepareSvgMarkupForViewport } from '../utils/svgViewport';

export function useD2Viewport(svg: string) {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgElRef = useRef<SVGSVGElement | null>(null);
  const prepared = useMemo(() => prepareSvgMarkupForViewport(svg), [svg]);
  const displaySvg = prepared?.svg ?? svg;
  const baseSize = prepared?.size ?? null;

  useEffect(() => {
    if (!displaySvg || !containerRef.current) {
      svgElRef.current = null;
      return;
    }
    svgElRef.current = containerRef.current.querySelector<SVGSVGElement>('svg');
  }, [displaySvg]);

  return { containerRef, svgElRef, displaySvg, baseSize };
}
