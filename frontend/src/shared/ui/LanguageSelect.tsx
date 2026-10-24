import { availableLocales, changeLocale, currentLocale, t } from '../i18n';

// Lists the languages of the served manifest; hidden while only one exists.
export function LanguageSelect({ className = '' }: { className?: string }) {
  const locales = availableLocales();
  if (locales.length < 2) return null;
  return <label className={`language-select ${className}`.trim()}>
    <span>{t('shared.languageSelect.label')}</span>
    <select value={currentLocale()} onChange={(event) => changeLocale(event.target.value)}>
      {locales.map((item) => <option key={item.code} value={item.code} lang={item.code}>{item.name}</option>)}
    </select>
  </label>;
}
