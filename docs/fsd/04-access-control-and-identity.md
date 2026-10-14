# 04: Access Control & Identity

## Decisions on record

- **2026-10-13:** Authentication: **SSO primary + local break-glass admin fallback**. SSO backed by `IdPi` module (OIDC and SAML abstractions). Local admin accounts exist for recovery only and are policy-fenced.
- **2026-10-13:** Authorization: **full ACL per section, per document, per field** on top of three base roles (`owner`, `editor`, `viewer`) plus platform roles (`platform_admin`, `governance_author`, `compliance_officer`, `portfolio_executive`).
- **2026-10-14:** Fourth base role **`contributor`** for external EPCM partner, contractor, supplier, and designer staff, scoped by `Contract`. New workflow roles: `construction_manager`, `discipline_engineer`, `inspector`.
- **2026-10-13:** Default policy is **deny**. Access requires an explicit base role (via project membership) or a platform role.
- **2026-10-13:** Grant evaluation order: **explicit DENY → explicit ALLOW → base role default**. Base role is only consulted if no explicit grant matches.
- **2026-10-13:** Project **Owner** always has read + write on their own project. Grants cannot DENY an Owner on their own project. If finer-grained control is needed, assign a different person as Owner.
- **2026-10-13:** Session identity for pro_manage ↔ MDEMG is keyed as `user_id:project_id` so MDEMG escalation and trust accumulate per (user, project) rather than per HTTP request.

## Identity

### IdPi: the SSO abstraction

`IdPi` is the pluggable identity module. It presents a stable interface to the core and adapts to specific backends.

**Interface (conceptual):**
- `AuthenticateInteractive(ctx, redirect_url) → (user_id, claims, refresh_token)`
- `ValidateToken(ctx, token) → (user_id, claims, exp)`
- `RefreshToken(ctx, refresh_token) → (new_access_token, new_refresh_token, exp)`
- `ResolveClaimsToRoles(ctx, claims) → (platform_roles[], group_memberships[])`
- `Logout(ctx, user_id, session_id)`

**Supported backends (year one):**
- OIDC generic (any compliant provider)
- SAML 2.0 generic (any compliant provider)
- Entra ID (OIDC flavor; convenience wrapper over the generic OIDC backend)
- Okta (OIDC flavor)
- On-prem AD via ADFS (SAML or OIDC)

**Claims-to-role mapping:** Platform admin configures a declarative mapping that resolves IdP claims (group memberships, custom attributes) into pro_manage platform roles. Example:

```yaml
# IdPi claim mapping (platform admin UI stores this)
mappings:
  - claim: "groups"
    contains: "pro_manage.platform_admin"
    assigns: ["platform_admin"]
  - claim: "groups"
    contains: "pro_manage.compliance"
    assigns: ["compliance_officer", "governance_author"]
  - claim: "department"
    equals: "Finance"
    assigns: ["finance_approver_eligible"]  # eligibility, not grant
```

Group membership and claim resolution happen on login and are cached for the lifetime of the access token.

### User model

- `User`: `user_id` (UUIDv7), `external_id` (IdP's sub), `email`, `display_name`, `platform_roles[]`, `is_local` (bool), `is_active`, `created_at`, `last_login_at`.
- Local users (break-glass only) also have `local_password_hash` + `mfa_secret` and are flagged in logs on every login.
- Users are provisioned **just-in-time** on first SSO login. Deprovisioning: a user who disappears from the IdP (SCIM-driven when available, or periodic reconciliation when not) is marked `is_active=false`. Their historical attributions are preserved; their active grants are suspended but recoverable if reactivated.

### Break-glass admin

- Up to 2 local admin accounts per deployment, created at install time and documented.
- Local login requires both password and MFA (TOTP) by policy. Credentials are stored in the deployment's secret vault, not in-band.
- Every local admin login writes a conspicuous audit event. Access by local admins to project content is logged, and generates an operational alert via `Commsi` to a configured recipient list.
- Not usable for day-to-day administration. If a Platform Admin finds themselves using it routinely, that's a reportable gap in the SSO integration.

### Session model

- Session tokens are opaque bearer tokens issued by pro_manage, backed by a Redis-stored session record pointing to the user and the active IdP tokens.
- Session lifetime: configurable (default 8 hours sliding) with absolute cap (24 hours). After absolute cap, re-authentication is required.
- Sessions carry a `pm_session_id` that pro_manage prepends to the MDEMG `session_id` when calling J17 (`session_id = user_id:project_id`; `pm_session_id` is used in audit correlation).
- Revocation: Platform Admin can revoke any session; a user's SSO logout cascades to pro_manage sessions.

## Authorization

### Platform roles

Platform roles are global, assigned via IdPi claims-mapping, and grant abilities across projects.

| Role | Grants |
|---|---|
| `platform_admin` | Operate the platform. Configure modules. Manage users. **Does not** have blanket read on project content. |
| `policy_owner` | Sets policy variables (approval thresholds, SLAs, cadences; `§08`) at global, project-type, portfolio, and project scope. Typically held by Finance controls. Cannot set a project override on any project where they sit in the approval chain. No project-content access beyond the policy view. |
| `governance_author` | Author Jiminy constraints via the Governance admin UI. Scope: `space=org.<id>.governance`. No project-content access. |
| `compliance_officer` | Read access to safety/quality/compliance sections of projects under their jurisdiction (configured per-officer). May be composed with `governance_author`. |
| `portfolio_executive` | Read across all projects in configured portfolios. May be configured as `finance_approver_eligible` for CAR chains. |
| `finance_approver_eligible` | Eligibility, not grant. Approval workflows (`§08`) can route to this pool and resolve the specific approver by amount threshold + delegation chain. |

### Workflow roles (stage assignment)

Approval stages in `§08` name a **workflow role**, not a person. pm-core resolves each role to an eligibility pool when the stage opens. Workflow roles grant the right to *decide a stage* plus the read access that stage needs; they are not standing access to project content.

| Workflow role | Pool resolved from | Typical stages |
|---|---|---|
| `project_owner` | The project's Owner | First stage of most workflows |
| `finance_approver` | `finance_approver_eligible` ∩ business unit | CAR, baseline change, Benefits Realization |
| `vp_finance`, `cfo`, `ceo` | Named holders + one delegate each (admin-configured) | CAR above amount thresholds |
| `site_operations_lead`, `site_maintenance_lead` | Per-site assignment (admin-configured; may map from IdPi groups) | Shutdown & tie-in plan, Early Management exit |
| `operator_representative`, `maintenance_representative` | Named per project by the Owner from site pools | Design review sign-off, CQV co-sign |
| `commissioning_engineer` | Named per project | CQV records and sign-off |
| `qa_lead` | Per-site, regulated contexts only | Final CQV sign-off |
| `compliance_officer` | Platform role, by jurisdiction | Early Management exit (regulated), closeout |
| `portfolio_executive` | Platform role, by portfolio | Benefits Realization closure |
| `facilitator` | Any user other than the Owner, named per review | Design review, project health review |
| `construction_manager` | Named per project (owner-side or EPCM staff) | Design review (constructability), work package release, shutdown & tie-in plan |
| `discipline_engineer` | Named per project and discipline | RFI response, submittal review |
| `inspector` | Named per ITP (owner QA, EPCM QA, or third party) | Hold point release |

Site representatives and the commissioning engineer get `read + comment` on the project sections their stages touch (scope, schedule, documents, design reviews, CQV, turnover) for the life of their assignment. Budget sections stay deny-by-default for them.

### Base roles (project-scoped)

Assigned via `ProjectMember.base_role`.

| Role | Default access |
|---|---|
| `owner` | **Everything** on their own project. Cannot be masked by DENY grants on their own project. |
| `editor` | Default `read` on **non-sensitive** sections (Overview, Scope, Schedule, Risk/Issues, Documents, Metrics, Audit summary). Writes require an explicit WRITE grant per section. |
| `viewer` | Default `read` on non-sensitive sections. No write. |
| `contributor` | **External** (EPCM partner, contractor, supplier, designer staff). Access is scoped by `Contract`: `read + write` on the work packages, RFIs, submittals, change orders, deviations, hold points, and punchlist items under their organization's contract(s); `read` on the drawings, schedule, and scope those packages reference. **Deny** on budget (except their own contract value), other contracts' commercials, internal approvals, governance, audit, and other contractors' packages. Cannot hold `owner` or be a CAR approver. |

### What is a "section"

Sections are the top-level divisions of a project page. Each gets its own permission bucket.

| Section | Default visibility (editor/viewer) | Common grants |
|---|---|---|
| `overview` | read | n/a |
| `team` | read (public), write (invite teammates, change ACLs; requires owner or explicit grant) | `write` to delegated owners |
| `budget` | **deny** by default | `read:budget` to Finance Approvers; `read+write:budget` to the financial analyst(s) |
| `schedule` | read | `write` to scheduler(s) |
| `scope` | read | `write` to engineers / product leads |
| `risk` | read | `write` to risk owners |
| `issues` | read | `write` to assignees |
| `change_requests` | read | `write` to initiators; approval via `§08` |
| `documents` | read on non-sensitive | per-document grants |
| `metrics` | read | `write` on manual metrics via grant |
| `approvals` | read (own + routed-to-me) | n/a |
| `audit` | **summary only** by default; **full** on grant | `read_full:audit` to compliance officers |
| `governance` | read (constraints applied to this project) | `write` only to governance authors (via platform role) |
| `settings` | **deny** by default | `write:settings` for selected editors; `read:settings` for everyone with any access |

### Documents and fields

Beyond sections, grants may target specific documents or specific fields.

**Document grants** (`scope_kind=document`):
- Target: a specific document (`scope_target=document:doc_123`) or a tag (`scope_target=document_tag:confidential`).
- Common pattern: a project has docs tagged `confidential`, visible by default only to Owner + explicit grantees.

**Field grants** (`scope_kind=field`):
- Target: a specific field path (`scope_target=field:budget.line_item.cost`), including a glob (`scope_target=field:budget.*.cost`).
- Enforcement: the backend projects responses with masked/omitted fields based on the caller's effective grants. The UI never sees content it isn't entitled to.
- Reserved namespace: `field:budget.*.salary`, `field:budget.*.vendor_cost`, `field:budget.*.bid_price`, `field:risk.*.legal_sensitivity`, `field:document.*.restricted_metadata` are **deny by default even for editors**. Explicit `read` grants required.

### Grant evaluation

For a given caller + target + requested permission:

1. Resolve the **caller's effective grants** by walking:
   - Platform roles (apply global rules, e.g., compliance_officer gets `read` on safety sections in jurisdiction)
   - Project membership base role
   - Explicit ACLGrant rows for the project, sorted by specificity (field > document > section > tag)
2. **Evaluate from most-specific to least-specific:**
   - If any matching grant is `deny`: deny.
   - If any matching grant is `allow`: allow (and `write` implies `read`).
   - If no explicit grant matched: apply base-role default per the section table above.
3. **Owner override:** If caller is Owner of this project, allow, unless the request is for a platform-level restricted operation that even Owners cannot perform (e.g., accessing another project they don't belong to).

**Writes also require an unexpired version check** (optimistic concurrency) and a transaction that commits the AuditEvent in the same transaction as the mutation. There is no write that lacks an audit row.

### Default sensitive fields (reserved namespace)

Platform admin can extend; these are hard-wired as deny-by-default for `editor`/`viewer` from day one:

- `budget.*.salary`
- `budget.*.vendor_cost`
- `budget.*.bid_price`
- `budget.*.unit_rate`
- `risk.*.legal_hold`
- `risk.*.litigation_notes`
- `document.*.restricted_metadata`
- `team.*.compensation`
- `audit.*.full_payload` (editors see summary, not full before/after)

### Delegation and expiration

- Grants can specify `expires_at`. Expired grants are inert; the UI surfaces expiring grants to the Owner weekly.
- Owners can delegate grant-management on specific sections ("Alice may manage document grants on this project") through a meta-grant with `scope_kind=section, scope_target=team.grants.documents`.
- Platform Admin cannot grant themselves access to project content via the normal grant UI; any such access is a separate audited action requiring a reason, time-boxed, and surfaced to the Owner.

### EPCM and contractor contributors

- **Identity:** preferably federated: the contractor's own IdP trusted through IdPi (OIDC/SAML), so leavers are deprovisioned at their source; otherwise company-issued guest accounts with MFA and an `expires_at` tied to the contract end date.
- **Scoping:** a contributor's effective grants are computed from `Contract → WorkPackage` membership, not hand-assigned per section. Adding a package to a contract extends access; closing a contract revokes it.
- **Audit:** every contributor action carries `counterparty_org` in the AuditEvent, so "what did contractor X change" is one query.
- **MDEMG:** contributors see Jiminy guidance only from constraints whose `annotations.roles` include `contributor`; project-internal observations are not shown to them.

### External stakeholders and tokenized access

For external stakeholders (`§02`):

- **SSO-federated external users** go through IdPi with their own IdP federated to the company IdP (via trust relationships configured by the Platform Admin). Standard User/ProjectMember model applies.
- **Tokenized dashboard links** are a separate credential type. Owner (or above) issues a `DashboardShare` with:
  - `share_id`, `dashboard_id`, `audience_label` (free text for the Owner to remember who), `allowed_identities[]` (empty = anyone with the link, usually constrained), `expires_at`, `revoked_at?`.
  - On first open, pro_manage may require the viewer to attest their identity (name + email) and accept terms. Attestations are logged.
  - Tokens are opaque, short-lived, rotated. Underlying dashboard permissions are computed as if the viewer had a read-only ACL covering exactly the dashboard's bound data.

## Enforcement architecture

Access checks happen at four layers, each a defense:

1. **Edge gateway (Go):** authenticates the session, resolves the user, loads effective grants into the request context. Rejects anonymous traffic. Caches resolved grants per request.
2. **Service layer (Go):** the request handler for a specific section/entity checks the exact permission needed. If a request can serve a mix (e.g., list of risks spanning a project), the handler filters by effective grants per row before returning.
3. **Projection layer (Go):** before serializing responses, strips or masks fields the caller doesn't have `read` on. UI never receives disallowed content, so it cannot leak via developer tools.
4. **Database row-level security (defense-in-depth):** Postgres row-security policies on sensitive tables enforce `project_id ∈ user's visible projects` even if a bug slipped past the application layer. Set up with `SET LOCAL app.user_id = …` per connection pool checkout.

## Audit of access decisions

- Every **denied** access attempt is logged with `(user, action, target, reason)`. Not every allow (that would be too noisy for a 500-user system), but all state-changing actions write a full AuditEvent regardless of outcome.
- The AuditEvent carries `guidance_id` when MDEMG was consulted (approval, baseline change, closeout) so after-the-fact review can reconstruct exactly what guidance the user saw.
- Local-admin and external-stakeholder actions are singled out in audit queries ("show me every project read by a tokenized external viewer in the last 30 days").

## Secrets and key management

- All secrets live in the deployment's vault (Platform Admin configures: HashiCorp Vault, cloud KMS, or on-prem equivalent). Code never reads secrets from env vars directly in production; the Go backend fetches from vault at startup and refreshes rotatable secrets on a schedule.
- Key secrets:
  - `IdPi` provider credentials (per backend)
  - IdPi signing/verification keys (OIDC JWKS cache, SAML certificate)
  - pro_manage ↔ MDEMG shared secret (`AUTH_API_KEYS` or `AUTH_JWT_SECRET`)
  - J17 ticket secret (if we use session resumption)
  - Database passwords, Redis passwords, Timescale passwords
  - S3 object-storage credentials
  - `Sigi` provider API keys
  - `Commsi` SMTP/Teams/Slack webhook credentials
  - `ERPi`, `Schedulei`, `Doci`, `Metrici`, `HSEi`, `CMMSi` backend credentials (per-backend, per-tenant). CMMSi credentials carry **write** scope for turnover delivery and get the same rotation discipline as the MDEMG key.
- Rotation: automated for secrets with programmatic rotation (DB passwords, pro_manage ↔ MDEMG shared key via overlap-rotate). Documented quarterly review for the rest.

## Open items for Section 04

- Decide whether external stakeholders routinely use federated SSO or whether tokenized links dominate (affects IdPi work and the `DashboardShare` design).
- Confirm MFA policy for local admins beyond TOTP (hardware token?). Also confirm whether SSO users must have MFA enforced at the IdP layer (we assume yes).
- Decide whether a time-boxed auditor role is a distinct platform role (`auditor_time_boxed`) or just a tagged External Stakeholder session. Current draft: just a session attribute.
- Decide whether field-level grants need wildcarding beyond simple globs, e.g., CEL-style expressions. Probably not for year one; keep globs.
- Define the SCIM provisioning path (if any) for user lifecycle: JIT-only on login, or periodic sync?
