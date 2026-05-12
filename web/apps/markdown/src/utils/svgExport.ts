import { Canvg } from 'canvg';

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

function cloneForExport(svg: SVGSVGElement): { clone: SVGSVGElement; geometry: ExportGeometry } {
  const geometry = getExportGeometry(svg);
  const clone = svg.cloneNode(true) as SVGSVGElement;
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  clone.setAttribute('viewBox', geometry.viewBox);
  clone.setAttribute('width', String(geometry.width));
  clone.setAttribute('height', String(geometry.height));
  clone.style.transform = '';
  clone.style.transformOrigin = '';
  clone.style.cursor = '';
  clone.removeAttribute('data-original-view-box');
  clone.removeAttribute('data-export-width');
  clone.removeAttribute('data-export-height');
  return { clone, geometry };
}

export function svgToBlob(svg: SVGSVGElement): Blob {
  const { clone } = cloneForExport(svg);
  return new Blob([new XMLSerializer().serializeToString(clone)], {
    type: 'image/svg+xml;charset=utf-8',
  });
}

export async function svgToPngBlob(svg: SVGSVGElement): Promise<Blob> {
  const { clone, geometry } = cloneForExport(svg);

  const exportStyle = document.createElementNS('http://www.w3.org/2000/svg', 'style');
  exportStyle.textContent = [
    'svg { background: #ffffff; }',
    'text, tspan { font-family: Arial, "Liberation Sans", sans-serif !important; }',
    '.label, .nodeLabel, .edgeLabel, .clusterLabel { font-family: Arial, "Liberation Sans", sans-serif !important; }',
  ].join('\n');
  clone.insertBefore(exportStyle, clone.firstChild);

  const scale = 2;
  const canvas = document.createElement('canvas');
  canvas.width = geometry.width * scale;
  canvas.height = geometry.height * scale;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('Canvas 2D context is unavailable');

  ctx.scale(scale, scale);
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, geometry.width, geometry.height);

  const v = await Canvg.from(ctx, new XMLSerializer().serializeToString(clone), {
    DOMParser,
    ignoreMouse: true,
    ignoreAnimation: true,
    fontBold: 'Arial',
    fontNormal: 'Arial',
    fontMono: 'Courier New',
  });
  await v.ready();
  await v.render();

  return new Promise<Blob>((resolve, reject) =>
    canvas.toBlob(b =>
      b ? resolve(b) : reject(new Error('PNG export failed')),
      'image/png'
    )
  );
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
