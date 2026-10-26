import Link from "next/link";
import { api, money, type Project } from "../lib/api";

export const dynamic = "force-dynamic";

// The projects the caller can see (membership, an open stage, or a platform role that reads across; FSD §04).
export default async function ProjectsPage() {
  const { items } = await api<{ items: Project[] }>("/api/v1/projects", { returnTo: "/projects" });
  return (
    <main>
      <h1>Projects</h1>
      <p className="muted">What you are a member of, what waits on you, or what your platform role lets you read.</p>
      <section className="card">
        {items.length === 0 ? (
          <p>No projects are visible to you yet. Ask a project owner to add you to the team.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Code</th>
                <th>Name</th>
                <th>State</th>
                <th>FEL</th>
                <th>Type</th>
                <th className="num">Baseline</th>
              </tr>
            </thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.project_id}>
                  <td>
                    <Link href={`/projects/${p.project_id}`}>{p.code}</Link>
                  </td>
                  <td>{p.name}</td>
                  <td>{p.lifecycle_state}</td>
                  <td>{p.fel_stage}</td>
                  <td>{p.project_type}</td>
                  <td className="num">{money(p.baseline_budget_total, p.currency)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </main>
  );
}
