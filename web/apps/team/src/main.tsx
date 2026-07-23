import { createRoot } from 'react-dom/client';
import { App as AntApp, ConfigProvider, theme } from 'antd';
import '@fontsource/noto-sans/400.css';
import '@fontsource/noto-sans/700.css';
import 'antd/dist/reset.css';
import { useEffect, useMemo, useState } from 'react';
import { App } from './App';
import './styles.css';

export type AppTheme = 'light' | 'dark';

const THEME_STORAGE_KEY = 'team-ui-theme';

function readInitialTheme(): AppTheme {
  if (typeof window === 'undefined') return 'dark';
  return window.localStorage.getItem(THEME_STORAGE_KEY) === 'light' ? 'light' : 'dark';
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
      colorPrimary: '#82d6ff',
      colorInfo: '#82d6ff',
      colorSuccess: '#b6f36b',
      colorWarning: '#ffd479',
      colorError: '#ff8b8b',
      borderRadius: 16,
      fontFamily: '"Noto Sans", sans-serif',
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
