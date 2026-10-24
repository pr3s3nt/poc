import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { currentLocale, intlLocale, pickLocale, setCatalogs, t, type LocaleInfo } from '.';

const localesDir = resolve(__dirname, '../../../public/locales');
const readJSON = <T>(name: string): T => JSON.parse(readFileSync(resolve(localesDir, name), 'utf8')) as T;
const vi = readJSON<Record<string, string>>('vi.json');
const manifest = readJSON<LocaleInfo[]>('manifest.json');
const placeholders = (message: string) => [...message.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();

afterEach(() => setCatalogs('vi', vi));

describe('message catalogs', () => {
  it('lists Vietnamese and English in the manifest', () => {
    expect(manifest.map((item) => item.code)).toEqual(expect.arrayContaining(['vi', 'en']));
  });

  it.each(manifest.map((item) => item.code))('catalog %s has every key of vi with the same placeholders', (code) => {
    const catalog = readJSON<Record<string, string>>(`${code}.json`);
    expect(Object.keys(catalog).sort()).toEqual(Object.keys(vi).sort());
    for (const [key, message] of Object.entries(vi)) expect(placeholders(catalog[key] ?? ''), key).toEqual(placeholders(message));
  });
});

describe('t', () => {
  it('interpolates params and renders empty values like JSX', () => {
    setCatalogs('en', { greet: 'Open {env}{suffix}', count_one: '{count} item', count_other: '{count} items' }, vi);
    expect(t('greet', { env: 'staging', suffix: undefined })).toBe('Open staging');
    expect(t('count', { count: 1 })).toBe('1 item');
    expect(t('count', { count: 3 })).toBe('3 items');
  });

  it('falls back to the default catalog, then to the key', () => {
    setCatalogs('en', {}, { only: 'Chỉ tiếng Việt' });
    expect(t('only')).toBe('Chỉ tiếng Việt');
    expect(t('missing.key')).toBe('missing.key');
  });

  it('switches the Intl locale with the language', () => {
    setCatalogs('en', {}, vi, manifest);
    expect(currentLocale()).toBe('en');
    expect(intlLocale()).toBe('en-US');
    expect(document.documentElement.lang).toBe('en');
  });
});

describe('pickLocale', () => {
  const list: LocaleInfo[] = [{ code: 'vi', name: 'Tiếng Việt' }, { code: 'en', name: 'English' }];
  it('prefers the stored choice, then the browser language, then Vietnamese', () => {
    expect(pickLocale(list, 'en', ['vi-VN'])).toBe('en');
    expect(pickLocale(list, 'fr', ['en-GB', 'vi'])).toBe('en');
    expect(pickLocale(list, null, ['ja-JP'])).toBe('vi');
  });
});
