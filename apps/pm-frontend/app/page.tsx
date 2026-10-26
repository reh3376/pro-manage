import { headers } from "next/headers";
import Link from "next/link";
import { whoAmI } from "./lib/api";

type Probe = { name: string; url: string };

const probes: Probe[] = [
  { name: "pm-core", url: process.env.PM_CORE_URL ?? "http://127.0.0.1:8080" },
  { name: "pm-workers", url: process.env.PM_WORKERS_URL ?? "http://127.0.0.1:8081" },
  { name: "pm-sidecar", url: process.env.PM_SIDECAR_URL ?? "http://127.0.0.1:7070" },
];

async function probe(p: Probe): Promise<{ name: string; ok: boolean; detail: string }> {
  try {
    const res = await fetch(`${p.url}/healthz`, { cache: "no-store", signal: AbortSignal.timeout(2000) });
    const body = (await res.json()) as { version?: string };
    return { name: p.name, ok: res.ok, detail: res.ok ? `v${body.version ?? "?"}` : `HTTP ${res.status}` };
  } catch (err) {
    return { name: p.name, ok: false, detail: err instanceof Error ? err.message : "unreachable" };
  }
}

export const dynamic = "force-dynamic";

export default async function Home() {
  await headers(); // opt into request-time rendering
  const [results, me] = await Promise.all([Promise.all(probes.map(probe)), whoAmI()]);
  return (
    <main>
      <h1>pro_manage</h1>
      <p className="muted">Spine development. Seed company: Northfield Spirits Co. (NF-KY, NF-ON).</p>
      <section className="card" aria-label="Start">
        {me ? (
          <p>
            Signed in as <strong>{me.user.display_name || me.user.username}</strong>. Roles: {me.platform_roles.length ? me.platform_roles.join(", ") : "none"}; pools:{" "}
            {me.pools.length ? me.pools.join(", ") : "none"}. Go to your <Link href="/inbox">inbox</Link> or the <Link href="/projects">projects</Link> you can see.
          </p>
        ) : (
          <p>
            Sign in with the company identity provider (top right). Dev realm users: alice.owner, bob.finance, carol.admin, dana.policy, evan.exec, fay.ops, gus.maint,
            hana.epcm, password northfield-dev.
          </p>
        )}
      </section>
      <section className="card" aria-label="Service status">
        <h2>Services</h2>
        <table>
          <tbody>
            {results.map((r) => (
              <tr key={r.name}>
                <td>{r.name}</td>
                <td className={r.ok ? "ok" : "fail"}>{r.ok ? "up" : "down"}</td>
                <td className="muted">{r.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  );
}
