#!/usr/bin/env bash
# Proves the outbox path end to end in the running dev stack:
# create a project through pm-core (enqueues an observe in the same transaction),
# then wait for pm-workers to drain it to the MDEMG target (stub or dedicated stack).
set -euo pipefail
CORE="http://127.0.0.1:${PM_CORE_PORT:-8080}"
WORKERS="http://127.0.0.1:${PM_WORKERS_PORT:-8081}"
TIMEOUT="${OUTBOX_TIMEOUT:-90}"
SITE="01900000-0000-7000-8000-00000000a001"   # NF-KY from the seed
code="P-OUTBOX-$(date +%s)"

created=$(curl -fsS -X POST "$CORE/api/v1/projects" -H 'X-Dev-Actor: outbox-check' -H 'Content-Type: application/json' \
  -d "{\"code\":\"$code\",\"name\":\"outbox check\",\"site_id\":\"$SITE\",\"project_type\":\"reliability\"}")
pid=$(printf '%s' "$created" | sed -n 's/.*"project_id":"\([^"]*\)".*/\1/p')
[[ -n "$pid" ]] || { echo "FAIL  could not create project: $created" >&2; exit 1; }
echo "ok    project $code created ($pid)"

deadline=$((SECONDS + TIMEOUT))
while :; do
  stats=$(curl -fsS -H 'X-Dev-Actor: outbox-check' "$CORE/api/v1/outbox/stats")
  pending=$(printf '%s' "$stats" | sed -n 's/.*"pending":\([0-9]*\).*/\1/p')
  delivered=$(printf '%s' "$stats" | sed -n 's/.*"delivered":\([0-9]*\).*/\1/p')
  failed=$(printf '%s' "$stats" | sed -n 's/.*"failed":\([0-9]*\).*/\1/p')
  if [[ "${pending:-1}" == "0" && "${delivered:-0}" -ge 1 ]]; then
    echo "ok    outbox drained: delivered=$delivered failed=$failed"
    curl -fsS "$WORKERS/stats"; echo
    [[ "${failed:-0}" == "0" ]] || { echo "FAIL  outbox has failed rows: $stats" >&2; exit 1; }
    exit 0
  fi
  if (( SECONDS >= deadline )); then
    echo "FAIL  outbox not drained within ${TIMEOUT}s: $stats" >&2
    exit 1
  fi
  sleep 2
done
