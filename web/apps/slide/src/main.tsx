import { createRoot } from 'react-dom/client';
import { SlideApp } from './components/SlideApp';
import type { SlideRuntimeConfig } from './types';

import 'reveal.js/dist/reveal.css';
import 'reveal.js/plugin/highlight/monokai.css';
import './styles.css';

const params = new URLSearchParams(location.search);
const filePath = params.get('file') || '';
const contentMode = params.get('content') === '1';
const templateOverride = params.get('template') || '';

const parseBool = (value: string | null): boolean | undefined => {
  if (value == null || value === '') return undefined;
  return ['1', 'true', 'yes', 'on'].includes(value.toLowerCase());
};

const parseNumber = (value: string | null): number | undefined => {
  if (value == null || value === '') return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
};

const parseSlideNumber = (value: string | null): boolean | string | undefined => {
  if (value == null || value === '') return undefined;
  if (['0', 'false', 'no', 'off'].includes(value.toLowerCase())) return false;
  if (['1', 'true', 'yes', 'on'].includes(value.toLowerCase())) return true;
  return value;
};

const runtimeConfig: SlideRuntimeConfig = {
  transition: params.get('transition') || undefined,
  controls: parseBool(params.get('controls')),
  progress: parseBool(params.get('progress')),
  slideNumber: parseSlideNumber(params.get('slideNumber') || params.get('slide-number')),
  overview: parseBool(params.get('overview')),
  center: parseBool(params.get('center')),
  autoSlide: parseNumber(params.get('autoSlide') || params.get('auto-slide')),
  width: parseNumber(params.get('width')),
  height: parseNumber(params.get('height')),
  margin: parseNumber(params.get('margin')),
};

createRoot(document.getElementById('root')!).render(
  <SlideApp contentMode={contentMode} filePath={filePath} templateOverride={templateOverride} runtimeConfig={runtimeConfig} />
);
