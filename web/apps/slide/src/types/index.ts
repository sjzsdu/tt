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
  };
};

export type SlideLayout = 'default' | 'center' | 'two-column' | 'split' | 'grid' | 'cards' | 'flex' | 'hero' | 'media-left' | 'media-right' | 'logo' | 'closing' | 'full-image';

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
  margin?: number;
};

export type SlideData = {
  index: number;
  parts: SlidePart[];
  layout: SlideLayout;
  class: string;
};

export type SlidePart =
  | { type: 'markdown'; html: string; role?: 'column' | 'card' | 'item' | 'media' | 'main' | 'aside' }
  | { type: 'mermaid'; code: string; role?: 'column' | 'card' | 'item' | 'media' | 'main' | 'aside' }
  | { type: 'd2'; code: string; role?: 'column' | 'card' | 'item' | 'media' | 'main' | 'aside' }
  | { type: 'widget'; widgetType: string; data: Record<string, unknown>; raw: string; role?: 'column' | 'card' | 'item' | 'media' | 'main' | 'aside' };

export type SlideWidgetTemplate = {
  type: string;
  html: string;
  css?: string;
  source?: 'project' | 'global';
};

export type SlideWidgetRegistry = Record<string, SlideWidgetTemplate>;
