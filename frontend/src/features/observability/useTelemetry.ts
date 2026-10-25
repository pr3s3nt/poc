import { useEffect, useRef, useState } from 'react';
import type { EnvironmentKey } from '../../shared/types/application';
import { describeError } from '../../shared/api/client';
import { interval, readTelemetry, type Filters, type Snapshot } from './api';
import { text } from './model';

export function useTelemetry(app: string, env: EnvironmentKey, kind: 'overview' | 'logs' | 'metrics', filters: Filters, pollMs = 0, relativeMinutes?: number, refreshKey = '') {
  const key = JSON.stringify(filters), scope = `${app}/${env}/${kind}/${key}/${relativeMinutes ?? ''}`;
  const [record, setRecord] = useState<{ scope: string; value: Snapshot }>();
  const [failure, setFailure] = useState<{ scope: string; message: string }>();
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const sequence = useRef(0);
  useEffect(() => {
    const current = ++sequence.current, controller = new AbortController();
    const selected = JSON.parse(key) as Filters;
    let running = false;
    const load = async () => {
      if (running || controller.signal.aborted) return;
      running = true; setLoading(true);
      try {
        const query = relativeMinutes ? { ...selected, ...interval(relativeMinutes) } : selected;
        const value = await readTelemetry(app, env, kind, query, controller.signal);
        if (current === sequence.current && !controller.signal.aborted) { setRecord({ scope, value }); setFailure(undefined); }
      } catch (error) {
        if (current === sequence.current && !controller.signal.aborted) setFailure({ scope, message: describeError(error, text('errorText')) });
      } finally {
        running = false;
        if (current === sequence.current && !controller.signal.aborted) setLoading(false);
      }
    };
    void load();
    const timer = pollMs ? window.setInterval(() => void load(), pollMs) : undefined;
    return () => { controller.abort(); if (timer) window.clearInterval(timer); };
  }, [app, env, kind, key, scope, pollMs, relativeMinutes, refreshKey, attempt]);
  return { data: record?.scope === scope ? record.value : undefined, error: failure?.scope === scope ? failure.message : '', loading, reload: () => setAttempt(value => value + 1) };
}
