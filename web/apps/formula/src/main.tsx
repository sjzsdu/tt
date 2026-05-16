import { createRoot } from 'react-dom/client';
import { App as AntApp, ConfigProvider, theme } from 'antd';
import '@fontsource/noto-sans/400.css';
import '@fontsource/noto-sans/700.css';
import 'antd/dist/reset.css';
import 'github-markdown-css/github-markdown-light.css';
import { App } from './components/App';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <ConfigProvider
    theme={{
      algorithm: theme.darkAlgorithm,
      token: {
        colorPrimary: '#66e3c4',
        colorInfo: '#7dd3fc',
        colorSuccess: '#4ade80',
        colorWarning: '#fbbf24',
        colorError: '#fb7185',
        borderRadius: 16,
        fontFamily: '"Noto Sans", sans-serif',
      },
    }}
  >
    <AntApp>
      <App />
    </AntApp>
  </ConfigProvider>
);
