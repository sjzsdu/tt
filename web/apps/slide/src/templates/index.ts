import type { TemplateConfig } from '../types';

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
  dark,
  light,
  serif,
  white,
};

export function getTemplate(name: string): TemplateConfig {
  return templates[name] || templates.dark;
}

export function listTemplates(): string[] {
  return Object.keys(templates);
}
