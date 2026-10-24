import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { initI18n } from './shared/i18n';
import './styles/index.css';

const root = document.getElementById('root');
if (!root) throw new Error('Missing #root');

// Catalogs load before the application modules evaluate, so module-level
// labels already resolve in the chosen language.
void initI18n(import.meta.env.BASE_URL).then(() => import('./app/App')).then(({ App }) => {
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
});
