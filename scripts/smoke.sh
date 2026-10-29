#!/usr/bin/env bash
# Smoke test for the compose dev stack: every service answers its health probe.
# Usage: scripts/smoke.sh [--stub]   (--stub also checks stub-mdemg)
set -euo pipefail

STUB=0
[[ "${1:-}" == "--stub" ]] && STUB=1

CORE="http://127.0.0.1:${PM_CORE_PORT:-8080}"
WORKERS="http://127.0.0.1:${PM_WORKERS_PORT:-8081}"
SIDECAR="http://127.0.0.1:${PM_SIDECAR_HTTP_PORT:-7070}"
FRONTEND="http://127.0.0.1:${PM_FRONTEND_PORT:-3100}"
IDPI="http://127.0.0.1:${PM_IDPI_HTTP_PORT:-7081}"
SIGI="http://127.0.0.1:${PM_SIGI_HTTP_PORT:-7091}"
COMMSI="http://127.0.0.1:${PM_COMMSI_HTTP_PORT:-7101}"
ERPI="http://127.0.0.1:${PM_ERPI_HTTP_PORT:-7111}"
STUBURL="http://127.0.0.1:${PM_STUB_MDEMG_PORT:-9997}"
TIMEOUT="${SMOKE_TIMEOUT:-180}"

wait_for() {
  local name="$1" url="$2" deadline=$((SECONDS + TIMEOUT))
  until curl -fsS -m 3 "$url" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "FAIL  $name did not answer at $url within ${TIMEOUT}s" >&2
      return 1
    fi
    sleep 2
  done
  echo "ok    $name  $url"
}

fail=0
wait_for pm-core     "$CORE/healthz"           || fail=1
wait_for pm-core     "$CORE/readyz"            || fail=1
wait_for pm-workers  "$WORKERS/healthz"        || fail=1
wait_for pm-sidecar  "$SIDECAR/healthz"        || fail=1
wait_for pm-frontend "$FRONTEND/api/healthz"   || fail=1
wait_for idpi-oidc   "$IDPI/healthz"           || fail=1
wait_for idpi-oidc   "$IDPI/readyz"            || fail=1   # ready = Keycloak discovery answered
wait_for sigi-stub   "$SIGI/healthz"           || fail=1
wait_for commsi-smtp "$COMMSI/readyz"          || fail=1   # ready = Mailpit's SMTP answered
wait_for erpi-stub   "$ERPI/healthz"           || fail=1
# pm-core must see its MDEMG (stub or dedicated stack) through the adapter.
mstat=$(curl -s -m 5 -H 'X-Dev-Actor: smoke' "$CORE/api/v1/mdemg/status" | sed -n 's/.*"reachable":\([a-z]*\).*/\1/p')
if [[ "$mstat" != "true" ]]; then echo "FAIL  pm-core cannot reach its MDEMG (reachable=$mstat)" >&2; fail=1; else echo "ok    pm-core -> mdemg reachable"; fi
if (( STUB )); then
  wait_for stub-mdemg "$STUBURL/healthz"       || fail=1
  # The stub must refuse an unauthenticated classify and accept a keyed one.
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$STUBURL/v1/jiminy/classify" -d '{"space_id":"smoke"}')
  if [[ "$code" != "401" ]]; then echo "FAIL  stub-mdemg accepted an unauthenticated classify (HTTP $code)" >&2; fail=1; else echo "ok    stub-mdemg rejects missing key"; fi
  verdict=$(curl -fsS -X POST "$STUBURL/v1/jiminy/classify" -H "X-API-Key: ${MDEMG_API_KEY:-ci-stub-key}" -d '{"space_id":"smoke","session_id":"smoke","agent_output":"stub:deny"}' | sed -n 's/.*"verdict":"\([a-z]*\)".*/\1/p')
  if [[ "$verdict" != "deny" ]]; then echo "FAIL  stub-mdemg canned deny not returned (got '$verdict')" >&2; fail=1; else echo "ok    stub-mdemg canned deny"; fi
fi

if (( fail )); then
  echo "smoke: FAILED" >&2
  exit 1
fi
echo "smoke: all services healthy"
