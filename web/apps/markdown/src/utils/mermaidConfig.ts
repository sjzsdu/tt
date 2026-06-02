import { mermaidThemeCss, mermaidThemeVariables, type AppTheme } from './mermaidTheme';

export type { AppTheme } from './mermaidTheme';

export function createMermaidConfig(theme: AppTheme) {
  return {
    startOnLoad: false,
    theme: 'base' as const,
    securityLevel: 'loose' as const,
    logLevel: 'error' as const,
    flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' as const },
    themeVariables: mermaidThemeVariables(theme),
    themeCSS: mermaidThemeCss(theme),
  };
}
