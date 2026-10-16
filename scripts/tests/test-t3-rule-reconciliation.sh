#!/usr/bin/env bash
# Regression gate for ODD block T3 (odd/tasks/t3-rule-reconciliation.md).
#
# Locks the two user rules the block applied so a later edit cannot quietly
# reintroduce a migration tool, drop a schema element, smuggle a guard into the
# unguarded reset, or make the adversarial verifier writable.
#
# Design note, learned the hard way: these checks assert INVARIANTS (a tool is
# absent, a path is the new one, no guard exists), never vocabulary. A check of
# the form "this string must never appear" cannot tell "use migrations/" from
# "no migrations/" and fails against the very negation the rule requires.
#
# Trap, learned by falsifying this file: never write `producer | grep -q` here.
# `grep -q` exits on the first match without draining the pipe, the producer dies
# with SIGPIPE (141), and `set -o pipefail` turns that into a pipeline failure — so
# a check written that way fails always, and a negated one silently always passes.
# Use `grep -c` (which drains input) or a single anchored grep instead.
#
# Usage: bash scripts/tests/test-t3-rule-reconciliation.sh
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
fail=0
chk() { if ! eval "$2"; then echo "FAIL[$1] $2"; fail=1; fi; }

# --- S1: no migration tool exists, is depended on, or is prescribed.
chk S1 '[ "$(git ls-files | grep -cE "(^|/)migrations/")" -eq 0 ]'
chk S1 '! grep -rqE "^-- \+goose (Up|Down)" docs/ .agents/skills/ scripts/ .pi/agents/'
chk S1 '! ls core/go.mod >/dev/null 2>&1 || ! grep -qi goose core/go.mod'
chk S1 '! grep -qE "^## Migration" docs/design/identity-ddl.md'
chk S1 'grep -q "adapters/outbound/postgres/schema/" docs/design/identity-ddl.md'
chk S1 'grep -q "adapters/outbound/postgres/schema/" docs/design/platform-module-layout.md'
chk S1 '! grep -qE "postgres/migrations/" docs/design/identity-ddl.md docs/design/platform-module-layout.md'
chk S1 'grep -qi "Reset & Schema Standards" .agents/skills/database-postgres-patterns/SKILL.md'

# --- S1/S5: the frozen schema elements are still present by name.
for el in account_email_kind_key account_status_idx account_platform_role_is_work \
  invitation_scope_shape invitation_consumption_shape membership_role_scope \
  membership_branch_in_business membership_guard_idx membership_one_active_owner_key \
  membership_one_active_employee_key branch_id_business_key; do
  chk S1 "grep -qE '(^|[^A-Za-z0-9_])$el([^A-Za-z0-9_]|\$)' docs/design/identity-ddl.md"
done

# --- S5: reset is DROP+CREATE, binary-triggered, unguarded, and the risk is recorded.
chk S5 'grep -q "DROP DATABASE" docs/design/identity-ddl.md && grep -q "CREATE DATABASE" docs/design/identity-ddl.md'
chk S5 'grep -qi "every start of the API binary" docs/design/identity-ddl.md'
chk S5 'grep -qi "Accepted risk: the reset has no guard" docs/design/identity-ddl.md'
chk S5 'grep -qi "no environment check" docs/design/identity-ddl.md'
chk S5 'grep -qi "lexical order" docs/design/identity-ddl.md'
chk S5 '! grep -qiE "guarded by|if .*env.*production" docs/design/identity-ddl.md'

# --- S3/S4: section 5.4 of docs/endpoint-acceptance-criteria.md defines the
# adversarial mandate; section 5.1 item 7 resolves into it.
chk S3 'grep -qi "Adversarial verification" docs/endpoint-acceptance-criteria.md'
chk S3 'grep -qi "SQL-injection" docs/endpoint-acceptance-criteria.md'
chk S3 'grep -qi "normalization" docs/endpoint-acceptance-criteria.md'
chk S3 'grep -qiE "refusal.*confirmed against the database|rejected input leaves stored data unchanged" docs/endpoint-acceptance-criteria.md'
chk S4 'grep -qi "adversarial verification defined in section 5.4" docs/endpoint-acceptance-criteria.md'

# --- S4: the 100% gate is stated in all three binding places.
chk S4 'grep -qi "gate is \*\*100%\*\*" .pi/agents/api-contract-verifier.md'
chk S4 'grep -q "passes at 100%" docs/endpoint-acceptance-criteria.md'
chk S4 'grep -q "closes only at 100%" docs/endpoint-acceptance-criteria.md'

# --- S3: the agent stays read-only, keeps its nine tools, and has one DB source.
chk S3 '! grep -qE "^  - (edit|write)$" .pi/agents/api-contract-verifier.md'
chk S3 '[ "$(sed -n "/^tools:/,/^---/p" .pi/agents/api-contract-verifier.md | grep -c "^  - ")" -eq 9 ]'
chk S5 'grep -q "DATABASE_URL" .pi/agents/api-contract-verifier.md'
chk S5 'grep -q "DATABASE_URL" core/internal/platform/config/config.go'
chk S5 '! grep -qE "\"database\"|database\":" .pi/api.json.example .pi/api.json.local.example'
chk S5 '! grep -rq "api.json.local" .pi/extensions/api-tools/src/'

# --- S5: the harness keeps its guarantees and carries the reset evidence.
chk S5 'bash -n scripts/verify-contract.sh'
chk S5 'grep -q "db_fingerprint" scripts/verify-contract.sh'
chk S5 'grep -q "reset evidence" scripts/verify-contract.sh'

bash scripts/check-inventory-consistency.sh
chk S1 '[ "$(sed -n "/^| Operation ID | Method and path/,/^$/p" docs/endpoint-acceptance-criteria.md | grep -cE "^\| \`[a-zA-Z]+\`")" -eq 17 ]'
chk S1 'grep -q "inviteSupportOperator" docs/endpoint-acceptance-criteria.md && grep -q "revokeSupportOperator" docs/endpoint-acceptance-criteria.md'
chk S1 'grep -q "password_recovery_token" docs/design/identity-ddl.md'
chk S1 'grep -qi "argon2id" docs/design/identity-ddl.md && grep -qi "12 characters" docs/design/identity-endpoints.md'
if [ "$fail" -eq 0 ]; then echo "ALL T3 CHECKS PASS"; else echo "T3 CHECKS FAILED"; exit 1; fi
