# pro_manage: how an AI coding agent authors and opens PRs

You are working in https://github.com/reh3376/pro-manage, local checkout
`/Users/reh3376/pro_manage`. The owner (Roger Henley, GitHub `reh3376`) reviews,
approves, and merges every change. You never merge anything.

## Standing rules

1. Never commit to `main`. All work happens on the branch `pro-dev01`.
2. One slice of work per PR. `main` must stay runnable after every merge.
3. Commits use Conventional Commits (`feat:`, `fix:`, `docs:`, `ci:`, `deps:`),
   are authored as Claude, carry the co-author trailer, and are made without
   GPG signing:

   ```bash
   git -c commit.gpgsign=false commit --author='Claude <noreply@anthropic.com>' -F msg.txt
   ```

   The last line of every commit message is:

   ```text
   Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
   ```

4. No em-dashes anywhere in documentation, commit messages, or PR text.
   Vendored code (for example the MDEMG test runners) is left verbatim.
5. The employer's name never appears in anything committed. `docs/personal/`
   is gitignored and stays that way.
6. Never run `gh pr create`. A workflow (`.github/workflows/auto-pr.yml`)
   opens or updates the PR automatically on every push to `pro-dev01`.
7. Never merge, approve, or close a PR, and never act on Dependabot PRs
   except to investigate and fix them (see step 8).

## Step by step

### 1. Start from a synced branch

```bash
cd /Users/reh3376/pro_manage
git checkout pro-dev01
git fetch origin
git merge --no-edit origin/main
git push origin pro-dev01
```

If `main` moved since the last sync, this brings it in. Do this after every
merge the owner reports, before starting new work.

### 2. Build the slice

Read the project context first: `docs/fsd/` (the FSD), `docs/fsd/roadmap.md`,
`docs/backlog.md`, the README, and the CLAUDE.md files. Make the change
against the FSD's decisions. Keep the scope to the one slice you were given.

### 3. Verify locally before you push

Run what applies to the change. The bar is: everything you touched is proven
against the running compose stack, not only by unit tests.

```bash
make lint                 # go vet + gofmt, ruff, mypy, buf lint
make test                 # Go tests, sidecar tests, frontend type check
make uxts                 # spec hashes, UNTS registry, inventories, matrix
make dev-stub             # the whole stack against the stub MDEMG, as CI runs it
make db-seed
make test-uats            # REST contract specs against the running stack
make test-udts            # gRPC contract specs
make test-usts            # security specs
make check-car            # the toy CAR end to end
make check-login          # SSO end to end
make check-screens        # the frontend walked as the project owner
```

If you added or changed a UATS spec: `make uxts-add-hashes` (it also refreshes
the UNTS registry). If you added a route, gRPC method, policy variable, or
connector capability: adjudicate it in `docs/inventories/` and run
`python3 scripts/verify_inventories.py`.

On this workstation another process holds `localhost:8080`, so the local
`deploy/compose/.env` sets `PM_PUBLIC_URL=http://127.0.0.1:8080` and
`PM_FRONTEND_URL=http://127.0.0.1:3100`; use `127.0.0.1` in local checks.
When you recreate `pm-core` or `pm-workers` by hand, pass
`MDEMG_BASE_URL=http://stub-mdemg:9999` (what `make dev-stub` does), or they
point at the dedicated MDEMG stack instead of the stub.

### 4. Documentation sweep (required before every push)

Enumerate every tracked Markdown file (`git ls-files '*.md'`) and update each
one the change affects: `README.md`, the decision lines of the `docs/fsd/`
sections, the roadmap spine row, `docs/backlog.md` (move finished items to its
Done table), `docs/tests/UXTS_FRAMEWORK_MATRIX.md` counts, `connectors/README.md`,
`proto/README.md`, `deploy/compose/README.md`, `SECURITY.md`. Check for em-dashes:

```bash
git ls-files '*.md' | grep -v '^docs/personal' | xargs grep -n -- '—'
```

### 5. Commit and push

One commit per logical change, with a message body that explains what and
why, including the tests and docs touched. Then:

```bash
git push origin pro-dev01
gh pr list --head pro-dev01     # the auto-PR workflow opened or updated the PR
```

### 6. Watch CI, fix anything red

```bash
gh run list --branch pro-dev01 --workflow ci.yml --limit 1
gh run watch <run-id> --exit-status
gh api repos/reh3376/pro-manage/commits/<sha>/check-runs \
  --jq '.check_runs[] | "\(.name): \(.conclusion)"'
```

The last command shows the code-scanning result, which is separate from the
workflow jobs and also gates the PR. If a job or the CodeQL check fails: read
the log, fix the cause on `pro-dev01` (never dismiss an alert to make it pass),
push again, and watch again. A run cancelled because a newer commit arrived is
not a failure.

### 7. Report, then stop and wait

Tell the owner: what the slice delivers, how it was verified (name the
checks), anything left out and why, and the PR link. Then stop. Do not merge,
and do not start the next slice. When the owner says "pr N merged", go back to
step 1, confirm with `gh pr view N --json state` (the owner's note can be ahead
of GitHub), and check for open PRs and security alerts:

```bash
gh pr list --state open
gh api 'repos/reh3376/pro-manage/dependabot/alerts?state=open' --jq 'length'
```

### 8. Dependabot PRs

The owner merges Dependabot PRs. If one fails CI, investigate the failing
step's log. If the fix belongs in the repository (for example a bump that must
land in every Go module at once because the connector modules `replace`
pm-core, or a Dependabot rule that needs changing), make it on `pro-dev01` so
it supersedes the Dependabot PR, and tell the owner which Dependabot PRs to
close once yours merges.

### 9. Things that need the owner's explicit go-ahead

Repository settings (branch protection, Pages), anything that publishes
outside the repository, deleting data, and anything the FSD marks as an owner
decision (`docs/fsd/open-items.md`). Ask, do the rest of the work that does not
depend on the answer, and continue when told.
