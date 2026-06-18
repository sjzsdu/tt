export interface SvgSize {
  minX: number;
  minY: number;
  width: number;
  height: number;
}

const SVG_PADDING = 12;

interface PrepareSvgOptions {
  transparentBackground?: boolean;
}

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

export function prepareSvgMarkupForViewport(markup: string, options: PrepareSvgOptions = {}): { svg: string; size: SvgSize } | null {
  if (!markup.trim() || typeof DOMParser === 'undefined' || typeof XMLSerializer === 'undefined') return null;

  const doc = new DOMParser().parseFromString(markup, 'image/svg+xml');
  if (doc.querySelector('parsererror')) return null;

  const svg = doc.querySelector<SVGSVGElement>('svg');
  if (!svg) return null;

  const size = readSvgSize(svg);
  prepareSvgForViewport(svg, size);
  if (options.transparentBackground) removeFullCanvasBackground(svg, size);
  return { svg: new XMLSerializer().serializeToString(svg), size };
}

function removeFullCanvasBackground(svg: SVGSVGElement, size: SvgSize) {
  svg.removeAttribute('background');
  svg.style.backgroundColor = 'transparent';
  svg.style.background = 'transparent';

  const classFills = readClassFills(svg);
  const fullCanvasRects = Array.from(svg.querySelectorAll<SVGRectElement>('rect')).filter(rect => {
    if (!isPlainBackgroundFill(readFill(rect, classFills))) return false;

    const canvas = rectCanvas(rect, size);
    const x = numberAttr(rect, 'x', canvas.minX);
    const y = numberAttr(rect, 'y', canvas.minY);
    const width = lengthAttr(rect, 'width', canvas.width);
    const height = lengthAttr(rect, 'height', canvas.height);

    return (
      nearlyEqual(x, canvas.minX) &&
      nearlyEqual(y, canvas.minY) &&
      width >= canvas.width - 1 &&
      height >= canvas.height - 1
    );
  });

  fullCanvasRects.forEach(rect => rect.remove());

  const style = svg.ownerDocument.createElementNS('http://www.w3.org/2000/svg', 'style');
  style.setAttribute('data-tt-d2-transparent-background', 'true');
  style.textContent = `
    svg.d2-svg > rect:first-child[stroke-width="0"],
    svg.d2-svg > rect[fill="white"],
    svg.d2-svg > rect[fill="#fff"],
    svg.d2-svg > rect[fill="#ffffff"],
    svg.d2-svg > rect.fill-N7[stroke-width="0"] {
      fill: transparent !important;
    }
  `;
  svg.appendChild(style);
}

function readClassFills(svg: SVGSVGElement) {
  const fills = new Map<string, string>();
  svg.querySelectorAll('style').forEach(style => {
    const css = style.textContent || '';
    for (const match of css.matchAll(/\.([_a-zA-Z][\w-]*)\s*\{[^}]*?fill\s*:\s*([^;}]+)/g)) {
      fills.set(match[1], match[2]);
    }
  });
  return fills;
}

function readFill(el: SVGElement, classFills: Map<string, string>) {
  const directFill = el.getAttribute('fill') || el.style.fill || readStyleDeclaration(el.getAttribute('style'), 'fill');
  if (directFill) return directFill;
  for (const className of Array.from(el.classList)) {
    const fill = classFills.get(className);
    if (fill) return fill;
  }
  return null;
}

function rectCanvas(rect: SVGRectElement, fallback: SvgSize): SvgSize {
  const parentSvg = rect.closest('svg') as SVGSVGElement | null;
  const viewBox = parentSvg?.viewBox?.baseVal;
  if (viewBox && viewBox.width > 0 && viewBox.height > 0) {
    return { minX: viewBox.x, minY: viewBox.y, width: viewBox.width, height: viewBox.height };
  }
  return fallback;
}

function isPlainBackgroundFill(fill: string | null) {
  if (!fill) return false;
  const normalized = fill.trim().toLowerCase().replace(/\s+/g, '');
  return normalized === '#fff' || normalized === '#ffffff' || normalized === 'white' || normalized === 'rgb(255,255,255)' || normalized === 'rgba(255,255,255,1)';
}

function readStyleDeclaration(style: string | null, name: string) {
  if (!style) return null;
  const match = style.match(new RegExp(`(?:^|;)\\s*${name}\\s*:\\s*([^;]+)`, 'i'));
  return match?.[1] ?? null;
}

function numberAttr(el: Element, name: string, fallback: number) {
  const value = Number(el.getAttribute(name));
  return Number.isFinite(value) ? value : fallback;
}

function lengthAttr(el: Element, name: string, fallback: number) {
  const raw = el.getAttribute(name)?.trim();
  if (!raw) return fallback;
  if (raw === '100%') return fallback;
  const value = Number(raw.replace(/px$/i, ''));
  return Number.isFinite(value) ? value : fallback;
}

function nearlyEqual(a: number, b: number) {
  return Math.abs(a - b) < 1;
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
