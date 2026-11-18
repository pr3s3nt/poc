import { useEffect, useState } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { InitializePage } from '../setup/InitializePage';
import { navigate } from '../../app/routes';
import { SignInPage } from './SignInPage';

export function SignInEntry(props: { onSuccess(username: string, password: string): Promise<void>; notice?: string }) {
  const [mode, setMode] = useState<'local' | 'oidc' | 'setup' | 'loading' | 'error'>('loading');
  const [localLoginAllowed, setLocalLoginAllowed] = useState(false);
  const [initialized, setInitialized] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    api<{ mode: string; localLoginAllowed?: boolean; setupAvailable?: boolean }>('/auth/config').then((result) => {
      if (active) { setMode(result.setupAvailable ? 'setup' : result.mode === 'local' || result.mode === 'oidc' ? result.mode : 'error'); setLocalLoginAllowed(result.localLoginAllowed === true); }
    }).catch(() => { if (active) setMode('error'); });
    return () => { active = false; };
  }, [attempt]);
  if (mode === 'setup') return <InitializePage onComplete={() => { setMode('local'); setInitialized(true); }} />;
  if (mode === 'loading') return <div className="page" role="status">{t('app.app.loadingOrchestrator')}</div>;
  if (mode === 'error') return <div className="page"><p role="alert">{t('auth.sso.configError')}</p><Button onClick={() => { setMode('loading'); setAttempt(attempt + 1); }}>{t('app.app.retry')}</Button></div>;
  return <SignInPage {...props} onSuccess={async (username, password) => { await props.onSuccess(username, password); if (initialized) navigate({ name: 'setup' }); }} notice={initialized ? t('setup.initialized') : props.notice} mode={mode} localLoginAllowed={localLoginAllowed} />;
}
