// Runtime message catalogs of the Web Console. Catalogs are plain JSON files
// served from `<base>/locales/`; `manifest.json` lists the selectable languages,
// so adding a language means adding a catalog and a manifest entry, not code.
// The fallback catalog fills keys a translation does not have yet.

export interface LocaleInfo { code: string; name: string; intl?: string }
export type Catalog = Readonly<Record<string, string>>;
export type MessageParams = Readonly<Record<string, unknown>>;

export const fallbackLocale = 'vi';
export const localeStorageKey = 'orchestrator.locale';

let locale = fallbackLocale;
let locales: readonly LocaleInfo[] = [{ code: fallbackLocale, name: 'Tiếng Việt', intl: 'vi-VN' }];
let active: Catalog = {};
let fallback: Catalog = {};

function lookup(key: string, count: number | undefined): string | undefined {
  if (count !== undefined) {
    const form = `${key}_${new Intl.PluralRules(intlLocale()).select(count)}`;
    const plural = active[form] ?? (active === fallback ? undefined : fallback[form]);
    if (plural !== undefined) return plural;
  }
  return active[key] ?? fallback[key];
}

// Translates `key` and replaces `{name}` placeholders with `params`; like JSX,
// null, undefined and booleans render as nothing. A missing key renders as the
// key itself so the gap is visible instead of blank.
export function t(key: string, params?: MessageParams): string {
  const count = typeof params?.count === 'number' ? params.count : undefined;
  const message = lookup(key, count) ?? key;
  if (!params) return message;
  return message.replace(/\{(\w+)\}/g, (match, name: string) => {
    if (!Object.prototype.hasOwnProperty.call(params, name)) return match;
    const value = params[name];
    return value === null || value === undefined || typeof value === 'boolean' ? '' : String(value);
  });
}

export function currentLocale(): string { return locale; }
export function availableLocales(): readonly LocaleInfo[] { return locales; }
export function intlLocale(): string { return locales.find((item) => item.code === locale)?.intl ?? locale; }

export function formatDateTime(value: Date): string { return value.toLocaleString(intlLocale()); }

// Installs catalogs directly; used by tests and by `initI18n`.
export function setCatalogs(code: string, catalog: Catalog, fallbackCatalog: Catalog = catalog, list: readonly LocaleInfo[] = locales): void {
  locale = code;
  active = catalog;
  fallback = fallbackCatalog;
  locales = list;
  if (typeof document !== 'undefined') document.documentElement.lang = code;
}

function storedLocale(): string | null {
  try { return window.localStorage.getItem(localeStorageKey); }
  catch { return null; }
}

export function pickLocale(list: readonly LocaleInfo[], stored: string | null, browser: readonly string[]): string {
  const codes = list.map((item) => item.code);
  if (stored && codes.includes(stored)) return stored;
  for (const tag of browser) {
    const lower = tag.toLowerCase();
    const match = codes.find((code) => code.toLowerCase() === lower) ?? codes.find((code) => code.toLowerCase() === lower.split('-')[0]);
    if (match) return match;
  }
  return codes.includes(fallbackLocale) ? fallbackLocale : codes[0] ?? fallbackLocale;
}

async function fetchJSON<T>(url: string): Promise<T> {
  const response = await fetch(url, { cache: 'no-cache' });
  if (!response.ok) throw new Error(`${url}: ${response.status}`);
  return (await response.json()) as T;
}

// Loads the manifest, the chosen catalog and the fallback catalog. A missing
// manifest or catalog degrades to the fallback language rather than blocking.
export async function initI18n(base: string): Promise<void> {
  const root = `${base.replace(/\/?$/, '/')}locales/`;
  let list = locales;
  try {
    const manifest = await fetchJSON<LocaleInfo[]>(`${root}manifest.json`);
    if (Array.isArray(manifest) && manifest.length) list = manifest.filter((item) => typeof item?.code === 'string' && /^[A-Za-z0-9-]+$/.test(item.code));
  } catch { /* keep the built-in fallback entry */ }
  const code = pickLocale(list, storedLocale(), typeof navigator === 'undefined' ? [] : navigator.languages ?? [navigator.language]);
  const load = (item: string) => fetchJSON<Record<string, string>>(`${root}${item}.json`).catch(() => ({}));
  const [fallbackCatalog, catalog] = await Promise.all([load(fallbackLocale), code === fallbackLocale ? Promise.resolve(undefined) : load(code)]);
  setCatalogs(code, catalog ?? fallbackCatalog, fallbackCatalog, list);
}

// Persists the choice and reloads so every module renders in the new language.
export function changeLocale(code: string): void {
  try { window.localStorage.setItem(localeStorageKey, code); }
  catch { /* the choice lasts for this load only */ }
  window.location.reload();
}
