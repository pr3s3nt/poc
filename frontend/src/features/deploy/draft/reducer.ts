// Deploy form state. Validation here is a fast feedback loop only; the Go
// backend stays the authority on business rules.
export interface DeployDraft {
  readonly applicationKey: string;
  readonly environmentKey: string;
  readonly workloadId: string;
  readonly scoreText: string;
}

export type DeployDraftAction =
  | { readonly type: 'application-selected'; readonly applicationKey: string; readonly environmentKey: string }
  | { readonly type: 'environment-selected'; readonly environmentKey: string }
  | { readonly type: 'workload-selected'; readonly workloadId: string; readonly scoreText: string }
  | { readonly type: 'score-edited'; readonly scoreText: string };

export const emptyDraft: DeployDraft = {
  applicationKey: '',
  environmentKey: '',
  workloadId: '',
  scoreText: '',
};

export function deployDraftReducer(state: DeployDraft, action: DeployDraftAction): DeployDraft {
  switch (action.type) {
    case 'application-selected':
      return { ...state, applicationKey: action.applicationKey, environmentKey: action.environmentKey };
    case 'environment-selected':
      return { ...state, environmentKey: action.environmentKey };
    case 'workload-selected':
      return { ...state, workloadId: action.workloadId, scoreText: action.scoreText };
    case 'score-edited':
      return { ...state, scoreText: action.scoreText };
    default:
      return state;
  }
}

export interface DraftValidation {
  readonly valid: boolean;
  readonly errors: readonly string[];
  readonly score: unknown;
}

export function validateDraft(draft: DeployDraft): DraftValidation {
  const errors: string[] = [];
  if (draft.applicationKey === '') {
    errors.push('Select an application.');
  }
  if (draft.environmentKey === '') {
    errors.push('Select an environment.');
  }
  if (draft.workloadId === '') {
    errors.push('Select a workload.');
  }
  let score: unknown = null;
  if (draft.scoreText.trim() === '') {
    errors.push('Score document is empty.');
  } else {
    try {
      score = JSON.parse(draft.scoreText) as unknown;
    } catch (cause) {
      errors.push(`Score document is not valid JSON: ${cause instanceof Error ? cause.message : 'parse error'}`);
    }
  }
  if (score !== null && typeof score === 'object') {
    const metadata = (score as { metadata?: { name?: unknown } }).metadata;
    const name = metadata?.name;
    if (typeof name === 'string' && draft.workloadId !== '' && name !== draft.workloadId) {
      errors.push(`Score describes workload "${name}" but "${draft.workloadId}" is selected.`);
    }
  }
  return { valid: errors.length === 0, errors, score };
}
