import { useEffect, useState } from 'react';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { fetchEnvironment } from './api';

// Whether an active or interrupted Environment operation currently holds the
// Environment's writes (ADR-012). The page may have been opened before the
// operation started, so the target is refreshed periodically; the answer is
// local to the page and also pushed to the shared cache when a callback exists.
// Unsaved form state of the caller is never touched, so it survives a release.
export function useEnvironmentBusy(application: Application, environment: EnvironmentKey, onTargetChange?: (applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget) => void, pollMs = 3000): boolean {
  const cached = Boolean(application.environments[environment].activeOperation);
  const [live, setLive] = useState<{ scope: string; busy: boolean }>();
  const scope = `${application.id}/${environment}`;
  useEffect(() => {
    let cancelled = false;
    const refresh = () => fetchEnvironment(application.id, environment).then((latest) => {
      if (cancelled || !latest) return;
      setLive({ scope, busy: Boolean(latest.activeOperation) });
      onTargetChange?.(application.id, environment, latest);
    }).catch(() => undefined);
    const timer = window.setInterval(() => void refresh(), pollMs);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [application.id, environment, pollMs]); // eslint-disable-line react-hooks/exhaustive-deps
  // A newer cache (for example the Settings banner) wins over an older live answer.
  useEffect(() => { setLive((current) => current && current.scope === scope && current.busy !== cached ? { scope, busy: cached } : current); }, [cached, scope]);
  return live && live.scope === scope ? live.busy : cached;
}
