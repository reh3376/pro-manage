import Link from "next/link";
import { notFound } from "next/navigation";
import { api, tryApi, money, when, type Access, type AuditRow, type CAR, type Member, type Project, type Promise_ } from "../../lib/api";
import { PromiseForm } from "./promise-form";

export const dynamic = "force-dynamic";

// The project page (roadmap: one screen per persona in the chain). Every section
// below is fetched with the caller's session and simply absent when the §04 ACL
// says no, so the page is the access map made visible.
export default async function ProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const returnTo = `/projects/${id}`;
  const project = await tryApi<Project>(`/api/v1/projects/${id}`, { returnTo });
  if (!project) notFound();
  const [access, cars, members, audit, promises] = await Promise.all([
    api<Access>(`/api/v1/projects/${id}/access`, { returnTo }),
    tryApi<{ items: CAR[] }>(`/api/v1/projects/${id}/cars`, { returnTo }),
    tryApi<{ items: Member[] }>(`/api/v1/projects/${id}/members`, { returnTo }),
    tryApi<{ items: AuditRow[]; full: boolean }>(`/api/v1/projects/${id}/audit?limit=15`, { returnTo }),
    tryApi<{ items: Promise_[] }>(`/api/v1/projects/${id}/promises`, { returnTo }),
  ]);
  const canWriteBudget = access.sections.budget === "write";

  return (
    <main className="wide">
      <p className="crumbs">
        <Link href="/projects">Projects</Link> / {project.code}
      </p>
      <h1>
        {project.code} <span className="muted">{project.name}</span>
      </h1>
      <p className="muted">{project.description}</p>
      <section className="grid">
        <div className="card">
          <h2>Overview</h2>
          <dl>
            <dt>Lifecycle</dt>
            <dd>
              {project.lifecycle_state} ({project.fel_stage})
            </dd>
            <dt>Type</dt>
            <dd>{project.project_type}</dd>
            <dt>Owner</dt>
            <dd>{project.owner_user_id}</dd>
            <dt>Baseline</dt>
            <dd>{money(project.baseline_budget_total, project.currency) || "not set"}</dd>
            <dt>Dates</dt>
            <dd>
              {project.start_date ?? "?"} to {project.target_end_date ?? "?"}
            </dd>
            <dt>Onboarding</dt>
            <dd>{project.onboarding_level}</dd>
          </dl>
        </div>
        <div className="card">
          <h2>Your access</h2>
          <p className="muted">
            {access.base_role ? `Base role: ${access.base_role}` : access.stage_assignee ? "Stage assignee (no membership)" : "Platform role only"}
          </p>
          <ul className="sections">
            {Object.entries(access.sections).map(([s, p]) => (
              <li key={s} className={p === "none" ? "muted" : ""}>
                {s}: {p}
              </li>
            ))}
          </ul>
        </div>
      </section>

      {cars && (
        <section className="card">
          <div className="row">
            <h2>Capital authorizations</h2>
            {canWriteBudget && (
              <Link className="button" href={`/projects/${id}/cars/new`}>
                New CAR
              </Link>
            )}
          </div>
          {cars.items.length === 0 ? (
            <p className="muted">No CARs yet.</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Code</th>
                  <th>Kind</th>
                  <th className="num">Amount</th>
                  <th>Status</th>
                  <th>Signature</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {cars.items.map((c) => (
                  <tr key={c.car_id}>
                    <td>
                      <Link href={`/cars/${c.car_id}`}>{c.code}</Link>
                    </td>
                    <td>{c.kind}</td>
                    <td className="num">{money(c.amount_requested, c.currency)}</td>
                    <td>
                      <span className={`badge ${c.status}`}>{c.status}</span>
                    </td>
                    <td className="muted">{c.signed_artifact_ref ? c.signed_artifact_ref.split(":")[0] : ""}</td>
                    <td className="muted">{when(c.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      {promises && (
        <section className="card">
          <h2>Business-case promises</h2>
          <p className="muted">A full CAR needs every promise committed with a baseline and a measurement basis (FSD §03).</p>
          {promises.items.length === 0 ? <p className="muted">None yet.</p> : (
            <ul>
              {promises.items.map((p) => (
                <li key={p.promise_id}>
                  <strong>{p.kind}</strong> {p.metric_id && <span>({p.metric_id})</span>} promised {p.promised_value ?? "?"}: <span className={`badge ${p.status}`}>{p.status}</span>{" "}
                  <span className="muted">{p.measurement_basis}</span>
                </li>
              ))}
            </ul>
          )}
          {canWriteBudget && <PromiseForm projectID={id} />}
        </section>
      )}

      {members && (
        <section className="card">
          <h2>Team</h2>
          <table>
            <thead>
              <tr>
                <th>User</th>
                <th>Base role</th>
                <th>Since</th>
                <th>Expires</th>
              </tr>
            </thead>
            <tbody>
              {members.items.map((m) => (
                <tr key={m.user_id}>
                  <td>{m.user_id}</td>
                  <td>{m.base_role}</td>
                  <td className="muted">{when(m.invited_at)}</td>
                  <td className="muted">{m.expires_at ? when(m.expires_at) : ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {audit && (
        <section className="card">
          <h2>
            Audit <span className="muted">{audit.full ? "(full)" : "(summary)"}</span>
          </h2>
          <table>
            <thead>
              <tr>
                <th>#</th>
                <th>When</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Subject</th>
                <th>Verdict</th>
              </tr>
            </thead>
            <tbody>
              {audit.items.map((e) => (
                <tr key={e.audit_id}>
                  <td className="muted">{e.seq}</td>
                  <td className="muted">{when(e.at)}</td>
                  <td>{e.actor_user_id}</td>
                  <td>{e.action_type}</td>
                  <td className="muted">
                    {e.subject_type} {e.subject_id.slice(0, 8)}
                  </td>
                  <td>{e.classification_verdict ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </main>
  );
}
