import Link from "next/link";
import { api, type InboxItem, when } from "../lib/api";

export const dynamic = "force-dynamic";

const stateLabel: Record<string, string> = { ok: "on time", nudged: "nudged", overdue: "overdue", escalated: "escalated" };

// The workflow inbox (FSD §08 "In-app inbox"): every stage awaiting this user's decision, with its SLA state.
export default async function InboxPage() {
  const { items } = await api<{ items: InboxItem[] }>("/api/v1/inbox", { returnTo: "/inbox" });
  return (
    <main>
      <h1>Inbox</h1>
      <p className="muted">Stages waiting on your decision. Rows clear when you decide, never by dismissal.</p>
      {items.length === 0 ? (
        <section className="card">
          <p>Nothing waits on you.</p>
        </section>
      ) : (
        <section className="card">
          <table>
            <thead>
              <tr>
                <th>Subject</th>
                <th>Stage</th>
                <th>Due</th>
                <th>SLA</th>
              </tr>
            </thead>
            <tbody>
              {items.map((it) => (
                <tr key={it.stage_id}>
                  <td>
                    {it.subject_type === "capital_authorization" ? <Link href={`/cars/${it.subject_id}`}>CAR {it.subject_id.slice(0, 8)}</Link> : `${it.subject_type} ${it.subject_id}`}
                  </td>
                  <td>
                    {it.stage_order} <span className="muted">({it.role_required})</span>
                  </td>
                  <td>{when(it.stage_due_at)}</td>
                  <td>
                    <span className={`badge ${it.sla_state}`}>{stateLabel[it.sla_state] ?? it.sla_state}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </main>
  );
}
