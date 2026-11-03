import { cookies } from "next/headers";
import { coreURL } from "../../lib/api";

export const dynamic = "force-dynamic";

// Same-origin proxy for pm-core's live-refresh streams (FSD §05 SSE), so the
// browser's EventSource stays inside connect-src 'self' and never sees pm-core
// directly. ?scope=inbox opens the inbox stream; ?project=<id> opens a project's.
// The session cookie is forwarded; pm-core's ACL decides.
export async function GET(req: Request): Promise<Response> {
  const url = new URL(req.url);
  const project = url.searchParams.get("project");
  const scope = url.searchParams.get("scope");
  let upstream: string;
  if (project && /^[0-9a-f-]{36}$/i.test(project)) upstream = `${coreURL}/api/v1/projects/${project}/stream`;
  else if (scope === "inbox") upstream = `${coreURL}/api/v1/inbox/stream`;
  else return new Response("scope=inbox or project=<id> required", { status: 400 });

  const session = (await cookies()).get("pm_session")?.value;
  if (!session) return new Response("sign in", { status: 401 });
  const res = await fetch(upstream, { headers: { cookie: `pm_session=${session}`, accept: "text/event-stream" }, cache: "no-store", signal: req.signal });
  if (!res.ok || !res.body) return new Response(await res.text(), { status: res.status });
  return new Response(res.body, {
    status: 200,
    headers: { "content-type": "text/event-stream; charset=utf-8", "cache-control": "no-store", "x-accel-buffering": "no" },
  });
}
