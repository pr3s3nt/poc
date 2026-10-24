import { expect, it } from 'vitest';
import { dependencyAliasesProblem, emptyDependency } from './DependencyFields';

it('rejects duplicate, invalid and colliding aliases before Score serialization', () => {
  const dependency = { ...emptyDependency(), alias: 'db', type: 'postgres' };
  expect(dependencyAliasesProblem([dependency, { ...dependency }])).toBeDefined();
  expect(dependencyAliasesProblem([{ ...dependency, alias: '' }])).toBeDefined();
  expect(dependencyAliasesProblem([{ ...dependency, alias: 'env' }])).toBeDefined();
  expect(dependencyAliasesProblem([dependency], ['db'])).toBeDefined();
  expect(dependencyAliasesProblem([dependency])).toBeUndefined();
  // Imported aliases are preserved if they do not actually collide.
  expect(dependencyAliasesProblem([{ ...dependency, alias: 'svc_database' }])).toBeUndefined();
});
