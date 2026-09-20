# Fixture validation

Validation performed for this bundle:

- parsed all YAML and JSON artifacts;
- checked all manifest-relative paths;
- checked that before/after Score names identify at most one workload;
- checked every Resource Definition uses `entity.humanitec.io/v1b1` with `entity.criteria`;
- checked every Terraform source mapping resolves to at least one `.tf` file;
- checked there is no `contract.json`;
- compared component YAML with `expected/result.json`;
- applied every accepted Deployment Delta and verified it equals the expected Candidate Deployment Set;
- validated 33 cases: 27 accepted and 6 rejected.

Run again with:

```bash
python validate_fixtures.py
```

The environment used to assemble the bundle did not contain a Go toolchain, so the starter and grader were reviewed as source but not compiled in that environment.

