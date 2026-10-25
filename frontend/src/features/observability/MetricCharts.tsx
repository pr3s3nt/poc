import { intlLocale } from '../../shared/i18n';
import { useState } from 'react';
import type { Metric, Snapshot } from './api';
import { color, dateTime, filledPoints, number, text, time, unit } from './model';

export function MetricChart({ data, metric, selected = '', focused = false, onLogs }: { data: Snapshot; metric: Metric; selected?: string; focused?: boolean; onLogs(at: number): void }) {
  const [hover, setHover] = useState<number | null>(null);
  const names = data.workloads.filter(workload => !selected || workload === selected);
  const series = names.map(workload => ({ workload, points: filledPoints(data, workload, metric).map(point => ({ ...point, value: point.value !== null && metric === 'memory' ? point.value / 1048576 : point.value })) }));
  const known = series.flatMap(series => series.points.flatMap(point => point.value === null ? [] : [point.value]));
  const length = series[0]?.points.length ?? 0;
  const width = 540, left = 46, right = 524, top = 18, bottom = 188;
  const highest = Math.max(1, ...known), magnitude = 10 ** Math.floor(Math.log10(highest));
  const max = Math.ceil(highest * 1.15 / magnitude) * magnitude;
  const start = Date.parse(data.from), end = Date.parse(data.to);
  const x = (at: number) => left + Math.max(0, Math.min(1, (at - start) / Math.max(1, end - start))) * (right - left);
  const stamp = hover === null ? undefined : series[0]?.points[hover]?.at;
  const title = text(metric === 'cpu' ? 'historyCPU' : 'historyMemory');
  return <section className={`obs-chart${focused ? ' obs-chart-focused' : ''}`} aria-label={title}>
    <div className="obs-chart-heading"><h3>{title}</h3><span>{unit(metric)}</span></div>
    {known.length ? <><div className="obs-chart-body" role="group" tabIndex={0} aria-label={`${title}. ${text('tooltipKeyboard')}`} onMouseLeave={() => setHover(null)} onKeyDown={event => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); setHover(previous => Math.max(0, Math.min(length - 1, (previous ?? length - 1) + (event.key === 'ArrowLeft' ? -1 : 1)))); } }}>
      <svg viewBox={`0 0 ${width} 222`} role="img" aria-label={`${title}, ${dateTime(data.from)} – ${dateTime(data.to)}`} onMouseMove={event => { const box = event.currentTarget.getBoundingClientRect(); setHover(Math.max(0, Math.min(length - 1, Math.round(((event.clientX - box.left) / box.width * width - left) / (right - left) * (length - 1))))); }}>
        {[0, 1, 2, 3, 4].map(tick => <g key={tick}><line x1={left} x2={right} y1={bottom - tick / 4 * (bottom - top)} y2={bottom - tick / 4 * (bottom - top)} className="obs-gridline" /><text x={left - 10} y={bottom - tick / 4 * (bottom - top) + 4} textAnchor="end">{number(tick / 4 * max, 1)}</text></g>)}
        {series.map(series => {
          let active = false;
          const d = series.points.map(point => { if (point.value === null) { active = false; return ''; } const segment = `${active ? 'L' : 'M'}${x(point.at)},${bottom - point.value / max * (bottom - top)}`; active = true; return segment; }).join(' ');
          return <g key={series.workload}><path className="obs-series" d={d} stroke={color(series.workload, data.workloads)} />{series.points.map((point, index) => point.value !== null && (series.points[index - 1]?.value == null && series.points[index + 1]?.value == null) ? <circle key={point.at} cx={x(point.at)} cy={bottom - point.value / max * (bottom - top)} r="2.5" fill={color(series.workload, data.workloads)} /> : null)}</g>;
        })}
        {[0, 1, 2, 3, 4].map(index => { const at = start + (end - start) * index / 4; return <text key={index} x={left + (right - left) * index / 4} y={213} textAnchor={index === 0 ? 'start' : index === 4 ? 'end' : 'middle'}>{end - start > 86400000 ? new Date(at).toLocaleDateString(intlLocale(), { day: '2-digit', month: '2-digit' }) : time(at)}</text>; })}
        {stamp !== undefined ? <line className="obs-crosshair" x1={x(stamp)} x2={x(stamp)} y1={top} y2={bottom} /> : null}
      </svg>
      {hover !== null && stamp !== undefined ? <div className="obs-chart-tooltip" role="status"><strong>{dateTime(stamp)}</strong>{series.map(series => <span key={series.workload}><i style={{ background: color(series.workload, data.workloads) }} />{series.workload}<b>{series.points[hover]?.value === null || series.points[hover]?.value === undefined ? '—' : `${number(series.points[hover]!.value!, 1)} ${unit(metric)}`}</b></span>)}{stamp < Date.now() - data.logsRetentionDays * 86400000 ? <span>{text('logExpired')}</span> : <button onClick={() => onLogs(stamp)}>{text('zoom')} →</button>}</div> : null}
    </div><div className="obs-legend">{names.map(workload => <span key={workload}><i style={{ background: color(workload, data.workloads) }} />{workload}</span>)}</div></> : <div className="obs-empty obs-chart-unavailable"><strong>—</strong><span>{text('chartEmpty')}</span></div>}
  </section>;
}
