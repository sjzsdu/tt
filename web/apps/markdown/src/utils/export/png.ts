import { initWasm, Resvg } from '@resvg/resvg-wasm';
import notoSansUrl from '@fontsource/noto-sans/files/noto-sans-latin-400-normal.woff?url';
import resvgWasmUrl from '@resvg/resvg-wasm/index_bg.wasm?url';
import type { ExportGeometry } from './geometry';
import { canvasToPngBlob, renderPngWithBrowser } from './browser';
import { cloneForExport, forceExportTextStyles, prepareMarkupSvgForPng, serializedSvg } from './svg';

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

function addPngExportStyle(svg: SVGSVGElement) {
  const style = document.createElementNS('http://www.w3.org/2000/svg', 'style');
  style.textContent = [
    'svg { background: #ffffff; }',
    'text, tspan { fill: #111827 !important; font-family: "Noto Sans", Arial, "Liberation Sans", sans-serif !important; }',
  ].join('\n');
  svg.insertBefore(style, svg.firstChild);
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
      const [r, g, b, a] = [pixels[i], pixels[i + 1], pixels[i + 2], pixels[i + 3]];
      if (a > 8 && (r < 248 || g < 248 || b < 248)) {
        minX = Math.min(minX, x);
        minY = Math.min(minY, y);
        maxX = Math.max(maxX, x);
        maxY = Math.max(maxY, y);
      }
    }
  }

  if (maxX < minX || maxY < minY) return new Blob([image.asPng()], { type: 'image/png' }) as unknown as Promise<Blob>;
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
  forceExportTextStyles(clone);
  addPngExportStyle(clone);
  const svgText = serializedSvg(clone);
  try {
    return await renderPngWithBrowser(svgText, geometry);
  } catch {
    return renderPngWithResvg(svgText, geometry);
  }
}

export async function svgMarkupToPngBlob(svgMarkup: string): Promise<Blob> {
  await ensureResvgReady();
  const { clone, geometry } = prepareMarkupSvgForPng(svgMarkup);
  addPngExportStyle(clone);
  const svgText = serializedSvg(clone);
  try {
    return await renderPngWithBrowser(svgText, geometry);
  } catch {
    return renderPngWithResvg(svgText, geometry);
  }
}
