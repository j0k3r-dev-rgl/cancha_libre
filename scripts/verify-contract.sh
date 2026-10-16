#!/usr/bin/env bash
# Runner for the section 5.4 empirical contract verification (docs/roadmap-backend.md).
#
# This script owns the deterministic half of the harness: it boots the API on a
# free port, waits until the liveness probe answers, prints the readiness facts a
# verifier needs, and always tears down the instance it started. The judgement
# half — reading the spec through `api-tools`, exercising the case matrix in
# section 5.2 and returning a verdict per cell — belongs to the read-only
# `api-contract-verifier` subagent, which uses this script instead of hand-rolling
# process management.
#
# Usage:
#   scripts/verify-contract.sh            # boot, report, keep running until Ctrl-C
#   scripts/verify-contract.sh --check    # boot, report readiness, tear down, exit
#
# Exit codes: 0 ready, 1 boot failed or readiness timed out, 2 usage error.
set -euo pipefail

PORT="${PORT:-8080}"
BASE_URL="http://localhost:${PORT}"
HEALTH_PATH="/health"
READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-60}"
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

if port_busy; then
  log "port ${PORT} is already serving; reusing that instance and NOT managing its lifecycle"
  REUSED=1
else
  REUSED=0
  log "building the API from ${CORE_DIR}"
  if ! (cd "${CORE_DIR}" && mise exec -- go build -o "${API_BIN}" ./cmd/api) >"${LOG_FILE}" 2>&1; then
    log "FAILED: the API did not build. Output:"
    tail -n 20 "${LOG_FILE}" >&2
    exit 1
  fi
  log "booting the API on port ${PORT} (log: ${LOG_FILE})"
  PORT="${PORT}" "${API_BIN}" >"${LOG_FILE}" 2>&1 &
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
