#!/usr/bin/env bash
# Runner for the section 5.4 empirical contract verification (docs/roadmap-backend.md).
#
# This script owns the deterministic half of the harness: it boots the API on a
# free port, waits until the liveness probe answers, proves the local database was
# recreated for this run, prints the readiness facts a verifier needs, and always
# tears down the instance it started. The judgement half — reading the spec
# through `api-tools`, exercising the case matrix in section 5.2 and returning a
# verdict per cell — belongs to the read-only `api-contract-verifier` subagent,
# which uses this script instead of hand-rolling process management.
#
# Booting the API *is* the local reset: the binary drops the target database,
# creates it again, applies every context's `schema/*.sql` in lexical order and
# loads the seed under `core/dev/` (docs/design-identity-ddl.md, *Development
# reset and seed*). The reset is unguarded by an explicit, re-confirmed decision,
# so running this script destroys the local development database; that is the
# point of the harness, not a side effect. A verdict is only meaningful against a
# database that was recreated for the run, so the runner fingerprints the target
# database read-only (its OID) before boot and after readiness, reports both, and
# refuses to call the instance ready when the fingerprint did not change.
#
# Usage:
#   scripts/verify-contract.sh            # boot, report, keep running until Ctrl-C
#   scripts/verify-contract.sh --check    # boot, report readiness, tear down, exit
#
# Exit codes: 0 ready, 1 boot failed, readiness timed out, or the boot did not
# recreate the database, 2 usage error.
set -euo pipefail

PORT="${PORT:-8080}"
BASE_URL="http://localhost:${PORT}"
HEALTH_PATH="/health"
READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-60}"
# The reset target is whatever the API connects to, so the fingerprint reads the
# same URL the binary reads. The runner supplies a local fallback for check/boot
# convenience when the caller environment does not export DATABASE_URL or JWT_SECRET.
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable}"
JWT_SECRET="${JWT_SECRET:-dev-only-jwt-secret-must-be-at-least-32-bytes}"
CHECK_ONLY=0

case "${1:-}" in
  --check) CHECK_ONLY=1 ;;
  "") ;;
  *) echo "usage: $0 [--check]" >&2; exit 2 ;;
esac

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CORE_DIR="${REPO_ROOT}/core"
LOG_FILE="$(mktemp -t canchalibre-api.XXXXXX.log)"
# `go run` executes the server as a grandchild, so killing the wrapper leaves the
# real listener alive holding the port. The runner therefore builds a binary and
# tracks its PID directly: the PID it kills is the process that listens.
API_BIN="$(mktemp -d -t canchalibre-api-bin.XXXXXX)/api"
API_PID=""
PRE_FP=""
POST_FP=""

log() { printf '%s %s\n' "[verify-contract]" "$*"; }

cleanup() {
  if [[ -n "${API_PID}" ]] && kill -0 "${API_PID}" 2>/dev/null; then
    log "stopping the API instance it started (pid ${API_PID})"
    kill "${API_PID}" 2>/dev/null || true
    for _ in $(seq 1 15); do
      kill -0 "${API_PID}" 2>/dev/null || break
      sleep 1
    done
    if kill -0 "${API_PID}" 2>/dev/null; then
      log "instance ignored SIGTERM; sending SIGKILL"
      kill -9 "${API_PID}" 2>/dev/null || true
    fi
    wait "${API_PID}" 2>/dev/null || true
  fi
  [[ -n "${API_BIN}" ]] && rm -rf "$(dirname "${API_BIN}")"
  [[ -n "${LOG_FILE}" ]] && rm -f "${LOG_FILE}"
}
trap cleanup EXIT INT TERM

port_busy() {
  # Anything already listening on the port is reused, never killed: it may not be ours.
  (exec 3<>"/dev/tcp/127.0.0.1/${PORT}") 2>/dev/null && exec 3<&- && exec 3>&- && return 0
  return 1
}

# Read-only fingerprint of the database the API targets. The reset drops and
# recreates that database, and PostgreSQL assigns a fresh OID to the recreated
# one, so an OID that survives the boot means the database was not recreated and
# the instance is not a valid verification target. Nothing here writes: the probe
# is a single SELECT. With no `psql` client on PATH the evidence is reported as
# unavailable rather than guessed, and never as passed.
db_fingerprint() {
  local oid=""
  if command -v psql >/dev/null 2>&1; then
    oid="$(PGCONNECT_TIMEOUT=5 psql "${DATABASE_URL}" --no-psqlrc --no-password --tuples-only --no-align \
            --command 'SELECT oid FROM pg_database WHERE datname = current_database()' 2>/dev/null || true)"
  elif command -v docker >/dev/null 2>&1; then
    oid="$(docker run --rm -i --network host -e PGCONNECT_TIMEOUT=5 postgres:17-alpine \
            psql "${DATABASE_URL}" --no-psqlrc --no-password --tuples-only --no-align \
            --command 'SELECT oid FROM pg_database WHERE datname = current_database()' 2>/dev/null || true)"
  else
    printf 'unavailable (no psql client or docker on PATH)'
    return 0
  fi
  oid="$(printf '%s' "${oid}" | tr -d '[:space:]')"
  if [[ -n "${oid}" ]]; then
    printf 'oid=%s' "${oid}"
  else
    printf 'absent-or-unreachable'
  fi
}

# The connection string may carry a password, so never log it whole.
db_label() {
  printf '%s' "${DATABASE_URL%%\?*}" | sed -E 's#://[^@/]*@#://<redacted>@#'
}

# pre-boot fingerprint | post-boot fingerprint | reused flag -> reset verdict.
reset_verdict() {
  if [[ "$3" -eq 1 ]]; then
    printf 'unavailable (reused instance: this run booted nothing, so no reset can be attributed to it)'
    return 0
  fi
  case "$1:$2" in
    oid=*:oid=*) if [[ "$1" == "$2" ]]; then printf 'NOT recreated'; else printf 'recreated'; fi ;;
    absent-or-unreachable:oid=*) printf 'recreated (the target database did not exist before the boot)' ;;
    *) printf 'unavailable (the pre-boot or post-boot fingerprint could not be read)' ;;
  esac
}

if port_busy; then
  log "port ${PORT} is already serving; reusing that instance and NOT managing its lifecycle"
  REUSED=1
else
  REUSED=0
  PRE_FP="$(db_fingerprint)"
  log "database target $(db_label); pre-boot fingerprint: ${PRE_FP}"
  log "booting the API performs the unguarded local reset: the target database is dropped, recreated, re-schemed and reseeded"
  log "building the API from ${CORE_DIR}"
  if ! (cd "${CORE_DIR}" && mise exec -- go build -o "${API_BIN}" ./cmd/api) >"${LOG_FILE}" 2>&1; then
    log "FAILED: the API did not build. Output:"
    tail -n 20 "${LOG_FILE}" >&2
    exit 1
  fi
  log "booting the API on port ${PORT} (log: ${LOG_FILE})"
  PORT="${PORT}" DATABASE_URL="${DATABASE_URL}" JWT_SECRET="${JWT_SECRET}" "${API_BIN}" >"${LOG_FILE}" 2>&1 &
  API_PID=$!
fi

deadline=$(( $(date +%s) + READY_TIMEOUT_SECONDS ))
until curl -sf "${BASE_URL}${HEALTH_PATH}" >/dev/null 2>&1; do
  if [[ "${REUSED}" -eq 0 ]] && ! kill -0 "${API_PID}" 2>/dev/null; then
    log "FAILED: the API exited before becoming ready. Last log lines:"
    tail -n 20 "${LOG_FILE}" >&2
    exit 1
  fi
  if [[ "$(date +%s)" -ge "${deadline}" ]]; then
    log "FAILED: readiness timed out after ${READY_TIMEOUT_SECONDS}s at ${BASE_URL}${HEALTH_PATH}"
    [[ "${REUSED}" -eq 0 ]] && tail -n 20 "${LOG_FILE}" >&2
    exit 1
  fi
  sleep 1
done

log "ready at ${BASE_URL}"

# Readiness facts the verifier reports verbatim. The spec itself is read through
# `api-tools` (api_swagger), not here: this script only proves the process answers.
log "liveness: $(curl -sf "${BASE_URL}${HEALTH_PATH}")"

# Reset evidence. Booting the API performs the unguarded local reset, so the
# fingerprint must have changed for this run's verdict to describe freshly
# recreated data. Both the pre-boot and post-boot values are printed because the
# verifier's report has to state the evidence, not just a boolean.
if [[ "${REUSED}" -eq 0 ]]; then
  POST_FP="$(db_fingerprint)"
fi
RESET_VERDICT="$(reset_verdict "${PRE_FP}" "${POST_FP}" "${REUSED}")"
log "reset: unguarded DROP DATABASE + CREATE DATABASE + schema/*.sql + core/dev seed, performed by the booted API binary"
log "reset evidence: pre-boot=${PRE_FP:-none} post-boot=${POST_FP:-none} verdict=${RESET_VERDICT}"
if [[ "${RESET_VERDICT}" == "NOT recreated" ]]; then
  log "FAILED: the database fingerprint is unchanged across the boot, so the local reset did not recreate the database."
  log "        Any verdict reached against this instance would describe stale data; refusing to report readiness."
  exit 1
fi
if [[ "${RESET_VERDICT}" == unavailable* ]]; then
  log "WARNING: database recreation is not proven for this run; report the reset evidence as unavailable, never as passed"
fi

if [[ "${CHECK_ONLY}" -eq 1 ]]; then
  # The lint gate of section 5 applies to every endpoint before its slice can
  # close, so the runner enforces it instead of leaving it as a manual step.
  SPEC_FILE="$(mktemp -t canchalibre-spec.XXXXXX.json)"
  if ! curl -sf "${BASE_URL}/openapi.json" -o "${SPEC_FILE}"; then
    log "FAILED: the spec was not readable at ${BASE_URL}/openapi.json"
    exit 1
  fi
  log "linting the spec at ${BASE_URL}/openapi.json against vacuum.ruleset.yaml"
  if (cd "${REPO_ROOT}" && mise exec -- vacuum lint -r vacuum.ruleset.yaml \
        --fail-severity error --min-score 98 "${SPEC_FILE}"); then
    log "lint gate passed"
  else
    log "FAILED: the lint gate rejected the spec"
    rm -f "${SPEC_FILE}"
    exit 1
  fi
  rm -f "${SPEC_FILE}"

  log "check complete"
  exit 0
fi

log "instance is up; the verifier should now drive the case matrix. Ctrl-C to stop."
wait "${API_PID}" 2>/dev/null || true
