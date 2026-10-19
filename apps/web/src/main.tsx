import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './i18n';
import { App } from './App';

const RootApp = __NGFW_UI_PREVIEW__
  ? (await import('./preview/PreviewApp')).PreviewApp
  : App;

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RootApp />
  </StrictMode>,
);
