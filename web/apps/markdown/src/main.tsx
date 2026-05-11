import { createRoot } from 'react-dom/client';
import { App as AntApp } from 'antd';
import { App } from './components/App';
import 'antd/dist/reset.css';
import 'github-markdown-css/github-markdown-light.css';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <AntApp>
    <App />
  </AntApp>
);
