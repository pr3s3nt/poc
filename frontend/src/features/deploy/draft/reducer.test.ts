import { describe, expect, it } from 'vitest';
import { deployDraftReducer, emptyDraft, validateDraft } from './reducer';

describe('deployDraftReducer', () => {
  it('selects an application together with its first environment', () => {
    const state = deployDraftReducer(emptyDraft, {
      type: 'application-selected',
      applicationKey: 'acceptance',
      environmentKey: 'dev',
    });
    expect(state).toMatchObject({ applicationKey: 'acceptance', environmentKey: 'dev' });
  });

  it('replaces the score document when the workload changes', () => {
    const state = deployDraftReducer(emptyDraft, {
      type: 'workload-selected',
      workloadId: 'backend',
      scoreText: '{"metadata":{"name":"backend"}}',
    });
    expect(state.workloadId).toBe('backend');
    expect(state.scoreText).toContain('backend');
  });
});

describe('validateDraft', () => {
  const valid = {
    applicationKey: 'acceptance',
    environmentKey: 'dev',
    workloadId: 'backend',
    scoreText: JSON.stringify({ apiVersion: 'score.dev/v1b1', metadata: { name: 'backend' } }),
  };

  it('accepts a complete draft', () => {
    expect(validateDraft(valid).valid).toBe(true);
  });

  it('reports missing selections', () => {
    const result = validateDraft(emptyDraft);
    expect(result.valid).toBe(false);
    expect(result.errors).toContain('Select an application.');
    expect(result.errors).toContain('Score document is empty.');
  });

  it('reports invalid JSON', () => {
    const result = validateDraft({ ...valid, scoreText: '{not json' });
    expect(result.valid).toBe(false);
    expect(result.errors.join(' ')).toContain('not valid JSON');
  });

  it('reports a workload name mismatch', () => {
    const result = validateDraft({ ...valid, workloadId: 'worker' });
    expect(result.valid).toBe(false);
    expect(result.errors.join(' ')).toContain('worker');
  });
});
