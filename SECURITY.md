# Security policy

pro_manage is pre-release software under active development in a public repository. It is not deployed anywhere yet; the only running instances are development stacks on the maintainer's workstation and GitHub-hosted CI.

## Reporting a vulnerability

Please do not open a public issue for a security problem. Use GitHub's private vulnerability reporting for this repository ("Report a vulnerability" under the Security tab), which reaches the maintainer directly. You can expect an acknowledgement within seven days.

If the report concerns MDEMG rather than pro_manage, report it in the [MDEMG repository](https://github.com/reh3376/mdemg) instead.

## Scope notes

- Credentials in this repository (`deploy/compose/.env.ci`, the Keycloak realm's seeded users and the `pm-core-dev-secret` client secret, `northfield-dev` passwords) are throwaway development values for the compose stack and CI. They grant nothing anywhere else. Reporting them is not necessary.
- The `X-Dev-Actor` and `X-Dev-Roles` headers accepted by pm-core are a development and CI mechanism gated by `PM_DEV_ACTOR_HEADER`; with it off, only an SSO session (`pm_session` cookie issued after sign-in through the `IdPi_OIDC` module) authenticates a request. A deployment with that flag on outside a developer's machine or CI is a misconfiguration, not a vulnerability in the code.
- The `Sigi_STUB` module's `/dev/envelopes/{id}/sign` endpoint completes an e-signature without a signer. It is the development and CI stand-in for a provider and is never deployed; the real backend (`Sigi_DS`) has no such endpoint.
- The `ERPi_STUB` module's `/dev/projects` and `/dev/releases` endpoints add ledger projects, post actuals, and list budget releases without authentication. They are the development and CI stand-in for an ERP back office and are never deployed; `ERPi_IFS` has no such endpoints.
- Dependency and action updates arrive through Dependabot; secret scanning with push protection is enabled on the repository.

## Supported versions

There are no released versions yet. Security fixes land on `main` through the normal pull request flow.
