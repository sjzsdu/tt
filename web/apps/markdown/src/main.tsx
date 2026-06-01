import { createRoot } from 'react-dom/client';
import { App as AntApp, ConfigProvider, theme } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { App } from './components/App';
import 'antd/dist/reset.css';
import 'github-markdown-css/github-markdown-light.css';
import './styles.css';

type AppTheme = 'light' | 'dark';
const THEME_STORAGE_KEY = 'markdown-ui-theme';

function readInitialTheme(): AppTheme {
  if (typeof window === 'undefined') return 'light';
  return window.localStorage.getItem(THEME_STORAGE_KEY) === 'dark' ? 'dark' : 'light';
}

function Root() {
  const [appTheme, setAppTheme] = useState<AppTheme>(() => readInitialTheme());

  useEffect(() => {
    document.documentElement.dataset.theme = appTheme;
    document.documentElement.style.colorScheme = appTheme;
    window.localStorage.setItem(THEME_STORAGE_KEY, appTheme);
  }, [appTheme]);

  const antdTheme = useMemo(() => ({
    algorithm: appTheme === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
    token: {
      colorPrimary: '#2563eb',
      colorInfo: '#2563eb',
      colorSuccess: '#16a34a',
      colorWarning: '#d97706',
      colorError: '#dc2626',
      borderRadius: 14,
    },
  }), [appTheme]);

  return (
    <ConfigProvider theme={antdTheme}>
      <AntApp>
        <App theme={appTheme} onThemeChange={setAppTheme} />
      </AntApp>
    </ConfigProvider>
  );
}

createRoot(document.getElementById('root')!).render(<Root />);
