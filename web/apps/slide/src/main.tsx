import { createRoot } from 'react-dom/client';
import { SlideApp } from './components/SlideApp';

import 'reveal.js/dist/reveal.css';
import 'reveal.js/plugin/highlight/monokai.css';
import './styles.css';

const params = new URLSearchParams(location.search);
const filePath = params.get('file') || '';
const contentMode = params.get('content') === '1';

createRoot(document.getElementById('root')!).render(<SlideApp contentMode={contentMode} filePath={filePath} />);
