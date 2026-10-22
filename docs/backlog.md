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

### B-02 Run the live MDEMG UATS set once the dedicated stack is up

`make mdemg-up` then `make test-uats-live`; record the date in the framework matrix. Pre-merge requirement for any adapter change thereafter.

## Next

### B-03 Keycloak realm hygiene

Move the `pm-core` client secret out of the realm JSON into an env substitution at import (`${PM_CORE_OIDC_SECRET}`), keep the seeded users but make the dev password an env value too.

### B-04 Toolchain install notes

`sqlc` and `goose` are invoked through `go run` pinned versions so nothing has to be installed globally; document that and the `uv run --with` pattern for the UxTS runners in the README's prerequisites.

### B-05 pm-sidecar gRPC contract

Replace the health-only sidecar with `pm.sidecar.v1.RenderService` stubs once the first export exists; UDTS spec with it.

## Done

| Id | Item | Closed in |
|---|---|---|
| (none yet) | | |
