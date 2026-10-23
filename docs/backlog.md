# Engineering backlog

Status: **Living list** · Last updated: 2026-10-22

Work the owner has asked for that is not a spine slice or an FSD open item. Ordered by priority within each group; each item names what "done" looks like. Owner decisions live in [open-items.md](./fsd/open-items.md); the delivery order lives in the [roadmap](./fsd/roadmap.md).

## Now

### B-01 Repository hardening (public repo)

Raised by the owner 2026-10-22. The repo is public and the only protection today is GitHub's default secret scanning (enabled, with push protection) and Dependabot security updates (enabled). `main` has **no branch protection**.

Done when:

1. **`main` is protected**: pull request required (no direct pushes, including by admins); at least one approving review from the owner; stale approvals dismissed on new commits; required status checks = the CI jobs (`Go`, `Python`, `Frontend`, `Protobuf (buf lint)`, `UxTS gates`, `Compose smoke + UATS`) and `open or update draft PR`; conversations must be resolved; force pushes and deletion blocked; linear history optional (squash or rebase merges keep it readable).
2. **`pro-dev01` keeps force-push allowed** (it is the working branch; history is rewritten when syncing after a merge), but deletion blocked.
3. **Actions hardened**: workflow permissions stay `contents: read` by default (already set); third-party actions pinned to commit SHAs with a Dependabot `github-actions` update config; `pull_request_target` never used.
4. **Dependabot version updates** for Go modules, pip (uv), npm (pnpm), Docker base images, and GitHub Actions, grouped weekly, so the pinned toolchain does not rot.
5. **CodeQL** (Go, JavaScript/TypeScript, Python) on push to `main` and on PRs, as a required check once it runs clean.
6. **Secret-scanning validity checks and non-provider patterns** enabled; a `SECURITY.md` with a private disclosure path; `CODEOWNERS` naming the owner for everything.
7. **Dev credentials in the repo are visibly dev-only**: the Keycloak realm's `pm-core-dev-secret` and `northfield-dev` passwords, and `.env.ci`, are documented as throwaway and never reused outside the compose dev stack (already true; add the note to `deploy/compose/README`).
8. Merge method restricted to squash or rebase; `delete_branch_on_merge` left off (pro-dev01 is reused).

Items 1, 2, 6, and 8 are repository settings an admin applies (the owner, or Claude via `gh api` with the owner's explicit go-ahead); 3, 4, 5, and 7 are files in a PR.

**Status 2026-10-22:** the file-side items landed (PR after #24): every third-party action in `ci.yml`, `auto-pr.yml`, and `codeql.yml` is pinned to a commit SHA with its version in a comment; `.github/dependabot.yml` covers GitHub Actions, Go modules, uv, npm, Docker base images, and the compose files (MDEMG images excluded, they follow the FSD §06 pin); `.github/workflows/codeql.yml` analyzes Go, JavaScript/TypeScript, Python, and Actions; `SECURITY.md` points at private vulnerability reporting; `.github/CODEOWNERS` names the owner; `deploy/compose/README.md` states that the dev credentials are throwaway. **Still open, admin-side (waiting for the owner's go):** branch protection on `main` (item 1), `pro-dev01` deletion block (2), secret-scanning validity checks and non-provider patterns (6), merge-method restriction (8), and making `codeql` a required check once it runs clean.

### B-02 Run the live MDEMG UATS set once the dedicated stack is up

`make mdemg-up` then `make test-uats-live`; record the date in the framework matrix. Pre-merge requirement for any adapter change thereafter.

## Next

### B-03 Keycloak realm hygiene

Move the `pm-core` client secret out of the realm JSON into an env substitution at import (`${PM_CORE_OIDC_SECRET}`), keep the seeded users but make the dev password an env value too.

### B-04 Toolchain install notes

`sqlc` and `goose` are invoked through `go run` pinned versions so nothing has to be installed globally; document that and the `uv run --with` pattern for the UxTS runners in the README's prerequisites.

### B-05 pm-sidecar gRPC contract

Replace the health-only sidecar with `pm.sidecar.v1.RenderService` stubs once the first export exists; UDTS spec with it.

### B-06 Pre-classification cache to Redis

The spine's `governance.PreclassCache` is in-process (scoped to user and project, 10-minute TTL, keyed by `sha256(action_kind, payload, policy_snapshot)` as `§06` specifies). Move it to the stack's Redis before pm-core runs more than one replica, so a verdict pre-classified on one instance is honoured by the one that takes the submit.

### B-07 CAR chain: FX conversion and business-day SLA clocks

The toy compares the routing amount in the CAR's currency against thresholds resolved for the project (the seed's site override expresses the Ontario threshold in CAD); `§08` wants conversion at `fx.rate.<from>.<to>` on submission with the rate in the snapshot. Stage SLAs run on the wall clock; `§08` wants business days of the site calendar. Both land with `ERPi_IFS` (rates) and the site calendar.

### B-08 SLA tick, nudges, and the empty-pool pause

`stage_due_at` is recorded and the inbox flags overdue stages, but nothing nudges yet. pm-workers gets the SLA tick (`§08`), Commsi_SMTP sends the nudge, and an empty approver pool pauses the workflow with a visible blocker instead of failing the submit as a precondition.

### B-10 pm-core login on IdPi_OIDC (slice 4b)

The module exists (slice 4a). pm-core still authenticates by `X-Dev-Actor`. Slice 4b adds the connector client, `GET /auth/login` and `/auth/callback` through `AuthenticateInteractive` and `CompleteAuthentication` (a pending-login record holds state, nonce, and the PKCE verifier), just-in-time `users` rows, Redis-backed opaque sessions (`§04` session model: 8 h sliding, 24 h cap), `GET /auth/me`, the logout cascade, the actor resolved from the session with platform roles, and the approver pools filled from `ResolveClaimsToRoles` (closes the first half of B-09). The Keycloak `pm-core` client needs `standardFlowEnabled` and the callback redirect URI (a realm change, so an existing dev volume must be dropped with `make dev-down V=1` or edited in the admin console). The dev realm's partial import creates no `profile` or `email` client scopes, so the module runs with `OIDC_SCOPES=openid` there; real IdPs keep the `openid profile email` default. The dev header stays available behind `PM_DEV_ACTOR_HEADER` for CI.

### B-09 Approver pools from IdP claims; admin override of a Tier A refusal

`approver_pools` is a global table filled by the seed and `POST /api/v1/approver-pools`; `IdPi_OIDC` claims mapping (groups to pools, `§04`) replaces the manual rows. The admin override that `§06` offers after a Tier A refusal (`overridden_by_admin`, `blocked_false_positive` feedback) is not exposed until the ACL layer can tell who is an admin.

## Done

| Id | Item | Closed in |
|---|---|---|
| (none yet) | | |
