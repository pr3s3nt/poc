import { createRoot } from 'react-dom/client';
import { initI18n, setCatalogs, type Catalog } from '../../shared/i18n';
import '../../styles/index.css';

// Isolated entry: no product router, API client or persistence is mounted.
void initI18n('/ui/').then(async () => {
  const [base, mock] = await Promise.all([
    fetch('/ui/locales/vi.json').then((r) => r.json() as Promise<Catalog>),
    fetch(new URL('./vi.json', import.meta.url)).then((r) => r.json() as Promise<Catalog>),
  ]);
  setCatalogs('vi', { ...base, ...mock }, { ...base, ...mock }, [{ code: 'vi', name: 'Tiếng Việt' }]);
  const { WorkloadMock } = await import('./WorkloadMock');
  createRoot(document.getElementById('root')!).render(<WorkloadMock />);
});
