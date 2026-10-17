#!/usr/bin/env bash
# V02P local browser scenario: companion-resource ("provision") authoring and
# planner consumption. A Platform Engineer authors a Redis Resource Definition
# whose provision rules create four PostgreSQL companions (one per
# is_dependent / match_dependents combination, nested typed params), round-trips
# the rules through the typed form and the advanced JSON editor, saves (POST),
# reloads, edits (PUT with the expected version), reloads, restarts the product
# server and reads a real Score Preview (graph, consumer -> provider edges,
# provider-first batches) after every stored version. Recorded headed with
# Vietnamese captions below the browser.
#   v02-provision-local.sh --headed --captions vi --evidence NEW_PATH
#
# This is the V02 runner (v02-definitions-local.sh) with --scenario V02P: the
# same environment variables, the same disposable PostgreSQL, the same Vault
# credential store, the same recording and cleanup. See that file for
# ORCH_V02_PG_ADMIN_URL_FILE, ORCH_V02_VAULT_BIN | ORCH_V02_VAULT_ADDR +
# ORCH_V02_VAULT_TOKEN_FILE, ORCH_V02_SCORE_K8S and the evidence layout.
#
# REAL: the Web Console, the HTTP handlers, the planner, PostgreSQL, Vault.
# FAKE (labelled in the captions): the Kubernetes verification of the one
#   fixture Internal account (awsconnserver -fake-kubernetes) that connects the
#   Environment so the Preview can plan. No AWS account, Terraform, cluster or
#   Docker workload is used and nothing is deployed: a PASS proves that stored
#   provision rules reach the PLAN, not that any companion resource exists.
# Extra evidence: previews.json (edge sets and batches of every Preview),
#   kube-calls-*.json, definition-writes.json.
exec "$(dirname "${BASH_SOURCE[0]}")/v02-definitions-local.sh" --scenario V02P "$@"
