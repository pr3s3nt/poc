import '@testing-library/jest-dom/vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { setCatalogs } from '../shared/i18n';

// Tests render the default Vietnamese catalog, the same file the Console serves.
const vi = JSON.parse(readFileSync(resolve(__dirname, '../../public/locales/vi.json'), 'utf8')) as Record<string, string>;
setCatalogs('vi', vi);
