# Security policy

pro_manage is pre-release software under active development in a public repository. It is not deployed anywhere yet; the only running instances are development stacks on the maintainer's workstation and GitHub-hosted CI.

## Reporting a vulnerability

Please do not open a public issue for a security problem. Use GitHub's private vulnerability reporting for this repository ("Report a vulnerability" under the Security tab), which reaches the maintainer directly. You can expect an acknowledgement within seven days.

If the report concerns MDEMG rather than pro_manage, report it in the [MDEMG repository](https://github.com/reh3376/mdemg) instead.

## Scope notes

- Credentials in this repository (`deploy/compose/.env.ci`, the Keycloak realm's seeded users and the `pm-core-dev-secret` client secret, `northfield-dev` passwords) are throwaway development values for the compose stack and CI. They grant nothing anywhere else. Reporting them is not necessary.
- The `X-Dev-Actor` header accepted by pm-core is a development-only mechanism gated by `PM_DEV_ACTOR_HEADER`; with it off, every `/api/v1` request is rejected until SSO (IdPi_OIDC) lands. A deployment with that flag on outside a developer's machine is a misconfiguration, not a vulnerability in the code.
- Dependency and action updates arrive through Dependabot; secret scanning with push protection is enabled on the repository.

## Supported versions

There are no released versions yet. Security fixes land on `main` through the normal pull request flow.
