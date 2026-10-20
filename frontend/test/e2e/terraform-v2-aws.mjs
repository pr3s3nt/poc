// AWS evidence helpers shared by the Definition-driven AWS scenarios (V05, A02, A03: terraform-v2-aws-definitions.mjs). The scenario
// itself, its catalog (backend/test/integration/v2run/testdata/definition-catalog.json) and its planner contract tests
// (backend/internal/planning/definition_catalog_*_test.go) have no Environment profile, Connection or binding. These helpers only
// parse run-tagged ARNs into manifest assets, prove a Preview created nothing and verify the cleanup absence layer.
/** Parses the run-tagged ARNs into manifest assets of the exact owner chain. */
export function assetsFromArns(arns) {
  const out = [];
  for (const arn of arns) {
    let m;
    if ((m = /^arn:aws:ec2:[^:]+:\d+:vpc\/(vpc-[0-9a-f]+)$/.exec(arn))) out.push({ kind: 'vpc', id: m[1], layer: 'vpc' });
    else if ((m = /^arn:aws:eks:[^:]+:\d+:cluster\/([A-Za-z0-9-_]+)$/.exec(arn))) out.push({ kind: 'eks-cluster', id: m[1], layer: 'eks' });
    else if ((m = /^arn:aws:rds:[^:]+:\d+:cluster:([a-z0-9-]+)$/.exec(arn))) out.push({ kind: 'rds-cluster', id: m[1], layer: 'app-resources' });
    else if ((m = /^arn:aws:elasticache:[^:]+:\d+:replicationgroup:([a-z0-9-]+)$/.exec(arn))) out.push({ kind: 'elasticache-group', id: m[1], layer: 'app-resources' });
    else if ((m = /^arn:aws:ecr:[^:]+:\d+:repository\/(.+)$/.exec(arn))) out.push({ kind: 'ecr-repository', id: m[1], layer: 'aux' });
    else if ((m = /^arn:aws:iam::\d+:role\/(.+)$/.exec(arn))) out.push({ kind: 'iam-role', id: m[1], layer: 'aux' });
  }
  return out;
}

/** Keeps the preview gate's independent proofs visible without exposing AWS keys. */
export function previewPurityProof({ baseline, expectedArns, expectedAssets, ownedKeys, afterPreview, deploymentCount, resourceCount, panelText, accessKeyId, secretAccessKey }) {
  const sorted = (values) => [...values].sort();
  const exactPreparation = expectedArns.length === 3
    && JSON.stringify(sorted(baseline)) === JSON.stringify(sorted(expectedArns))
    && expectedAssets.length === 3
    && expectedAssets.every((asset) => asset?.kind === 'ecr-repository' && ownedKeys.has(`${asset.kind}\t${asset.id}`));
  const unchanged = JSON.stringify(sorted(afterPreview)) === JSON.stringify(sorted(baseline));
  const stateFree = deploymentCount === 0 && resourceCount === 0;
  // Preserve the established broad credential-text guard, including configured key values.
  const noKeyMaterial = !/AKIA|secret/i.test(panelText.replace(/bí mật/g, ''))
    && !(accessKeyId && panelText.includes(accessKeyId))
    && !(secretAccessKey && panelText.includes(secretAccessKey));
  return { exactPreparation, unchanged, stateFree, noKeyMaterial };
}

/** Runs the authoritative fresh layer verifier and accepts only its exact success record. */
export function verifyCleanupAWSAbsence(v2ctl, input) {
  const proof = v2ctl(input, 'aws-verify', '-config', input.configFile, '-manifest', input.manifestFile, '-layer', 'aux');
  if (proof !== 'layer aux verified absent\n') {
    throw new Error('AWS auxiliary resources are not verified absent');
  }
  return 'layer aux verified absent';
}
