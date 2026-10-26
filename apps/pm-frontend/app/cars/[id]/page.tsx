import Link from "next/link";
import { notFound } from "next/navigation";
import { api, tryApi, money, when, corePublicURL, whoAmI, type AuditRow, type CARView, type GuidanceView, type Project, type Signature } from "../../lib/api";
import { DecidePanel, SubmitPanel } from "./panels";

export const dynamic = "force-dynamic";

const slaLabel = (s: { entered_at: string | null; escalated_at: string | null; overdue_at: string | null; nudged_at: string | null; stage_due_at: string | null; completed_at: string | null }) => {
  if (s.completed_at || !s.entered_at) return "";
  if (s.escalated_at) return "escalated";
  if (s.overdue_at || (s.stage_due_at && new Date(s.stage_due_at) < new Date())) return "overdue";
  if (s.nudged_at) return "nudged";
  return "on time";
};

// The approval screen with the guidance rail (FSD §06 GUIDE on open, §08 chain,
// §10 signature): what the approver sees, decides, and what the record shows.
export default async function CARPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const returnTo = `/cars/${id}`;
  const view = await tryApi<CARView>(`/api/v1/cars/${id}`, { returnTo });
  if (!view) notFound();
  const car = view.car;
  const [me, project, guidance, signature, audit] = await Promise.all([
    whoAmI(),
    tryApi<Project>(`/api/v1/projects/${car.project_id}`, { returnTo }),
    car.status === "draft" ? api<GuidanceView>(`/api/v1/cars/${id}/guidance`, { returnTo }) : tryApi<GuidanceView>(`/api/v1/cars/${id}/guidance`, { returnTo }),
    tryApi<Signature>(`/api/v1/cars/${id}/signature`, { returnTo }),
    tryApi<{ items: AuditRow[]; full: boolean }>(`/api/v1/projects/${car.project_id}/audit?limit=200`, { returnTo }),
  ]);
  const username = me?.user.username ?? "";
  const current = view.workflow ? view.stages.find((s) => s.stage_order === view.workflow!.current_stage) : undefined;
  const myTurn = !!(car.status === "in_flight" && current && !current.completed_at && current.assignees.includes(username));
  const carAudit = (audit?.items ?? []).filter((e) => e.subject_id === car.car_id);

  return (
    <main className="wide">
      <p className="crumbs">
        <Link href="/projects">Projects</Link> / {project ? <Link href={`/projects/${car.project_id}`}>{project.code}</Link> : car.project_id.slice(0, 8)} / {car.code}
      </p>
      <h1>
        CAR {car.code} <span className={`badge ${car.status}`}>{car.status}</span>
      </h1>
      <p className="muted">
        {car.kind} for {money(car.amount_requested, car.currency)} on basis {car.basis}
        {project ? `, project ${project.code} ${project.name} (${project.fel_stage})` : ""}. Cumulative authorized on the project: {money(view.cumulative_authorized, car.currency)}.
      </p>

      <section className="grid">
        <div className="card">
          <h2>Request</h2>
          <dl>
            <dt>Business case</dt>
            <dd className="prose">{car.business_case || <span className="muted">none</span>}</dd>
            <dt>Estimate</dt>
            <dd>
              {view.estimate ? `AACE class ${view.estimate.aace_class}, TIC ${money(view.estimate.tic, view.estimate.currency)}, as of ${view.estimate.as_of.slice(0, 10)}${view.estimate.basis_of_estimate_doc_ref ? `, BOE ${view.estimate.basis_of_estimate_doc_ref}` : ""}` : <span className="muted">none attached</span>}
            </dd>
            <dt>Created</dt>
            <dd>
              {when(car.created_at)} by {car.created_by}
            </dd>
            {car.authorized_amount && (
              <>
                <dt>Authorized</dt>
                <dd>
                  {money(car.authorized_amount, car.currency)} at {when(car.authorized_at)}
                </dd>
              </>
            )}
          </dl>
        </div>

        <aside className="card rail" aria-label="Guidance">
          <h2>Guidance</h2>
          {!guidance ? (
            <p className="muted">Not available for this state.</p>
          ) : !guidance.guidance.available ? (
            <p className="muted">The governance system did not answer; the page renders without cards. {guidance.guidance.error ? <span title={guidance.guidance.error}>(details in the logs)</span> : null}</p>
          ) : guidance.guidance.items.length === 0 ? (
            <p className="muted">No constraints surfaced for this request.</p>
          ) : (
            <ul className="cards">
              {guidance.guidance.items.map((it, i) => (
                <li key={`${it.constraint_code ?? "c"}-${i}`} className={`gcard ${it.priority}`}>
                  <div className="gmeta">
                    <span className="badge">{it.priority}</span> <span className="muted">{it.type}</span>
                    {it.constraint_code && <code>{it.constraint_code}</code>}
                    {typeof it.tier === "number" && <span className="muted">tier {it.tier}</span>}
                  </div>
                  <p>{it.content}</p>
                  <p className="muted small">
                    confidence {(it.confidence * 100).toFixed(0)}%{it.escalation_level ? `, ${it.escalation_level}` : ""}
                  </p>
                </li>
              ))}
            </ul>
          )}
          {guidance && (
            <div className="preview">
              <h3>Routing preview</h3>
              <p>
                Routing amount {money(guidance.preview.routing_amount, car.currency)}, governance tier <strong>{guidance.preview.tier}</strong>.
              </p>
              <ol>
                {guidance.preview.chain.map((s) => (
                  <li key={s.order}>
                    {s.role_required} <span className="muted">({s.pool}, {s.sla_hours} h)</span>
                  </li>
                ))}
              </ol>
            </div>
          )}
        </aside>
      </section>

      {car.status === "draft" && <SubmitPanel carID={car.car_id} guidanceIDs={guidance?.guidance.guidance_ids ?? []} />}

      {view.workflow && (
        <section className="card">
          <h2>
            Approval chain <span className={`badge ${view.workflow.status}`}>{view.workflow.status}</span>
          </h2>
          <table>
            <thead>
              <tr>
                <th>#</th>
                <th>Role</th>
                <th>Assignees</th>
                <th>Opened</th>
                <th>Due</th>
                <th>SLA</th>
                <th>Decision</th>
              </tr>
            </thead>
            <tbody>
              {view.stages.map((s) => (
                <tr key={s.stage_id} className={current?.stage_id === s.stage_id ? "current" : ""}>
                  <td>{s.stage_order}</td>
                  <td>{s.role_required}</td>
                  <td>{s.assignees.join(", ")}</td>
                  <td className="muted">{when(s.entered_at)}</td>
                  <td className="muted">{when(s.stage_due_at)}</td>
                  <td>{slaLabel(s) && <span className={`badge ${slaLabel(s).replace(" ", "-")}`}>{slaLabel(s)}</span>}</td>
                  <td>
                    {s.decision ? (
                      <>
                        <strong>{s.decision}</strong> by {s.completed_by} {when(s.completed_at)}
                        {s.comment ? <span className="muted"> "{s.comment}"</span> : null}
                      </>
                    ) : (
                      <span className="muted">pending</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {myTurn && current && <DecidePanel carID={car.car_id} stage={current.stage_order} role={current.role_required} guidanceID={guidance?.guidance.guidance_ids?.[0] ?? ""} />}
        </section>
      )}

      {signature && signature.mode !== "none" && (
        <section className="card">
          <h2>Signature</h2>
          {signature.mode === "attestation" ? (
            <p>
              Below the e-signature threshold: the audited approval is the attestation of record (<code>{signature.signed_artifact_ref}</code>).
            </p>
          ) : (
            <>
              <p>
                E-signature envelope <span className={`badge ${signature.envelope?.status ?? "requested"}`}>{signature.envelope?.status ?? "requested"}</span>
                {signature.envelope?.provider ? ` via ${signature.envelope.provider}` : ""}
                {signature.envelope?.provider_ref ? <span className="muted"> ({signature.envelope.provider_ref})</span> : null}
              </p>
              {signature.envelope?.signers?.length ? (
                <ul>
                  {signature.envelope.signers.map((sg) => (
                    <li key={`${sg.user_id}-${sg.role}`}>
                      {sg.user_id} <span className="muted">({sg.role})</span>: {sg.status}
                      {sg.signed_at ? ` ${when(sg.signed_at)}` : ""}
                    </li>
                  ))}
                </ul>
              ) : null}
              {signature.artifact_available && (
                <p>
                  <a className="button" href={`${corePublicURL}/api/v1/cars/${car.car_id}/signed-artifact`}>
                    Download signed PDF
                  </a>{" "}
                  <span className="muted small">SHA-256 {signature.envelope?.artifact_sha256}</span>
                </p>
              )}
            </>
          )}
        </section>
      )}

      {carAudit.length > 0 && (
        <section className="card">
          <h2>
            Audit trail <span className="muted">{audit?.full ? "(full)" : "(summary)"}</span>
          </h2>
          <table>
            <thead>
              <tr>
                <th>When</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Guidance</th>
                <th>Verdict</th>
              </tr>
            </thead>
            <tbody>
              {carAudit.map((e) => (
                <tr key={e.audit_id}>
                  <td className="muted">{when(e.at)}</td>
                  <td>{e.actor_user_id}</td>
                  <td>{e.action_type}</td>
                  <td className="muted small">{e.guidance_id ?? ""}</td>
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
