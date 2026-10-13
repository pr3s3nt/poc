---
id: VERIFY-2026-10-13-I04-01-PARTIAL
artifact: verification-record
status: evidence
last_reviewed: 2026-10-13
related: [I04-01, IMP-021]
---

# I04-01 — partial implementation, DB security decision pending

Task baseline 567c2bc, clean tree. Claude via task-owned tmux implemented
Go state/identity/attempt/dependency/cipher/HTTP/bootstrap code; no docs or commit.
No production mounting claimed: loopback serving helper is tested, executor
mounting remains I04-03. Cleanup public behavior remains I04-08.

## Review và checks

Codex review fixed operation release-vs-lock race, edge lifecycle guards,
cross-scope cleanup replay and added literal OS-process concurrency evidence.
Claude mutation check of race protection failed with the old ordering and
passed with fix; no mutation left in code.

- Claude: go vet ./..., go test ./... -count=1 -v on real disposable PG,
  go build ./...: PASS; 794 PASS, zero FAIL/SKIP. Logs outside Git in
  `/tmp/poc-m04-i04-01-20261013/`.
- Codex independently: go build ./... PASS; full go test ./... -count=1 -v
  on a second task-owned PostgreSQL16 container PASS, zero required tests skipped.
  Log `/tmp/poc-i0401-codex-review/go-test-all.log`, exit proof `result.txt`.
- Scope includes restart/ciphertext/wrong key, scope/identity, two OS processes,
  stale fence/expiry, serial/lineage/rollback, graph/provenance, admission race.
- No browser video: internal unit/HTTP/SQL task only, no Console flow yet.
- No k8s-4f/cloud mutation or acceptance scenario executed.

## Cleanup và blocking decision

Claude container `poc-i04-01-pg-20261013`, task volume and URL removed;
`cleanup-proof.txt` records zero leftover test databases. Codex container
`poc-i0401-codex-review` stopped/removed; tmpfs database discarded and credential
file unlinked, proof `cleanup.txt`. Retained containers/databases untouched.

DB role currently owns every table; Go interface separation alone does NOT
satisfy terraform-state database permission requirement. User explanation
requested before choosing separate owner/public/state roles + connections,
or SECURITY DEFINER. Neither accepted; no role implementation authorized yet.
Code preserved uncommitted in workspace. I04-01/I04-02 transition not permitted
until this requirement is implemented/reviewed/validated. No acceptance reduced.
Task tmux handoff saved before closing; resume Claude after user decision.
