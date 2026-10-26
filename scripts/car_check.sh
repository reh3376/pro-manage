#!/usr/bin/env bash
# The Phase 0 exit test (FSD roadmap "Success criteria"), driven through the REST edge of the
# running dev stack: estimate -> committed promise -> full CAR -> guide on open renders a card ->
# preclassify -> submit (classify round-trip, AuditEvent + GuidanceOutcome written) -> every stage
# approved by its pool -> CAR approved -> feedback and observe rows drained by pm-workers.
# Needs the Northfield seed (make db-seed) and a stub or live MDEMG behind pm-core.
set -euo pipefail
CORE="http://127.0.0.1:${PM_CORE_PORT:-8080}"
TIMEOUT="${OUTBOX_TIMEOUT:-90}"
PROJECT="01900000-0000-7000-8000-00000000b001"   # P-NF-KY-7, Still House 2 (FEL3, USD) from the seed
run=$(date +%s)

j() { python3 -c "import json,sys; d=json.load(sys.stdin); print(eval('d'+sys.argv[1]))" "$1"; }
call() { # method path actor [body]
  local m=$1 p=$2 actor=$3 body=${4:-}
  if [[ -n "$body" ]]; then
    curl -sS -X "$m" "$CORE$p" -H "X-Dev-Actor: $actor" -H 'Content-Type: application/json' -d "$body" -w '\n%{http_code}'
  else
    curl -sS -X "$m" "$CORE$p" -H "X-Dev-Actor: $actor" -w '\n%{http_code}'
  fi
}
expect() { # name status response
  local name=$1 want=$2 resp=$3
  local code=${resp##*$'\n'} body=${resp%$'\n'*}
  if [[ "$code" != "$want" ]]; then echo "FAIL  $name: http $code (want $want): $body" >&2; exit 1; fi
  echo "ok    $name ($code)" >&2
  printf '%s' "$body"
}

est=$(expect "estimate created" 201 "$(call POST "/api/v1/projects/$PROJECT/estimates" alice.owner \
  '{"aace_class":3,"as_of":"2026-10-01","prepared_by":"hana.epcm","basis_of_estimate_doc_ref":"BOE-R3","scope_definition_pct":"70","accuracy_range_low_pct":"-15","accuracy_range_high_pct":"20","tic":"42000000.00","contingency_amount":"3500000.00","contingency_method":"risk_based_p50"}')")
est_id=$(printf '%s' "$est" | j "['estimate_id']")

expect "promise committed" 201 "$(call POST "/api/v1/projects/$PROJECT/promises" alice.owner \
  '{"kind":"capacity","metric_id":"proof_gallons_per_year","baseline_value":"0","baseline_period":"n/a (greenfield)","promised_value":"1200000","promised_by_date":"2028-09-30","dollar_value_per_year":"9800000.00","measurement_basis":"TTB proof-gallon production report, trailing 12 months","status":"committed"}')" >/dev/null

car_body=$(printf '{"code":"CR-CHECK-%s","kind":"full","amount_requested":"42000000.00","estimate_id":"%s","business_case":"Still House 2 column still: 1.2M proof gallons per year of new capacity; payback 4.3 years."}' "$run" "$est_id")
car=$(expect "CAR drafted" 201 "$(call POST "/api/v1/projects/$PROJECT/cars" alice.owner "$car_body")")
car_id=$(printf '%s' "$car" | j "['car']['car_id']")

guidance=$(expect "guide on open" 200 "$(call GET "/api/v1/cars/$car_id/guidance" alice.owner)")
cards=$(printf '%s' "$guidance" | j "['guidance']['items'].__len__()")
tier=$(printf '%s' "$guidance" | j "['preview']['tier']")
stages=$(printf '%s' "$guidance" | j "['preview']['chain'].__len__()")
gids=$(printf '%s' "$guidance" | python3 -c "import json,sys; print(json.dumps(json.load(sys.stdin)['guidance']['guidance_ids']))")
echo "      cards=$cards tier=$tier stages=$stages guidance_ids=$gids"
[[ "$cards" -ge 1 ]] || { echo "FAIL  guide on open rendered no card" >&2; exit 1; }
[[ "$tier" == "A" && "$stages" == "4" ]] || { echo "FAIL  expected Tier A with 4 stages for 42M" >&2; exit 1; }

pre=$(expect "preclassify" 200 "$(call POST "/api/v1/cars/$car_id/preclassify" alice.owner)")
[[ "$(printf '%s' "$pre" | j "['verdict']")" == "pass" ]] || { echo "FAIL  preclassify: $pre" >&2; exit 1; }

submit_body=$(printf '{"guidance_ids":%s}' "$gids")
sub=$(expect "submit" 200 "$(call POST "/api/v1/cars/$car_id/submit" alice.owner "$submit_body")")
[[ "$(printf '%s' "$sub" | j "['governance']['cached']")" == "True" ]] || { echo "FAIL  submit did not use the pre-classified verdict: $sub" >&2; exit 1; }
[[ "$(printf '%s' "$sub" | j "['car_view']['car']['status']")" == "in_flight" ]] || { echo "FAIL  not in flight: $sub" >&2; exit 1; }
audit_id=$(printf '%s' "$sub" | j "['governance']['audit_id']")
echo "      audit_id=$audit_id user_response=$(printf '%s' "$sub" | j "['governance']['user_response']")"

outcomes=$(expect "guidance outcomes" 200 "$(call GET "/api/v1/projects/$PROJECT/guidance-outcomes?limit=5" alice.owner)")
[[ "$(printf '%s' "$outcomes" | j "['items'][0]['audit_id']")" == "$audit_id" ]] || { echo "FAIL  GuidanceOutcome not written with the AuditEvent" >&2; exit 1; }

for actor in alice.owner bob.finance evan.exec evan.exec; do
  inbox=$(expect "inbox $actor" 200 "$(call GET "/api/v1/inbox" "$actor")")
  printf '%s' "$inbox" | grep -q "$car_id" || { echo "FAIL  $actor inbox does not hold the CAR: $inbox" >&2; exit 1; }
  dec=$(expect "approve by $actor" 200 "$(call POST "/api/v1/cars/$car_id/decide" "$actor" '{"decision":"approve","comment":"ok"}')")
done
status=$(printf '%s' "$dec" | j "['car']['status']")
[[ "$status" == "approved" ]] || { echo "FAIL  CAR is $status after the chain" >&2; exit 1; }
echo "      authorized_amount=$(printf '%s' "$dec" | j "['car']['authorized_amount']") cumulative=$(printf '%s' "$dec" | j "['cumulative_authorized']")"

expect "second decide by the owner is 409" 409 "$(call POST "/api/v1/cars/$car_id/decide" alice.owner '{"decision":"approve"}')" >/dev/null
expect "former approver lost read once the chain closed (403)" 403 "$(call POST "/api/v1/cars/$car_id/decide" evan.exec '{"decision":"approve"}')" >/dev/null
expect "non-member cannot read the CAR (403)" 403 "$(call GET "/api/v1/cars/$car_id" nobody.check)" >/dev/null
expect "seeded draft without estimate is 422" 422 "$(call POST "/api/v1/cars/01900000-0000-7000-8000-00000000e002/submit" alice.owner)" >/dev/null
expect "audit chain verifies" 200 "$(call GET "/api/v1/audit/verify" alice.owner)" >/dev/null

deadline=$((SECONDS + TIMEOUT))
while :; do
  stats=$(curl -fsS -H 'X-Dev-Actor: car-check' "$CORE/api/v1/outbox/stats")
  pending=$(printf '%s' "$stats" | j "['pending']"); failed=$(printf '%s' "$stats" | j "['failed']")
  if [[ "$pending" == "0" ]]; then
    [[ "$failed" == "0" ]] || { echo "FAIL  outbox has failed rows: $stats" >&2; exit 1; }
    echo "ok    feedback and observe drained: $(printf '%s' "$stats" | j "['delivered']") delivered"
    break
  fi
  (( SECONDS < deadline )) || { echo "FAIL  outbox not drained within ${TIMEOUT}s: $stats" >&2; exit 1; }
  sleep 2
done
echo "PASS  toy CAR end to end (CR-CHECK-$run)"
