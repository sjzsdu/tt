import type { TemplateConfig } from '../types';

export const DEFAULT_TEMPLATE = 'magicloud';

const magicloud: TemplateConfig = {
  name: 'magicloud',
  revealTheme: 'white',
  css: `
    :root {
      --r-background-color: #f5faf8;
      --r-main-font: 'Noto Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
      --r-main-font-size: 34px;
      --r-main-color: #143b33;
      --r-block-margin: 18px;
      --r-heading-margin: 0 0 16px 0;
      --r-heading-font: 'Noto Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
      --r-heading-color: #005f4d;
      --r-heading-line-height: 1.15;
      --r-heading-letter-spacing: 0;
      --r-heading-text-transform: none;
      --r-heading-text-shadow: none;
      --r-heading-font-weight: 760;
      --r-heading1-size: 1.35em;
      --r-heading2-size: 0.9em;
      --r-heading3-size: 0.68em;
      --r-code-font: 'SF Mono', 'Fira Code', monospace;
      --r-link-color: #007a62;
      --r-link-color-hover: #00a77f;
      --r-selection-background-color: rgba(0, 122, 98, 0.18);
      --r-selection-color: #0a2d27;

      --slide-bg: #f5faf8;
      --slide-fg: #143b33;
      --slide-accent: #007a62;
      --slide-accent-strong: #005f4d;
      --slide-accent-soft: #eaf6f1;
      --slide-muted: #698078;
      --slide-code-bg: #eef6f3;
      --slide-card-bg: #f0f8f4;
      --slide-card-border: #d7e8e1;
      --slide-table-border: #d7e8e1;
      --slide-table-header-bg: #007a62;
      --slide-table-row-alt: #eef7f3;
      --slide-blockquote-border: #007a62;
    }

    .reveal {
      background: #f5faf8;
      color: var(--slide-fg);
      font-family: var(--r-main-font);
    }

    .reveal .slides section {
      overflow: hidden;
      min-height: 100%;
      height: 100%;
      padding: 76px 76px 52px;
      background:
        linear-gradient(180deg, rgba(0, 122, 98, 0.035), rgba(255, 255, 255, 0) 32%),
        #ffffff;
      border: 1px solid #d5dedb;
      color: var(--slide-fg);
    }

    .reveal .slides section:not(:first-child)::before {
      content: "FLEXCOMPUTE  |  MagiCloud";
      position: absolute;
      top: 26px;
      left: 34px;
      z-index: 2;
      color: #0d4f42;
      font-size: 10px;
      font-weight: 800;
      letter-spacing: 0;
      line-height: 1;
    }

    .reveal .slides section:not(:first-child)::after {
      content: "";
      position: absolute;
      top: 0;
      right: 0;
      width: 48%;
      height: 112px;
      pointer-events: none;
      opacity: 0.42;
      background:
        linear-gradient(135deg, transparent 0 44%, rgba(0, 122, 98, 0.1) 45% 46%, transparent 47%),
        repeating-linear-gradient(165deg, rgba(0, 122, 98, 0.09) 0 1px, transparent 1px 24px);
      mask-image: linear-gradient(90deg, transparent, #000 36%, transparent);
    }

    .reveal h1,
    .reveal h2,
    .reveal h3 {
      color: var(--slide-accent-strong);
      letter-spacing: 0;
      text-transform: none;
    }

    .reveal h1 {
      max-width: 940px;
      font-size: 1.38em;
      font-weight: 800;
      line-height: 1.08;
    }

    .reveal h2 {
      margin-top: 4px;
      font-size: 0.74em;
      font-weight: 780;
    }

    .reveal h3 {
      font-size: 0.58em;
      color: #1e6859;
    }

    .reveal p {
      max-width: 760px;
      color: var(--slide-muted);
      font-size: 0.46em;
      line-height: 1.55;
    }

    .reveal strong {
      color: var(--slide-accent-strong);
      font-weight: 800;
    }

    .reveal ul,
    .reveal ol {
      margin: 18px 0 0;
      color: var(--slide-fg);
      font-size: 0.43em;
      line-height: 1.55;
    }

    .reveal ul {
      list-style: none;
    }

    .reveal li {
      position: relative;
      margin: 0 0 8px;
      padding-left: 18px;
    }

    .reveal ul li::before {
      content: "";
      position: absolute;
      left: 0;
      top: 0.72em;
      width: 5px;
      height: 5px;
      border-radius: 50%;
      background: var(--slide-accent);
    }

    .reveal ol {
      padding-left: 24px;
    }

    .reveal ol li {
      padding-left: 6px;
    }

    .reveal ol li::marker {
      color: var(--slide-accent);
      font-weight: 800;
    }

    .reveal blockquote {
      margin: 22px 0;
      padding: 18px 22px;
      border-left: 5px solid var(--slide-blockquote-border);
      border-radius: 0 8px 8px 0;
      background: var(--slide-accent-soft);
      color: var(--slide-fg);
      box-shadow: none;
    }

    .reveal blockquote p {
      color: var(--slide-fg);
      font-size: 0.44em;
    }

    .reveal code {
      background: var(--slide-code-bg);
      color: #005f4d;
    }

    .reveal pre {
      border: 1px solid var(--slide-card-border);
      background: #f4faf7;
      box-shadow: none;
      color: #0f3d35;
    }

    .reveal table {
      overflow: hidden;
      border-collapse: separate;
      border-spacing: 0;
      width: 100%;
      margin-top: 22px;
      border: 1px solid var(--slide-table-border);
      border-radius: 8px;
      font-size: 0.38em;
    }

    .reveal th,
    .reveal td {
      border: 0;
      border-bottom: 1px solid var(--slide-table-border);
      padding: 9px 14px;
    }

    .reveal th {
      background: var(--slide-table-header-bg);
      color: #ffffff;
      font-weight: 760;
    }

    .reveal tr:nth-child(even) td {
      background: var(--slide-table-row-alt);
    }

    .reveal tr:last-child td {
      border-bottom: 0;
    }

    .reveal img {
      max-width: 100%;
      max-height: 470px;
      border-radius: 0;
      box-shadow: none;
    }

    .reveal .progress {
      height: 3px;
      color: var(--slide-accent);
    }

    .reveal .controls {
      color: rgba(0, 95, 77, 0.72);
    }

    .reveal .slide-number {
      color: rgba(0, 95, 77, 0.65);
      background: transparent;
      font-size: 12px;
    }

    .reveal .slides section:first-child {
      padding: 78px 74px 60px;
      background:
        radial-gradient(circle at 88% 14%, rgba(0, 176, 132, 0.26), transparent 24%),
        radial-gradient(circle at 16% 22%, rgba(255, 255, 255, 0.1), transparent 26%),
        linear-gradient(135deg, #071f1b 0%, #003b2e 54%, #00533f 100%);
      color: #ffffff;
      border: 0;
    }

    .reveal .slides section:first-child::before {
      content: "FLEXCOMPUTE  |  MagiCloud";
      position: absolute;
      top: 68px;
      right: 70px;
      z-index: 2;
      color: rgba(255, 255, 255, 0.96);
      font-size: 14px;
      font-weight: 800;
      letter-spacing: 0;
    }

    .reveal .slides section:first-child::after {
      content: "";
      position: absolute;
      inset: 0;
      pointer-events: none;
      opacity: 0.34;
      background:
        radial-gradient(circle at 86% 72%, transparent 0 28%, rgba(122, 255, 211, 0.18) 29% 30%, transparent 31%),
        repeating-linear-gradient(24deg, transparent 0 25px, rgba(143, 255, 220, 0.08) 26px, transparent 27px);
      mask-image: linear-gradient(90deg, transparent 0%, #000 58%, #000 100%);
    }

    .reveal .slides section:first-child .slide-content {
      position: relative;
      z-index: 1;
      display: flex;
      flex-direction: column;
      justify-content: center;
      min-height: 100%;
    }

    .reveal .slides section:first-child h1 {
      margin: 78px 0 4px;
      color: #03a778;
      font-size: 1.05em;
      font-weight: 820;
    }

    .reveal .slides section:first-child h2,
    .reveal .slides section:first-child h3 {
      color: #ffffff;
    }

    .reveal .slides section:first-child p,
    .reveal .slides section:first-child li {
      color: rgba(255, 255, 255, 0.95);
      font-weight: 700;
    }

    .reveal .slides section:first-child p {
      font-size: 0.42em;
    }

    .reveal .slides section:first-child ul {
      margin-top: 70px;
    }

    .reveal .slides section:first-child ul li::before {
      background: #ffffff;
    }

    .slide-two-column .slide-content {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 18px 26px;
      align-items: stretch;
      height: auto;
    }

    .slide-two-column .slide-markdown {
      min-width: 0;
      padding: 0;
      border-left: 0;
    }

    .slide-two-column .slide-markdown:not(.slide-part-column) {
      grid-column: 1 / -1;
    }

    .slide-two-column .slide-part-column {
      position: relative;
      min-height: 172px;
      padding: 24px 28px 26px;
      border: 1px solid var(--slide-card-border);
      border-radius: 8px;
      background: var(--slide-card-bg);
    }

    .slide-two-column .slide-part-column::after {
      content: "";
      position: absolute;
      left: 22px;
      bottom: 18px;
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background:
        linear-gradient(45deg, transparent 45%, #ffffff 46% 55%, transparent 56%),
        var(--slide-accent);
    }

    .slide-two-column .slide-part-column h2 {
      margin-bottom: 8px;
      font-size: 0.52em;
    }

    .slide-two-column .slide-part-column p,
    .slide-two-column .slide-part-column li {
      font-size: 0.36em;
    }

    .slide-two-column .slide-part-column ul {
      margin-top: 8px;
      padding-bottom: 16px;
    }

    .slide-split .slide-content {
      display: grid;
      grid-template-columns: 0.9fr 1.35fr;
      gap: 34px;
      align-items: center;
      height: 100%;
    }

    .slide-split .slide-markdown {
      padding: 0;
      background: transparent;
    }

    .slide-split .slide-markdown:first-child {
      background: transparent;
      border-radius: 0;
    }

    .slide-split .slide-markdown:nth-child(2) {
      display: flex;
      justify-content: center;
      align-items: center;
    }

    .slide-split img {
      width: 100%;
      max-height: 430px;
      object-fit: cover;
    }

    .slide-center .slide-content {
      justify-content: center;
      text-align: center;
    }

    .slide-center h1 {
      margin-left: auto;
      margin-right: auto;
      font-size: 1.1em;
    }

    .slide-center p {
      margin-left: auto;
      margin-right: auto;
      color: var(--slide-muted);
    }

    .slide-diagram {
      border: 1px solid var(--slide-card-border);
      background: #fbfefd;
      border-radius: 8px;
    }

    .diagram-svg svg {
      max-height: 430px;
    }
  `,
  defaults: { theme: 'light', transition: 'fade', center: false },
};

const dark: TemplateConfig = {
  name: 'dark',
  revealTheme: 'black',
  css: `
    :root {
      --slide-bg: #0f172a;
      --slide-fg: #e2e8f0;
      --slide-accent: #38bdf8;
      --slide-muted: #94a3b8;
      --slide-code-bg: #1e293b;
      --slide-card-bg: #1e293b;
      --slide-card-border: #334155;
      --slide-table-border: #334155;
      --slide-table-header-bg: #1e293b;
      --slide-table-row-alt: #1e293b40;
      --slide-blockquote-border: #38bdf8;
    }
  `,
  defaults: { theme: 'dark', transition: 'slide', center: false },
};

const light: TemplateConfig = {
  name: 'light',
  revealTheme: 'white',
  css: `
    :root {
      --slide-bg: #ffffff;
      --slide-fg: #0f172a;
      --slide-accent: #2563eb;
      --slide-muted: #64748b;
      --slide-code-bg: #f1f5f9;
      --slide-card-bg: #f8fafc;
      --slide-card-border: #e2e8f0;
      --slide-table-border: #e2e8f0;
      --slide-table-header-bg: #f1f5f9;
      --slide-table-row-alt: #f8fafc;
      --slide-blockquote-border: #2563eb;
    }
  `,
  defaults: { theme: 'light', transition: 'slide', center: false },
};

const serif: TemplateConfig = {
  name: 'serif',
  revealTheme: 'serif',
  css: `
    :root {
      --slide-bg: #faf9f6;
      --slide-fg: #1c1917;
      --slide-accent: #b45309;
      --slide-muted: #78716c;
      --slide-code-bg: #f5f5f4;
      --slide-card-bg: #f5f5f4;
      --slide-card-border: #d6d3d1;
      --slide-table-border: #d6d3d1;
      --slide-table-header-bg: #f5f5f4;
      --slide-table-row-alt: #f5f5f480;
      --slide-blockquote-border: #b45309;
    }
    .reveal { font-family: 'Georgia', 'Noto Serif', serif; }
    .reveal h1, .reveal h2, .reveal h3 { font-family: 'Georgia', 'Noto Serif', serif; font-weight: 700; }
  `,
  defaults: { theme: 'dark', transition: 'fade', center: false },
};

const white: TemplateConfig = {
  name: 'white',
  revealTheme: 'white',
  css: `
    :root {
      --slide-bg: #ffffff;
      --slide-fg: #1a1a1a;
      --slide-accent: #0066cc;
      --slide-muted: #666666;
      --slide-code-bg: #f5f5f5;
      --slide-card-bg: #f9f9f9;
      --slide-card-border: #e0e0e0;
      --slide-table-border: #e0e0e0;
      --slide-table-header-bg: #f5f5f5;
      --slide-table-row-alt: #fafafa;
      --slide-blockquote-border: #0066cc;
    }
  `,
  defaults: { theme: 'light', transition: 'fade', center: false },
};

export const templates: Record<string, TemplateConfig> = {
  magicloud,
  dark,
  light,
  serif,
  white,
};

export function getTemplate(name: string): TemplateConfig {
  return templates[name] || templates[DEFAULT_TEMPLATE];
}

export function listTemplates(): string[] {
  return Object.keys(templates);
}
