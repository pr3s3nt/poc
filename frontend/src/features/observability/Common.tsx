import { Button } from '../../shared/ui/Button';
import type { Snapshot } from './api';
import { text, time } from './model';

export function CollectionNotice({ data }: { data: Snapshot }) {
  let key = '';
  if (data.state === 'WAITING') key = 'waitingHelp';
  else if (data.state === 'METRICS_UNAVAILABLE') key = 'missingHelp';
  else if (data.state === 'STORAGE_LIMIT') key = 'storageHelp';
  else if (data.state === 'STALE' || data.state === 'PARTIAL') key = 'staleHelp';
  else if (data.state === 'UNAVAILABLE') key = 'unavailableHelp';
  else if (data.state === 'INACTIVE') key = 'inactiveHelp';
  if (!key) return null;

  const affected = data.sources.flatMap(source => [
    { signal: 'logs', code: source.logsCode, last: source.lastLogAt },
    { signal: 'metrics', code: source.metricsCode, last: source.lastMetricAt },
  ].filter(item => item.code !== 'OK' || source.storageLimited).map(item => ({
    ...item,
    code: source.storageLimited ? 'STORAGE_LIMIT' : item.code,
    workload: source.workloadId,
  })));
  const known = new Set(['FORBIDDEN', 'UNAVAILABLE', 'STALE', 'GAP', 'TRUNCATED', 'STORAGE_LIMIT']);
  return <div className="obs-warning">
    <span role="status">{text(key)}</span>
    {data.state !== 'INACTIVE' ? <details><summary>{text('viewCause')}</summary>
      {affected.length ? <ul>{affected.map(item => <li key={`${item.workload}/${item.signal}`}>
        <strong>{item.workload} · {text(item.signal)}</strong>
        {item.last ? <span> · {text('lastCollected', { time: time(item.last, true) })}</span> : null}
        <p>{text(known.has(item.code) ? `collectionCause.${item.code}` : 'collectionCause.unknown')}</p>
      </li>)}</ul> : <p>{text('collectionCause.unknown')}</p>}
    </details> : null}
  </div>;
}

export function EmptyTelemetry({ state }: { state: string }) {
  return <div className="obs-empty"><span className="obs-empty-icon" aria-hidden="true">◷</span><h3>{text(state === 'NOT_DEPLOYED' ? 'noDeploy' : state === 'UNAVAILABLE' ? 'unavailable' : 'waitingText')}</h3><p>{text(state === 'NOT_DEPLOYED' ? 'noDeployHelp' : state === 'UNAVAILABLE' ? 'unavailableHelp' : 'waitingHelp')}</p></div>;
}

export function QueryFailure({ message, retry }: { message: string; retry(): void }) {
  return <div className="obs-warning" role="status"><span>{message}</span><Button onClick={retry}>{text('retry')}</Button></div>;
}

export function MiniTrend({ values, tone = 'amber' }: { values: (number | null)[]; tone?: string }) {
  const known = values.filter((value): value is number => value !== null), max = Math.max(1, ...known);
  let active = false;
  const path = values.map((value, index) => {
    if (value === null) { active = false; return ''; }
    const point = `${active ? 'L' : 'M'}${index / Math.max(1, values.length - 1) * 160},${28 - value / max * 24}`;
    active = true; return point;
  }).join(' ');
  return <svg className={`obs-mini-line obs-tone-${tone}`} viewBox="0 0 160 32" aria-hidden="true"><path d={path} />{values.map((value, index) => value !== null && values[index - 1] == null && values[index + 1] == null ? <circle key={index} cx={index / Math.max(1, values.length - 1) * 160} cy={28 - value / max * 24} r="1.8" fill="currentColor" /> : null)}</svg>;
}
