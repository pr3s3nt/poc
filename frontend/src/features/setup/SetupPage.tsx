import { useCallback, useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { PlatformSettingsPage } from '../platform/PlatformSettingsPage';
import { DriverAccountsPage } from '../platform/accounts/DriverAccountsPage';
import { SecretStoresPage } from '../platform/SecretStoresPage';
import { CDProvidersPage } from '../delivery/CDProvidersPage';
import { GitInit } from './GitInit';

type Progress = { git: boolean; kubernetes: boolean; vault: boolean; cd: boolean; types: number; definitions: number };
const steps = ['git', 'kubernetes', 'vault', 'cd', 'catalog'] as const;
type Step = typeof steps[number];
export function SetupPage() {
  const [step, setStep] = useState<Step>('git');
  const [progress, setProgress] = useState<Progress>();
  const [error, setError] = useState('');
  const refresh = useCallback(async () => {
    setError('');
    try { setProgress(await api<Progress>('/platform/setup')); }
    catch (err) { setError(describeError(err, t('setup.failed'))); }
  }, []);
  useEffect(() => { void refresh(); }, [refresh, step]);
  const complete = progress && progress.git && progress.kubernetes && progress.vault && progress.cd && progress.types > 0 && progress.definitions > 0;
  return <div className="page setup-page"><header className="page-header"><div><h1>{t('setup.title')}</h1></div><Button tone="secondary" onClick={() => void refresh()}>{t('setup.refresh')}</Button></header>
    {error ? <p role="alert" className="form-error">{error}</p> : null}
    <nav aria-label={t('setup.steps')} className="setup-steps">{steps.map((name, index) => <Button key={name} tone={step === name ? 'primary' : 'secondary'} aria-current={step === name ? 'step' : undefined} onClick={() => setStep(name)}>{index + 1}. {t(`setup.step.${name}`)} · {t(progress ? name === 'catalog' ? progress.types > 0 && progress.definitions > 0 ? 'setup.configured' : 'setup.missing' : progress[name] ? name === 'git' ? 'setup.configured' : 'setup.verified' : 'setup.missing' : 'setup.loading')}</Button>)}</nav>
    <p className="form-info" role="status">{t(complete ? 'setup.complete' : 'setup.incomplete')}</p>
    {step === 'git' ? <PlatformSettingsPage /> : null}
    {step === 'kubernetes' ? <DriverAccountsPage /> : null}
    {step === 'vault' ? <SecretStoresPage /> : null}
    {step === 'cd' ? <CDProvidersPage /> : null}
    <div hidden={step !== 'catalog'}><GitInit onImported={() => void refresh()} /></div>
    {progress ? <p>{t('setup.counts', { types: progress.types, definitions: progress.definitions })}</p> : null}
    <details><summary>{t('setup.help')}</summary><p>{t('setup.prerequisites')}</p><p>{t('setup.retained')}</p></details>
  </div>;
}
