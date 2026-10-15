import { describe, expect, it } from 'vitest';
import { assetsFromArns, previewPurityProof } from './terraform-v2-aws.mjs';

const repositories = ['frontend', 'backend', 'worker'].map((name) =>
  `arn:aws:ecr:us-east-1:452025861381:repository/poc-m04-a02-20261015-r3-${name}`);
const assets = assetsFromArns(repositories);
const ownedKeys = new Set(assets.map((asset) => `${asset.kind}\t${asset.id}`));
const proof = (overrides = {}) => previewPurityProof({
  baseline: repositories,
  expectedArns: repositories,
  expectedAssets: assets,
  ownedKeys,
  afterPreview: repositories,
  deploymentCount: 0,
  resourceCount: 0,
  panelText: 'Xem trước triển khai · cấu hình an toàn',
  accessKeyId: 'AKIAABCDEFGHIJKLMNOP',
  secretAccessKey: 'synthetic-secret-key-for-test-only',
  ...overrides,
});

describe('AWS preview purity proof', () => {
  it('accepts the production ECRPublication ARN shape and exact manifest-owned repository IDs', () => {
    expect(assets).toEqual(repositories.map((arn) => ({
      kind: 'ecr-repository', id: arn.split('/').at(-1), layer: 'aux',
    })));
    expect(proof()).toEqual({ exactPreparation: true, unchanged: true, stateFree: true, noKeyMaterial: true });
  });

  it('rejects missing, extra, or unacknowledged ECR preparation assets', () => {
    expect(proof({ baseline: repositories.slice(0, 2) }).exactPreparation).toBe(false);
    expect(proof({ expectedArns: [...repositories, repositories[0]] }).exactPreparation).toBe(false);
    expect(proof({ ownedKeys: new Set([...ownedKeys].slice(0, 2)) }).exactPreparation).toBe(false);
  });

  it('requires preview to leave AWS assets and application state unchanged', () => {
    expect(proof({ afterPreview: [...repositories, 'arn:aws:ecr:us-east-1:452025861381:repository/foreign'] }).unchanged).toBe(false);
    expect(proof({ deploymentCount: 1 }).stateFree).toBe(false);
    expect(proof({ resourceCount: 1 }).stateFree).toBe(false);
  });

  it('preserves the established secret-text guard and rejects actual AWS key material', () => {
    expect(proof({ panelText: 'Secret row is optional; bí mật not configured' }).noKeyMaterial).toBe(false);
    expect(proof({ panelText: 'Bí mật not configured' }).noKeyMaterial).toBe(true);
    expect(proof({ panelText: 'preview AKIAABCDEFGHIJKLMNOP' }).noKeyMaterial).toBe(false);
    expect(proof({ panelText: 'preview synthetic-secret-key-for-test-only' }).noKeyMaterial).toBe(false);
  });
});
