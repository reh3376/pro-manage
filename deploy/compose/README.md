# deploy/compose

Development and CI stacks for pro_manage (FSD §05 "local dev"; roadmap Phase 0).

| File | What it runs |
|---|---|
| `docker-compose.dev.yml` | Postgres, TimescaleDB, Redis, RustFS (S3), Keycloak (Northfield realm), Mailpit, pm-core, pm-workers, pm-sidecar, pm-frontend, idpi-oidc (the `IdPi_OIDC` connector module against Keycloak), sigi-stub (the `Sigi_STUB` e-signature module; signed PDFs land in RustFS under `envelopes/`); `--profile stub` adds the stub MDEMG used by CI. |
| `docker-compose.mdemg.yml` | pro_manage's dedicated MDEMG stack, pinned by digest (FSD §06). |
| `.env.example` | Template for the developer's `.env` (`make env` fills the generated keys). |
| `.env.ci` | Values PR CI uses. Committed on purpose. |

## Credentials in this directory are throwaway

Everything here that looks like a secret is a development value for a stack that is created and destroyed on a workstation or inside one CI job, bound to loopback only:

- `.env.ci`: `ci-postgres`, `ci-keycloak`, `ci-rustfs-secret`, `ci-stub-key`, the CI audit chain key.
- `keycloak/northfield-realm.json`: seeded users (`alice.owner`, `bob.finance`, ...) with the password `northfield-dev`, and the `pm-core` client secret `pm-core-dev-secret`.

None of them are reused anywhere else, and none of them must ever be. Production secrets come from the vault through pm-core's `secrets.Provider` (FSD §11); backlog item B-03 moves the Keycloak client secret and dev password out of the realm file into environment substitution.

## Keycloak realm changes

Keycloak imports `keycloak/northfield-realm.json` only when the realm does not exist yet, so a change to the file does not reach a dev stack that already has one. Either drop the volumes (`make dev-down V=1`, then `make dev-stub` and `make db-seed`) or apply the change in the admin console (`http://localhost:8180`, the `KC_ADMIN_*` credentials from `.env`). Changes so far: 2026-10-22 (slice 4a) added the `subject` mapper to the `pro_manage` client scope because Keycloak 26 omits `sub` from access tokens unless it is mapped. The realm's partial import creates no `profile` or `email` client scopes, so clients request `openid` only (the `pro_manage` scope is default). `pm-dev-cli` is a dev-only public client with the password grant for tests (`make test-idpi-live`).

## SSO on the workstation

`make dev-stub` gives you a working sign-in: the frontend's **Sign in** link goes to pm-core's `/auth/login`, through the `idpi-oidc` module to Keycloak (`alice.owner` and friends, password `northfield-dev`), back to `/auth/callback`, and the `pm_session` cookie then authenticates `/api/v1` calls without the dev header. The callback base is `PM_PUBLIC_URL` (default `http://localhost:8080`). If another process on your machine listens on `[::1]:8080` (browsers and Go resolve `localhost` to IPv6 first), set `PM_PUBLIC_URL=http://127.0.0.1:8080` and `PM_FRONTEND_URL=http://127.0.0.1:3100` in `.env` and open the frontend at `127.0.0.1:3100`, since the cookie is host-scoped. Realm changes for SSO (2026-10-23, slice 4b): the `pm-core` client has the standard flow enabled with the two callback URIs; see "Keycloak realm changes" above for how to apply a realm change to an existing volume.
