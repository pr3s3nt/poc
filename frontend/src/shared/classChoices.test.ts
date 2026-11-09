import { describe, expect, it } from 'vitest';
import { autoClass, classChoiceMissing, offeredClasses } from './classChoices';
import { newProvisionRow, rowsToProvision } from '../features/platform/typedProvision';

// UC-16 BR-33 (issue #61).
describe('class choices', () => {
  it('offers Default only when served and keeps it when nothing is published', () => {
    expect(offeredClasses({ classes: ['b', 'a'], defaultAvailable: false }, '')).toEqual(['a', 'b']);
    expect(offeredClasses({ classes: ['b', 'a'], defaultAvailable: true }, '')).toEqual(['', 'a', 'b']);
    expect(offeredClasses({ classes: ['default', 'a'] }, 'default')).toEqual(['default', 'a']);
    expect(offeredClasses({ classes: [], defaultAvailable: false }, '')).toEqual(['']);
    expect(offeredClasses(undefined, 'default')).toEqual(['default']);
  });

  it('picks the only choice and reports a missing pick', () => {
    expect(autoClass({ classes: ['git-dev'] }, '')).toBe('git-dev');
    expect(autoClass({ classes: ['a', 'b'] }, '')).toBeUndefined();
    expect(autoClass({ classes: [], defaultAvailable: true }, '')).toBe('');
    expect(classChoiceMissing({ classes: ['a', 'b'] }, '', '')).toBe(true);
    expect(classChoiceMissing({ classes: ['a', 'b'] }, '', 'a')).toBe(false);
    expect(classChoiceMissing({ classes: ['a'], defaultAvailable: true }, '', '')).toBe(false);
  });

  it('blocks a companion rule whose class is still to be chosen', () => {
    const row = { ...newProvisionRow(), descriptor: 'postgres.default#environments.@app.@env', params: '{}' };
    expect(rowsToProvision([{ ...row, classPending: true }]).problem).toBe('Chọn class cho postgres: catalog không phục vụ class mặc định.');
    expect(rowsToProvision([row]).value).toEqual({ 'postgres.default#environments.@app.@env': {} });
  });
});
