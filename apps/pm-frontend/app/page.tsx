import { cookies, headers } from "next/headers";

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

type Me = { user: { username: string; display_name: string }; platform_roles: string[]; pools: string[] } | null;

// The session cookie is host-scoped (ports do not matter), so the browser sends pm-core's cookie
// here too; forwarding it to /auth/me tells the page who is signed in (FSD §04 session model).
async function whoAmI(): Promise<Me> {
  const jar = await cookies();
  const session = jar.get("pm_session")?.value;
  if (!session) return null;
  try {
    const res = await fetch(`${probes[0].url}/auth/me`, { cache: "no-store", headers: { cookie: `pm_session=${session}` }, signal: AbortSignal.timeout(2000) });
    return res.ok ? ((await res.json()) as Me) : null;
  } catch {
    return null;
  }
}

export const dynamic = "force-dynamic";

export default async function Home() {
  await headers(); // opt into request-time rendering
  const [results, me] = await Promise.all([Promise.all(probes.map(probe)), whoAmI()]);
  const corePublic = process.env.PM_CORE_PUBLIC_URL ?? "http://localhost:8080";
  return (
    <main>
      <h1>pro_manage</h1>
      <p className="muted">
        Spine development. Seed company: Northfield Spirits Co. (NF-KY, NF-ON).
      </p>
      <section className="status" aria-label="Sign-in">
        {me ? (
          <p>
            Signed in as <strong>{me.user.display_name || me.user.username}</strong> ({me.user.username}); roles:{" "}
            {me.platform_roles.length ? me.platform_roles.join(", ") : "none"}; pools: {me.pools.length ? me.pools.join(", ") : "none"}.{" "}
            <a href={`${corePublic}/auth/me`}>session</a>
          </p>
        ) : (
          <p>
            <a href={`${corePublic}/auth/login?return_to=${encodeURIComponent("http://localhost:3100/")}`}>Sign in</a> with the company
            identity provider (dev realm: alice.owner, bob.finance, carol.admin, dana.policy, evan.exec, fay.ops, gus.maint, hana.epcm / northfield-dev).
          </p>
        )}
      </section>
      <section className="status" aria-label="Service status">
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
