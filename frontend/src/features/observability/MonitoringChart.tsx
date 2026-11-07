import { t } from '../../shared/i18n';
export type Point = { timestamp: string; value: number | null };
export type Series = { metric: Record<string,string>; values?: Point[]; value?: Point };
const display = (n: number | null, divisor = 1) => n === null ? '—' : new Intl.NumberFormat(undefined, {maximumFractionDigits:2}).format(n / divisor);
const label = (key: string) => t(`monitoring.${key}`);

export function Chart({ title, series, divisor = 1 }: { title: string; series: Series[]; divisor?: number }) {
  const points = series[0]?.values ?? [];
  const max = Math.max(1, ...points.flatMap(p => p.value === null ? [] : [p.value / divisor]));
  const from = Date.parse(points[0]?.timestamp ?? ''), to = Date.parse(points.at(-1)?.timestamp ?? '');
  const x = (p: Point) => 10 + (Date.parse(p.timestamp) - from) / Math.max(1, to - from) * 580;
  const y = (p: Point) => 120 - (p.value ?? 0) / divisor / max * 100;
  return <section><h4>{title}</h4>{points.some(p => p.value !== null) ? <svg viewBox="0 0 600 150" role="img" aria-label={title} style={{ width: '100%', maxHeight: 180 }}>
    {points.slice(1).map((p, i) => { const prior = points[i]; return prior && p.value !== null && prior.value !== null && Date.parse(p.timestamp) - Date.parse(prior.timestamp) <= 1.5 * (to - from) / Math.max(1, points.length - 1) ? <line key={p.timestamp} x1={x(prior)} x2={x(p)} y1={y(prior)} y2={y(p)} stroke="#0969da" strokeWidth="2" /> : null; })}
    {points.filter(p => p.value !== null).map(p => <circle key={p.timestamp} cx={x(p)} cy={y(p)} r="3" tabIndex={0}><title>{new Date(p.timestamp).toLocaleString()} · {display(p.value, divisor)}</title></circle>)}
    <text x="10" y="145" fontSize="10">{new Date(from).toLocaleTimeString()}</text><text x="500" y="145" fontSize="10">{new Date(to).toLocaleTimeString()}</text>
  </svg> : <p>{label('noData')}</p>}</section>;
}
