export interface SvgSize {
  minX: number;
  minY: number;
  width: number;
  height: number;
}

const SVG_PADDING = 12;

export function readSvgSize(svg: SVGSVGElement): SvgSize {
  const viewBox = svg.viewBox?.baseVal;
  if (viewBox && viewBox.width > 0 && viewBox.height > 0) {
    return { minX: viewBox.x, minY: viewBox.y, width: Math.ceil(viewBox.width), height: Math.ceil(viewBox.height) };
  }

  const rect = svg.getBoundingClientRect();
  if (rect.width > 0 && rect.height > 0) {
    return {
      minX: viewBox?.x ?? 0,
      minY: viewBox?.y ?? 0,
      width: Math.max(1, Math.ceil(rect.width)),
      height: Math.max(1, Math.ceil(rect.height)),
    };
  }

  const paintedBox = readPaintedBox(svg);
  if (paintedBox) return paddedSize(paintedBox);

  const width = Number(svg.getAttribute('width')) || rect.width || 600;
  const height = Number(svg.getAttribute('height')) || rect.height || 300;
  return { minX: 0, minY: 0, width: Math.max(1, Math.ceil(width)), height: Math.max(1, Math.ceil(height)) };
}

export function prepareSvgForViewport(svg: SVGSVGElement, size: SvgSize) {
  const originalViewBox = svg.getAttribute('viewBox') || `${size.minX} ${size.minY} ${size.width} ${size.height}`;
  svg.setAttribute('viewBox', originalViewBox);
  svg.setAttribute('width', String(size.width));
  svg.setAttribute('height', String(size.height));
  svg.dataset.originalViewBox = originalViewBox;
  svg.dataset.exportWidth = String(size.width);
  svg.dataset.exportHeight = String(size.height);
  svg.style.width = `${size.width}px`;
  svg.style.height = `${size.height}px`;
  svg.style.maxWidth = 'none';
  svg.style.display = 'block';
}

export function prepareSvgMarkupForViewport(markup: string): { svg: string; size: SvgSize } | null {
  if (!markup.trim() || typeof DOMParser === 'undefined' || typeof XMLSerializer === 'undefined') return null;

  const doc = new DOMParser().parseFromString(markup, 'image/svg+xml');
  if (doc.querySelector('parsererror')) return null;

  const svg = doc.querySelector<SVGSVGElement>('svg');
  if (!svg) return null;

  const size = readSvgSize(svg);
  prepareSvgForViewport(svg, size);
  return { svg: new XMLSerializer().serializeToString(svg), size };
}

function readPaintedBox(svg: SVGSVGElement): SvgSize | null {
  try {
    const box = svg.getBBox();
    if (box.width > 0 && box.height > 0) return { minX: box.x, minY: box.y, width: box.width, height: box.height };

    const contentEl = svg.querySelector<SVGGraphicsElement>('g');
    return contentEl ? transformedBox(contentEl) : null;
  } catch {
    // getBBox may fail for detached or not-yet-painted SVGs. Fall back in caller.
    return null;
  }
}

function transformedBox(el: SVGGraphicsElement): SvgSize | null {
  const box = el.getBBox();
  const matrix = el.getCTM();
  if (!matrix || box.width <= 0 || box.height <= 0) return null;

  const points = [
    new DOMPoint(box.x, box.y),
    new DOMPoint(box.x + box.width, box.y),
    new DOMPoint(box.x, box.y + box.height),
    new DOMPoint(box.x + box.width, box.y + box.height),
  ].map(point => point.matrixTransform(matrix));

  const xs = points.map(point => point.x);
  const ys = points.map(point => point.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  if (maxX <= minX || maxY <= minY) return null;
  return { minX, minY, width: maxX - minX, height: maxY - minY };
}

function paddedSize(size: SvgSize): SvgSize {
  return {
    minX: Math.floor(size.minX - SVG_PADDING),
    minY: Math.floor(size.minY - SVG_PADDING),
    width: Math.ceil(size.width + SVG_PADDING * 2),
    height: Math.ceil(size.height + SVG_PADDING * 2),
  };
}
