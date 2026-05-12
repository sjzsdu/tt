import { initWasm, Resvg } from '@resvg/resvg-wasm';
import notoSansUrl from '@fontsource/noto-sans/files/noto-sans-latin-400-normal.woff?url';
import resvgWasmUrl from '@resvg/resvg-wasm/index_bg.wasm?url';

interface ExportGeometry {
  viewBox: string;
  width: number;
  height: number;
}

let resvgInitPromise: Promise<void> | null = null;
let fontBuffersPromise: Promise<Uint8Array[]> | null = null;

function ensureResvgReady() {
  resvgInitPromise ||= initWasm(resvgWasmUrl);
  return resvgInitPromise;
}

function ensureFontBuffers() {
  fontBuffersPromise ||= fetch(notoSansUrl)
    .then(response => {
      if (!response.ok) throw new Error(`Font load failed: ${response.status}`);
      return response.arrayBuffer();
    })
    .then(buffer => [new Uint8Array(buffer)]);
  return fontBuffersPromise;
}

function parsePositiveNumber(value: string | null): number | null {
  if (!value) return null;
  if (value.trim().endsWith('%')) return null;
  const match = value.trim().match(/^([0-9]*\.?[0-9]+)/);
  if (!match) return null;
  const number = Number(match[1]);
  return Number.isFinite(number) && number > 0 ? number : null;
}

function paddedGeometry(geometry: ExportGeometry, padding: number): ExportGeometry {
  if (padding <= 0) return geometry;
  const parts = geometry.viewBox.trim().split(/[\s,]+/).map(Number);
  if (parts.length !== 4 || !parts.every(Number.isFinite)) return geometry;
  return {
    viewBox: `${parts[0] - padding} ${parts[1] - padding} ${parts[2] + padding * 2} ${parts[3] + padding * 2}`,
    width: geometry.width + padding * 2,
    height: geometry.height + padding * 2,
  };
}

function getExportGeometry(svg: SVGSVGElement, padding = 0): ExportGeometry {
  const dataWidth = parsePositiveNumber(svg.dataset.exportWidth ?? null);
  const dataHeight = parsePositiveNumber(svg.dataset.exportHeight ?? null);
  const originalViewBox = svg.dataset.originalViewBox || svg.getAttribute('viewBox') || '';
  const parts = originalViewBox.trim().split(/[\s,]+/).map(Number);

  if (parts.length === 4 && parts.every(Number.isFinite) && parts[2] > 0 && parts[3] > 0) {
    return paddedGeometry({
      viewBox: parts.join(' '),
      width: Math.ceil(dataWidth || parts[2]),
      height: Math.ceil(dataHeight || parts[3]),
    }, padding);
  }

  const rect = svg.getBoundingClientRect();
  const width = Math.ceil(
    dataWidth ||
    parsePositiveNumber(svg.getAttribute('width')) ||
    rect.width ||
    600
  );
  const height = Math.ceil(
    dataHeight ||
    parsePositiveNumber(svg.getAttribute('height')) ||
    rect.height ||
    300
  );

  return paddedGeometry({
    viewBox: `0 0 ${width} ${height}`,
    width: Math.max(1, width),
    height: Math.max(1, height),
  }, padding);
}

function getMarkupGeometry(svg: SVGSVGElement, padding = 0): ExportGeometry {
  const viewBox = svg.getAttribute('viewBox') || '';
  const parts = viewBox.trim().split(/[\s,]+/).map(Number);
  if (parts.length === 4 && parts.every(Number.isFinite) && parts[2] > 0 && parts[3] > 0) {
    return paddedGeometry({
      viewBox: parts.join(' '),
      width: Math.ceil(parsePositiveNumber(svg.getAttribute('width')) || parts[2]),
      height: Math.ceil(parsePositiveNumber(svg.getAttribute('height')) || parts[3]),
    }, padding);
  }

  const width = Math.ceil(parsePositiveNumber(svg.getAttribute('width')) || 600);
  const height = Math.ceil(parsePositiveNumber(svg.getAttribute('height')) || 300);
  return paddedGeometry({ viewBox: `0 0 ${width} ${height}`, width, height }, padding);
}

function inlineComputedTextStyles(source: SVGSVGElement, clone: SVGSVGElement) {
  const sourceText = source.querySelectorAll<SVGTextElement | SVGTSpanElement>('text,tspan');
  const cloneText = clone.querySelectorAll<SVGTextElement | SVGTSpanElement>('text,tspan');
  sourceText.forEach((node, index) => {
    const target = cloneText[index];
    if (!target) return;
    const style = getComputedStyle(node);
    target.style.fill = style.fill && style.fill !== 'none' ? style.fill : '#111827';
    target.style.fontFamily = style.fontFamily || 'Arial, sans-serif';
    target.style.fontSize = style.fontSize || target.getAttribute('font-size') || '16px';
    target.style.fontWeight = style.fontWeight || target.getAttribute('font-weight') || '400';
    target.style.fontStyle = style.fontStyle || 'normal';
    target.style.opacity = style.opacity || '1';
    target.setAttribute('fill', target.style.fill || '#111827');
    target.setAttribute('font-family', target.style.fontFamily || 'Arial, sans-serif');
    target.setAttribute('font-size', target.style.fontSize || '16px');
    target.setAttribute('font-weight', target.style.fontWeight || '400');
  });
}

function inlineComputedSvgStyles(source: SVGSVGElement, clone: SVGSVGElement) {
  const sourceNodes = [source, ...Array.from(source.querySelectorAll<SVGElement>('*'))];
  const cloneNodes = [clone, ...Array.from(clone.querySelectorAll<SVGElement>('*'))];
  sourceNodes.forEach((node, index) => {
    const target = cloneNodes[index];
    if (!target) return;
    const style = getComputedStyle(node);
    target.style.fill = style.fill || target.style.fill;
    target.style.stroke = style.stroke || target.style.stroke;
    target.style.strokeWidth = style.strokeWidth || target.style.strokeWidth;
    target.style.strokeLinecap = style.strokeLinecap || target.style.strokeLinecap;
    target.style.strokeLinejoin = style.strokeLinejoin || target.style.strokeLinejoin;
    target.style.opacity = style.opacity || target.style.opacity;
    target.style.display = style.display || target.style.display;
    target.style.visibility = style.visibility || target.style.visibility;
  });
}

function replaceForeignObjectsWithText(source: SVGSVGElement, clone: SVGSVGElement) {
  const sourceObjects = source.querySelectorAll<SVGForeignObjectElement>('foreignObject');
  const cloneObjects = clone.querySelectorAll<SVGForeignObjectElement>('foreignObject');
  sourceObjects.forEach((sourceObject, index) => {
    const cloneObject = cloneObjects[index];
    if (!cloneObject) return;

    const text = (sourceObject.textContent || '').replace(/\s+/g, ' ').trim();
    if (!text) {
      cloneObject.remove();
      return;
    }

    const x = parsePositiveNumber(sourceObject.getAttribute('x')) || 0;
    const y = parsePositiveNumber(sourceObject.getAttribute('y')) || 0;
    const width = parsePositiveNumber(sourceObject.getAttribute('width')) || 1;
    const height = parsePositiveNumber(sourceObject.getAttribute('height')) || 16;
    const style = getComputedStyle(sourceObject);

    const textEl = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    textEl.textContent = text;
    textEl.setAttribute('x', String(x + width / 2));
    textEl.setAttribute('y', String(y + height / 2));
    textEl.setAttribute('text-anchor', 'middle');
    textEl.setAttribute('dominant-baseline', 'middle');
    textEl.setAttribute('fill', style.color && style.color !== 'rgba(0, 0, 0, 0)' ? style.color : '#111827');
    textEl.setAttribute('font-family', style.fontFamily || 'Arial, sans-serif');
    textEl.setAttribute('font-size', style.fontSize || '16px');
    textEl.setAttribute('font-weight', style.fontWeight || '400');
    cloneObject.replaceWith(textEl);
  });
}

function normalizeSvgForExport(svg: SVGSVGElement, geometry: ExportGeometry) {
  svg.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  svg.setAttribute('viewBox', geometry.viewBox);
  svg.setAttribute('width', String(geometry.width));
  svg.setAttribute('height', String(geometry.height));
  svg.style.width = `${geometry.width}px`;
  svg.style.height = `${geometry.height}px`;
  svg.style.maxWidth = 'none';
  svg.style.transform = '';
  svg.style.transformOrigin = '';
  svg.style.cursor = '';
  svg.removeAttribute('data-original-view-box');
  svg.removeAttribute('data-export-width');
  svg.removeAttribute('data-export-height');
}

function cloneForExport(svg: SVGSVGElement, padding = 0): { clone: SVGSVGElement; geometry: ExportGeometry } {
  const geometry = getExportGeometry(svg, padding);
  const clone = svg.cloneNode(true) as SVGSVGElement;
  normalizeSvgForExport(clone, geometry);
  inlineComputedSvgStyles(svg, clone);
  inlineComputedTextStyles(svg, clone);
  replaceForeignObjectsWithText(svg, clone);
  return { clone, geometry };
}

function parseSvgMarkup(svgMarkup: string): SVGSVGElement {
  const doc = new DOMParser().parseFromString(svgMarkup, 'image/svg+xml');
  const parseError = doc.querySelector('parsererror');
  if (parseError) throw new Error(`SVG parse failed: ${parseError.textContent || 'unknown parser error'}`);
  const svg = doc.querySelector('svg');
  if (!svg) throw new Error('SVG parse failed: missing <svg> root');
  return svg as unknown as SVGSVGElement;
}

function prepareMarkupSvgForPng(svgMarkup: string): { clone: SVGSVGElement; geometry: ExportGeometry } {
  const clone = parseSvgMarkup(svgMarkup);
  const geometry = getMarkupGeometry(clone, 96);
  normalizeSvgForExport(clone, geometry);
  replaceForeignObjectsWithText(clone, clone);
  return { clone, geometry };
}

function serializedSvg(svg: SVGSVGElement): string {
  const text = new XMLSerializer().serializeToString(svg);
  return text.startsWith('<?xml') ? text : `<?xml version="1.0" encoding="UTF-8"?>\n${text}`;
}

export function svgToBlob(svg: SVGSVGElement): Blob {
  const { clone } = cloneForExport(svg);
  return new Blob([serializedSvg(clone)], {
    type: 'image/svg+xml;charset=utf-8',
  });
}

function addPngExportStyle(svg: SVGSVGElement) {
  const style = document.createElementNS('http://www.w3.org/2000/svg', 'style');
  style.textContent = [
    'svg { background: #ffffff; }',
    'text, tspan { fill: #111827 !important; font-family: "Noto Sans", Arial, "Liberation Sans", sans-serif !important; }',
  ].join('\n');
  svg.insertBefore(style, svg.firstChild);
}

function canvasToPngBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise<Blob>((resolve, reject) =>
    canvas.toBlob(blob =>
      blob ? resolve(blob) : reject(new Error('PNG export failed')),
      'image/png'
    )
  );
}

function cropRenderedImageToContent(image: ReturnType<ReturnType<typeof Resvg.prototype.render>>, padding = 24): Promise<Blob> {
  const { width, height, pixels } = image;
  let minX = width;
  let minY = height;
  let maxX = -1;
  let maxY = -1;

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const i = (y * width + x) * 4;
      const r = pixels[i];
      const g = pixels[i + 1];
      const b = pixels[i + 2];
      const a = pixels[i + 3];
      if (a > 8 && (r < 248 || g < 248 || b < 248)) {
        minX = Math.min(minX, x);
        minY = Math.min(minY, y);
        maxX = Math.max(maxX, x);
        maxY = Math.max(maxY, y);
      }
    }
  }

  if (maxX < minX || maxY < minY) {
    return new Blob([image.asPng()], { type: 'image/png' }) as unknown as Promise<Blob>;
  }

  minX = Math.max(0, minX - padding);
  minY = Math.max(0, minY - padding);
  maxX = Math.min(width - 1, maxX + padding);
  maxY = Math.min(height - 1, maxY + padding);

  const cropWidth = maxX - minX + 1;
  const cropHeight = maxY - minY + 1;
  const source = new ImageData(new Uint8ClampedArray(pixels), width, height);
  const canvas = document.createElement('canvas');
  canvas.width = cropWidth;
  canvas.height = cropHeight;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('Canvas 2D context is unavailable');
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, cropWidth, cropHeight);
  ctx.putImageData(source, -minX, -minY);
  return canvasToPngBlob(canvas);
}

async function renderPngWithResvg(svgText: string, geometry: ExportGeometry): Promise<Blob> {
  await ensureResvgReady();
  const fontBuffers = await ensureFontBuffers();
  const targetWidth = Math.max(2400, Math.ceil(geometry.width * 3));
  const renderer = new Resvg(svgText, {
    background: '#ffffff',
    fitTo: { mode: 'width', value: targetWidth },
    textRendering: 1,
    shapeRendering: 2,
    imageRendering: 0,
    font: {
      fontBuffers,
      defaultFontFamily: 'Noto Sans',
      sansSerifFamily: 'Noto Sans',
      serifFamily: 'Noto Sans',
      monospaceFamily: 'Courier New',
    },
  });
  try {
    const image = renderer.render();
    try {
      return await cropRenderedImageToContent(image);
    } finally {
      image.free();
    }
  } finally {
    renderer.free();
  }
}

export async function svgToPngBlob(svg: SVGSVGElement): Promise<Blob> {
  await ensureResvgReady();
  const { clone, geometry } = cloneForExport(svg, 96);
  addPngExportStyle(clone);
  return renderPngWithResvg(serializedSvg(clone), geometry);
}

export async function svgMarkupToPngBlob(svgMarkup: string): Promise<Blob> {
  await ensureResvgReady();
  const { clone, geometry } = prepareMarkupSvgForPng(svgMarkup);
  addPngExportStyle(clone);
  return renderPngWithResvg(serializedSvg(clone), geometry);
}

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
