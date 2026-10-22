# deploy/compose

Development and CI stacks for pro_manage (FSD §05 "local dev"; roadmap Phase 0).

| File | What it runs |
|---|---|
| `docker-compose.dev.yml` | Postgres, TimescaleDB, Redis, RustFS (S3), Keycloak (Northfield realm), Mailpit, pm-core, pm-workers, pm-sidecar, pm-frontend; `--profile stub` adds the stub MDEMG used by CI. |
| `docker-compose.mdemg.yml` | pro_manage's dedicated MDEMG stack, pinned by digest (FSD §06). |
| `.env.example` | Template for the developer's `.env` (`make env` fills the generated keys). |
| `.env.ci` | Values PR CI uses. Committed on purpose. |

## Credentials in this directory are throwaway

Everything here that looks like a secret is a development value for a stack that is created and destroyed on a workstation or inside one CI job, bound to loopback only:

- `.env.ci`: `ci-postgres`, `ci-keycloak`, `ci-rustfs-secret`, `ci-stub-key`, the CI audit chain key.
- `keycloak/northfield-realm.json`: seeded users (`alice.owner`, `bob.finance`, ...) with the password `northfield-dev`, and the `pm-core` client secret `pm-core-dev-secret`.

None of them are reused anywhere else, and none of them must ever be. Production secrets come from the vault through pm-core's `secrets.Provider` (FSD §11); backlog item B-03 moves the Keycloak client secret and dev password out of the realm file into environment substitution.
