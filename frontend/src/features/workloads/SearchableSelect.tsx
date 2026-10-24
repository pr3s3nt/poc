import { useState, type Ref } from 'react';
import { t } from '../../shared/i18n';

type Option = { value: string; label: string };

// Keep native keyboard selection; long catalogs gain a name filter without changing the value.
export function SearchableSelect({ label, value, options, placeholder, disabled, selectRef, onChange }: {
  label: string; value: string; options: Option[]; placeholder: string; disabled: boolean;
  selectRef?: Ref<HTMLSelectElement>; onChange(value: string): void;
}) {
  const [query, setQuery] = useState('');
  const searchable = options.length > 10;
  const visible = options.filter((option) => !searchable || option.value === value || option.label.toLocaleLowerCase().includes(query.toLocaleLowerCase()));
  const select = <select ref={selectRef} aria-label={label} value={value} disabled={disabled} onChange={(event) => { onChange(event.target.value); setQuery(''); }}>
    <option value="">{placeholder}</option>
    {visible.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
  </select>;
  if (!searchable) return select;
  return <div className="env-search-select">
    <input type="search" aria-label={t('workloads.envTable.search', { label })} placeholder={t('workloads.envTable.searchPlaceholder')} value={query} disabled={disabled} onChange={(event) => setQuery(event.target.value)} />
    {select}
  </div>;
}
