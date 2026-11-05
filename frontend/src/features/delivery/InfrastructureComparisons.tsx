import { t } from '../../shared/i18n';
export type InfrastructureComparison = { address: string; path: string; desired: string | number | boolean; actual: string | number | boolean };
export function InfrastructureComparisons({ comparisons }: { comparisons?: InfrastructureComparison[] }) {
  if (!comparisons?.length) return null;
  return <table className="plain-table"><thead><tr><th>{t('infrastructureDrift.fields')}</th><th>{t('infrastructureDrift.desired')}</th><th>{t('infrastructureDrift.actual')}</th></tr></thead><tbody>{comparisons.map(item => <tr key={`${item.address}:${item.path}`}><td><code>{item.address}</code><br />{item.path}</td><td>{String(item.desired)}</td><td>{String(item.actual)}</td></tr>)}</tbody></table>;
}
