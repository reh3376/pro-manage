# USTS specs

Security behaviour contracts for pm-core (FSD §12, §04), run by the vendored `usts_runner.py` against the compose stack after the Northfield seed: `make test-usts` (merge-blocking inside the compose smoke job since slice 10, 2026-10-30).

One spec per endpoint and method (the runner's model): `test.endpoint` and `test.method` name the surface, each entry of `requests` sets headers, payload, and the status it expects, and `assertions` apply to every request of the spec (`status_in`, `body_not_contains`, `headers_present`, `headers_not_present`). Categories in use: `authentication` (needs `USTS_AUTH_ENABLED=true`, which the Make target sets), `authorization`, `data_exposure`, `injection`, `headers`. Actors are the seeded dev users through the dev header (`X-Dev-Actor`, `X-Dev-Roles`), the same way the UATS specs name them.

| Spec | Boundary |
|---|---|
| `auth_me_requires_session` | `/auth/me` answers only to a valid session cookie |
| `api_requires_actor`, `inbox_requires_actor` | every `/api/v1` route is 401 without a session or the dev header |
| `webhook_sigi_stub_signature`, `webhook_unknown_provider` | the Sigi webhook verifies the module's signature; unknown providers are refused |
| `project_read_boundary` | membership, open stage, or a cross-project platform role to read a project; platform admin has no blanket read |
| `car_draft_write_boundary`, `team_write_boundary`, `budget_section_deny`, `budget_sync_write_boundary` | section writes and reads follow the §04 table; denial comes before validation |
| `signed_artifact_boundary` | the signed PDF is a budget read |
| `platform_admin_gate_sites`, `platform_admin_gate_pools` | platform-level writes need `platform_admin` |
| `audit_summary_exposure` | before/after payloads and chain hashes only for `read_full` |
| `error_bodies_no_internals`, `project_filter_injection`, `project_create_injection` | typed ids and validated fields; no database internals in any body |
| `response_headers_baseline` | nosniff, no referrer, no-store, request id; no server banner |

Still to write: contributor contract scoping per endpoint (with B-12), share-token exposure (with the public renderer, §09), rate limiting (with the edge, §11).
