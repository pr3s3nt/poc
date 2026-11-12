import { useEffect, useState } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { SignInPage } from './SignInPage';

export function SignInEntry(props: { onSuccess(username: string, password: string): Promise<void>; notice?: string }) {
  const [mode, setMode] = useState<'local' | 'oidc' | 'loading' | 'error'>('loading');
  const [localLoginAllowed, setLocalLoginAllowed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    api<{ mode: string; localLoginAllowed?: boolean }>('/auth/config').then((result) => {
      if (active) { setMode(result.mode === 'local' || result.mode === 'oidc' ? result.mode : 'error'); setLocalLoginAllowed(result.localLoginAllowed === true); }
    }).catch(() => { if (active) setMode('error'); });
    return () => { active = false; };
  }, [attempt]);
  if (mode === 'loading') return <div className="page" role="status">{t('app.app.loadingOrchestrator')}</div>;
  if (mode === 'error') return <div className="page"><p role="alert">{t('auth.sso.configError')}</p><Button onClick={() => { setMode('loading'); setAttempt(attempt + 1); }}>{t('app.app.retry')}</Button></div>;
  return <SignInPage {...props} mode={mode} localLoginAllowed={localLoginAllowed} />;
}
