#!/usr/bin/env bash
# Locks the identity inventory's consistency invariants (T2, section 5.2 of
# docs/endpoint-acceptance-criteria.md).
#
# The defect class this prevents: the inventory rows and the error catalog are two
# tables in two files describing one contract, so they can drift into two
# vocabularies without any single file looking wrong. A cross-check found exactly
# that (rows wrote `validation` while the catalog defined
# `urn:canchalibre:error:validation`). This script makes the check repeatable.
#
# It also fails if an inventory row cites a requirement or acceptance criterion
# that does not exist, because an invented identifier is a fabricated requirement
# — the most expensive kind of documentation defect.
#
# Usage: scripts/check-inventory-consistency.sh
# Exit codes: 0 consistent, 1 inconsistent, 2 a required file is missing.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

CASES=docs/endpoint-acceptance-criteria.md
CATALOG=docs/design/platform-error-model.md
PRD=docs/prd-backend.md
MVP=docs/mvp.md

for f in "$CASES" "$CATALOG" "$PRD" "$MVP"; do
  [[ -f "$f" ]] || { echo "FAIL: missing required file $f" >&2; exit 2; }
done

failures=0
fail() { echo "FAIL: $*" >&2; failures=$((failures + 1)); }

# The frozen inventory block only, so unrelated prose cannot satisfy a check.
inventory="$(sed -n '/^#### Inventory — S1/,/^### 5\.3/p' "$CASES")"
[[ -n "$inventory" ]] || { echo "FAIL: the S1 inventory block was not found in $CASES" >&2; exit 2; }

# 1. Every error slug used by an inventory row must exist in the catalog.
# Matching only the error status codes is what isolates the errors column: success
# cells use 2xx codes only, so a cell such as `204` empty body can never be
# mistaken for an error type. The slug is taken with its optional parenthetical
# explanation ignored.
ERROR_STATUS='400|401|403|404|405|409|415|422|500'
used_slugs="$(printf '%s\n' "$inventory" | grep -oE "\`($ERROR_STATUS)\` [a-z-]+" | awk '{print $2}' | sort -u || true)"
used_count="$(printf '%s\n' "$used_slugs" | grep -c . || true)"
[[ "$used_count" -ge 8 ]] || fail "only $used_count error types found in the inventory; the row format probably changed and this check went blind"
catalog_slugs="$(grep -oE 'urn:canchalibre:error:[a-z-]+' "$CATALOG" | sed 's/.*error://' | sort -u || true)"
[[ -n "$used_slugs" ]] || fail "no error slugs found in the inventory; the row format changed"

undefined="$(comm -23 <(printf '%s\n' "$used_slugs") <(printf '%s\n' "$catalog_slugs"))"
[[ -z "$undefined" ]] || fail "inventory uses error types absent from the catalog: $(echo "$undefined" | tr '\n' ' ')"

# 2. No endpoint row may document about:blank as its own type.
if printf '%s\n' "$inventory" | grep -nE "\`($ERROR_STATUS)\`\s*about:blank" >/dev/null; then
  fail "an inventory row documents about:blank as an error type"
fi
# 3. Every BE-* and AC-* cited in the inventory must exist in its source of truth.
cited_be="$(printf '%s\n' "$inventory" | grep -oE 'BE-[A-Z]+-[0-9]{2}' | sort -u || true)"
defined_be="$(grep -oE '^\| BE-[A-Z]+-[0-9]{2}' "$PRD" | grep -oE 'BE-[A-Z]+-[0-9]{2}' | sort -u || true)"
invented_be="$(comm -23 <(printf '%s\n' "$cited_be") <(printf '%s\n' "$defined_be"))"
[[ -z "$invented_be" ]] || fail "inventory cites BE-* identifiers that do not exist in $PRD: $(echo "$invented_be" | tr '\n' ' ')"

cited_ac="$(printf '%s\n' "$inventory" | grep -oE 'AC-[0-9]{2}' | sort -u || true)"
defined_ac="$(grep -oE '^\| AC-[0-9]{2} \|' "$MVP" | grep -oE 'AC-[0-9]{2}' | sort -u || true)"
invented_ac="$(comm -23 <(printf '%s\n' "$cited_ac") <(printf '%s\n' "$defined_ac"))"
[[ -z "$invented_ac" ]] || fail "inventory cites AC-* identifiers that do not exist in $MVP: $(echo "$invented_ac" | tr '\n' ' ')"

# 4. The two inventory tables must cover exactly the same operations.
table_ops() { sed -n "/^| Operation ID | $1/,/^$/p" "$CASES" | grep -oE '^\| `[a-zA-Z]+`' | tr -d '|` ' | sort -u; }
by_route="$(table_ops 'Method and path')"
by_payload="$(table_ops 'Request shape')"
if [[ -z "$by_route" ]] || [[ -z "$by_payload" ]]; then
  fail "one of the two inventory tables was not found or is empty"
elif ! diff <(printf '%s\n' "$by_route") <(printf '%s\n' "$by_payload") >/dev/null; then
  fail "the two inventory tables do not cover the same operations"
fi

# 5. The route convention must hold: the table lists project-relative paths that
# omit the `/api/v1` prefix, and the inventory states that prefix. Asserting the
# prefix on each row would contradict the documented convention, so both halves
# are checked instead.
if ! printf '%s\n' "$inventory" | grep -qE 'served under the `/api/v1` prefix'; then
  fail "the inventory does not state the /api/v1 prefix its routes omit"
fi
if printf '%s\n' "$inventory" | grep -oE '\`(GET|POST|PUT|PATCH|DELETE) [^`]*\`' | grep -qvE '\`(GET|POST|PUT|PATCH|DELETE) /[a-z]'; then
  fail "an inventory route is not a project-relative path starting with /"
fi
if printf '%s\n' "$inventory" | grep -qE '\`(GET|POST|PUT|PATCH|DELETE) /api/v1/'; then
  fail "an inventory route repeats the /api/v1 prefix the table documents as omitted"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "FAILED: $failures inventory consistency problem(s)." >&2
  exit 1
fi

echo "PASS: inventory consistent — $(printf '%s\n' "$by_route" | wc -l | tr -d ' ') operations, $(printf '%s\n' "$used_slugs" | wc -l | tr -d ' ') error types used, all identifiers resolve."
