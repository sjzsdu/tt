export type AppTheme = 'light' | 'dark';

export type TemplateConfig = {
  name: string;
  revealTheme: string;
  css: string;
  defaults: {
    theme: AppTheme;
    transition: string;
    center: boolean;
    margin?: number;
    width?: number;
    height?: number;
  };
};

export type SlideLayout = 'default' | 'center' | 'two-column' | 'split' | 'logo' | 'closing' | 'full-image';

export type SlideMeta = {
  title: string;
  layout: SlideLayout;
  total: number;
  transition: string;
};

export type SlideRuntimeConfig = {
  transition?: string;
  controls?: boolean;
  progress?: boolean;
  slideNumber?: boolean | string;
  overview?: boolean;
  center?: boolean;
  autoSlide?: number;
  width?: number;
  height?: number;
  margin?: number;
};

export type SlideData = {
  index: number;
  parts: SlidePart[];
  layout: SlideLayout;
  class: string;
};

export type SlidePart =
  | { type: 'markdown'; html: string; role?: 'column' }
  | { type: 'mermaid'; code: string }
  | { type: 'd2'; code: string };
