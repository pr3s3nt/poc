import { useEffect, useRef, useState } from 'react';
import { fieldNote } from '../../shared/api/errors';
import { api, ApiError, sessionExpiredEvent } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

// Exact single-context matching preview (UC-03 V02-05, OC-04c). The server
// computes it from a read-only snapshot with the same five-field matcher the
// planner uses; nothing is saved. The result belongs to exactly one
// candidate+context (its signature): any input, scope or session change drops
// the result and ignores a reply still in flight. Only this server result may
// name a winner; the browser never ranks Definitions.

export type MatchingCandidateInput = { definition: Record<string, unknown> | null; problem: string };
export type MatchingSample = { applicationId: string; environmentId: string; resourceId: string; class: string };

type Criterion = { env_type?: string; app_id?: string; env_id?: string; res_id?: string; class?: string };
type Winner = {
  key: string; name?: string; version?: number; driverType?: string; score: number;
  account?: { key: string; name: string; type: string; version: number };
  region?: string;
  clusterSource?: { kind: 'RESOURCE' | 'LEGACY_ENVIRONMENT'; reference?: string };
};
type Result = {
  status: 'INCOMPLETE' | 'MATCHED' | 'NO_MATCH' | 'AMBIGUOUS' | 'CONNECTION_CONFLICT';
  missingFields?: string[];
  candidates?: { key: string; name?: string; matched: boolean; bestCriterion?: Criterion; score?: number }[];
  winner?: Winner;
  ties?: string[];
  connectionConflict?: { definitionKey: string; driverAccount?: string; reason: string };
  snapshot?: { marker: string; environmentVersion: number };
};
type State = { phase: 'idle' } | { phase: 'loading' } | { phase: 'result'; result: Result } | { phase: 'error'; message: string };

const statusText: Record<Result['status'], string> = {
  INCOMPLETE: t('platform.matchingPreviewPanel.notEnoughContextToConclude'),
  MATCHED: t('platform.matchingPreviewPanel.exactlyOneDefinitionWins'),
  NO_MATCH: t('platform.matchingPreviewPanel.noDefinitionMatchesDifferentFrom'),
  AMBIGUOUS: t('platform.matchingPreviewPanel.tieSeveralDefinitionsShareThe'),
  CONNECTION_CONFLICT: t('platform.matchingPreviewPanel.theWinningDefinitionHasAn'),
};
const conflictText: Record<string, string> = {
  ACCOUNT_MISSING: t('platform.matchingPreviewPanel.theDefinitionHasNoAccount'),
  ACCOUNT_NOT_FOUND: t('platform.matchingPreviewPanel.accountNotFoundInThe'),
  ACCOUNT_KIND_MISMATCH: t('platform.matchingPreviewPanel.accountOfTheWrongType'),
  ACCOUNT_NOT_READY: t('platform.matchingPreviewPanel.accountNotReadyREADY'),
  REGION_INVALID: t('platform.matchingPreviewPanel.aWSRegionInvalidOrNot'),
};
const fieldText: Record<string, string> = { resourceId: t('platform.matchingPreviewPanel.resourceIDResId'), class: 'Class' };

function describeCriterion(criterion?: Criterion): string {
  const parts = Object.entries(criterion ?? {}).filter(([, value]) => typeof value === 'string' && value !== '').map(([key, value]) => `${key}=${value}`);
  return parts.length === 0 ? t('platform.matchingPreviewPanel.everywhere') : parts.join(', ');
}

// Transport/validation failures are never shown as a matching result.
function describeTransport(reason: unknown): string {
  if (!(reason instanceof ApiError)) return t('platform.matchingPreviewPanel.noResponseFromTheServer');
  switch (reason.status) {
    case 400: return t('platform.matchingPreviewPanel.theServerRejectedTheDefinition') + fieldNote(reason.field);
    case 401: return t('platform.matchingPreviewPanel.yourSessionHasExpiredPlease');
    case 403: return t('platform.matchingPreviewPanel.youAreNotAllowedTo');
    case 404: return t('platform.matchingPreviewPanel.theSampleApplicationOrEnvironment');
    case 413: return t('platform.matchingPreviewPanel.theDefinitionContentIsToo');
    default: return t('platform.matchingPreviewPanel.theServerFailedWhileChecking');
  }
}

export function MatchingPreviewPanel({ candidate, sample, scopeKey = '' }: { candidate: MatchingCandidateInput; sample: MatchingSample; scopeKey?: string }) {
  const [state, setState] = useState<State>({ phase: 'idle' });
  const seq = useRef(0);
  const body = candidate.definition === null ? '' : JSON.stringify({ definition: candidate.definition, context: { applicationId: sample.applicationId, environmentId: sample.environmentId, resourceId: sample.resourceId.trim(), class: sample.class.trim() } });
  const signature = `${scopeKey}\u0000${body}`;

  // Any change of candidate, context or scope invalidates the result and the pending reply.
  useEffect(() => { seq.current += 1; setState({ phase: 'idle' }); }, [signature]);
  useEffect(() => {
    const expire = () => { seq.current += 1; setState({ phase: 'idle' }); };
    window.addEventListener(sessionExpiredEvent, expire);
    return () => { seq.current += 1; window.removeEventListener(sessionExpiredEvent, expire); }; // unmount: ignore any late reply
  }, []);

  const noScope = sample.applicationId === '' || sample.environmentId === '';
  const blocked = candidate.definition === null ? candidate.problem : noScope ? t('platform.matchingPreviewPanel.chooseASampleApplicationAnd') : '';

  async function run() {
    if (blocked || state.phase === 'loading') return;
    const request = ++seq.current;
    setState({ phase: 'loading' });
    try {
      const result = await api<Result>('/resource-definitions/matching-preview', { method: 'POST', body });
      if (request === seq.current) setState({ phase: 'result', result });
    } catch (reason) {
      if (request === seq.current) setState({ phase: 'error', message: describeTransport(reason) });
    }
  }

  const result = state.phase === 'result' ? state.result : null;
  return <div className="criteria-preview matching-preview" role="group" aria-label={t('platform.matchingPreviewPanel.checkExactMatching')}>
    <h4>{t('platform.matchingPreviewPanel.checkExactMatching')}</h4>
    <p className="feature-note">{t('platform.matchingPreviewPanel.theServerComputesOnThe')}</p>
    {blocked ? <p>{blocked}</p> : null}
    <div className="form-actions"><Button type="button" onClick={() => { void run(); }} disabled={blocked !== '' || state.phase === 'loading'}>{t('platform.matchingPreviewPanel.checkMatching')}</Button></div>
    {state.phase === 'loading' ? <p role="status">{t('platform.matchingPreviewPanel.checkingMatching')}</p> : null}
    {state.phase === 'error' ? <div className="form-error" role="alert" aria-label={t('platform.matchingPreviewPanel.matchingCheckError')}>{state.message} <Button type="button" onClick={() => { void run(); }}>{t('platform.matchingPreviewPanel.retryMatchingCheck')}</Button></div> : null}
    {result ? <div role="region" aria-label={t('platform.matchingPreviewPanel.exactMatchingResult')}>
      <p><strong>{statusText[result.status] ?? t('platform.matchingPreviewPanel.unknownStatus')}</strong></p>
      {result.status === 'INCOMPLETE' ? <p>{t('platform.matchingPreviewPanel.contextNeededValueNoConclusion', { value: (result.missingFields ?? []).map((field) => fieldText[field] ?? field).join(', ') })}</p> : null}
      {result.winner ? <div role="group" aria-label={t('platform.matchingPreviewPanel.winningDefinition')}><p>{t('platform.matchingPreviewPanel.winningDefinition2')}{' '} <strong>{result.winner.name || result.winner.key}</strong> (ID <code>{result.winner.key}</code>{result.winner.version ? t('platform.matchingPreviewPanel.versionVersion', { version: result.winner.version }) : ''}{t('platform.matchingPreviewPanel.score')}{' '} {result.winner.score}).</p>
        {result.winner.account ? <p>{t('platform.matchingPreviewPanel.account')}{' '} <strong>{result.winner.account.name}</strong> (<code>{result.winner.account.key}</code>, {result.winner.account.type}{t('platform.matchingPreviewPanel.version')}{' '} {result.winner.account.version}){result.winner.region ? <>{t('platform.matchingPreviewPanel.effectiveRegion')}{' '} <strong>{result.winner.region}</strong></> : null}.</p> : null}
        {result.winner.clusterSource ? <p>{t('platform.matchingPreviewPanel.clusterInheritedFrom')}{' '} {result.winner.clusterSource.kind === 'RESOURCE' ? <>{t('platform.matchingPreviewPanel.resource')}{' '} <code>{result.winner.clusterSource.reference}</code></> : <><strong>{t('platform.matchingPreviewPanel.unsupportedClusterSource')}</strong></>}{t('platform.matchingPreviewPanel.theAccountIsNotChosen')}</p> : null}</div> : null}
      {result.ties && result.ties.length > 0 ? <p>{t('platform.matchingPreviewPanel.definitionsTiedAtTheTop')}{' '} <strong>{result.ties.join(', ')}</strong>{t('platform.matchingPreviewPanel.inThisSnapshotMatchingIs')}</p> : null}
      {result.connectionConflict ? <p role="alert">{t('platform.matchingPreviewPanel.accountConflictOfTheWinning')}{' '} <strong>{result.connectionConflict.definitionKey}</strong>{result.connectionConflict.driverAccount ? t('platform.matchingPreviewPanel.accountDriverAccount', { driverAccount: result.connectionConflict.driverAccount }) : ''}: {conflictText[result.connectionConflict.reason] ?? t('platform.matchingPreviewPanel.unusableAccount')}{t('platform.matchingPreviewPanel.theSystemDoesNotChoose')}</p> : null}
      {(result.candidates ?? []).length > 0 ? <ul aria-label={t('platform.matchingPreviewPanel.detailsPerDefinition')}>{(result.candidates ?? []).map((item) => <li key={item.key}><strong>{item.name || item.key}</strong> (<code>{item.key}</code>): {result.status === 'INCOMPLETE' ? t('platform.matchingPreviewPanel.notEvaluatedDueToMissing') : item.matched ? t('platform.matchingPreviewPanel.matchesScoreValueBestCriterion', { value: item.score ?? 0, describeCriterion: describeCriterion(item.bestCriterion) }) : t('platform.matchingPreviewPanel.doesNotMatchTheContext')}</li>)}</ul> : null}
      {result.snapshot ? <p className="feature-note">{t('platform.matchingPreviewPanel.snapshotSliceEnvironmentVersionEnvironmentVersion', { slice: result.snapshot.marker.slice(0, 12), environmentVersion: result.snapshot.environmentVersion })}</p> : null}
    </div> : null}
  </div>;
}
