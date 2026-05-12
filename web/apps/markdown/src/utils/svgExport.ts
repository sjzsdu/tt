interface ExportGeometry {
  viewBox: string;
  width: number;
  height: number;
}

function parsePositiveNumber(value: string | null): number | null {
  if (!value) return null;
  const match = value.trim().match(/^([0-9]*\.?[0-9]+)/);
  if (!match) return null;
  const number = Number(match[1]);
  return Number.isFinite(number) && number > 0 ? number : null;
}

function getExportGeometry(svg: SVGSVGElement): ExportGeometry {
  const dataWidth = parsePositiveNumber(svg.dataset.exportWidth ?? null);
  const dataHeight = parsePositiveNumber(svg.dataset.exportHeight ?? null);
  const originalViewBox = svg.dataset.originalViewBox || svg.getAttribute('viewBox') || '';
  const parts = originalViewBox.trim().split(/[\s,]+/).map(Number);

  if (parts.length === 4 && parts.every(Number.isFinite) && parts[2] > 0 && parts[3] > 0) {
    return {
      viewBox: parts.join(' '),
      width: Math.ceil(dataWidth || parts[2]),
      height: Math.ceil(dataHeight || parts[3]),
    };
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

  return {
    viewBox: `0 0 ${width} ${height}`,
    width: Math.max(1, width),
    height: Math.max(1, height),
  };
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

function cloneForExport(svg: SVGSVGElement): { clone: SVGSVGElement; geometry: ExportGeometry } {
  const geometry = getExportGeometry(svg);
  const clone = svg.cloneNode(true) as SVGSVGElement;
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  clone.setAttribute('viewBox', geometry.viewBox);
  clone.setAttribute('width', String(geometry.width));
  clone.setAttribute('height', String(geometry.height));
  clone.style.width = `${geometry.width}px`;
  clone.style.height = `${geometry.height}px`;
  clone.style.maxWidth = 'none';
  clone.style.transform = '';
  clone.style.transformOrigin = '';
  clone.style.cursor = '';
  clone.removeAttribute('data-original-view-box');
  clone.removeAttribute('data-export-width');
  clone.removeAttribute('data-export-height');
  inlineComputedSvgStyles(svg, clone);
  inlineComputedTextStyles(svg, clone);
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

function canvasToPngBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise<Blob>((resolve, reject) =>
    canvas.toBlob(b =>
      b ? resolve(b) : reject(new Error('PNG export failed')),
      'image/png'
    )
  );
}

function createExportCanvas(geometry: ExportGeometry, scale = 2) {
  const canvas = document.createElement('canvas');
  canvas.width = geometry.width * scale;
  canvas.height = geometry.height * scale;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('Canvas 2D context is unavailable');
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  return { canvas, ctx, scale };
}

async function renderPngWithCanvg(svgText: string, geometry: ExportGeometry): Promise<Blob> {
  const { Canvg } = await import('canvg');
  const { canvas, ctx, scale } = createExportCanvas(geometry);
  ctx.scale(scale, scale);
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, geometry.width, geometry.height);
  const renderer = await Canvg.from(ctx, svgText, {
    DOMParser,
    ignoreMouse: true,
    ignoreAnimation: true,
    fontBold: 'Arial',
    fontNormal: 'Arial',
    fontMono: 'Courier New',
  });
  await renderer.ready();
  await renderer.render();
  return canvasToPngBlob(canvas);
}

export async function svgToPngBlob(svg: SVGSVGElement): Promise<Blob> {
  const { clone, geometry } = cloneForExport(svg);
  const style = document.createElementNS('http://www.w3.org/2000/svg', 'style');
  style.textContent = [
    'svg { background: #ffffff; }',
    'text, tspan { fill: #111827 !important; font-family: Arial, "Liberation Sans", sans-serif !important; }',
  ].join('\n');
  clone.insertBefore(style, clone.firstChild);

  const svgText = serializedSvg(clone);
  return renderPngWithCanvg(svgText, geometry);
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
