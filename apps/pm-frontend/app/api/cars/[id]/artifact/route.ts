import { cookies } from "next/headers";
import { coreURL } from "../../../../lib/api";

export const dynamic = "force-dynamic";

// The signed CAR artifact through the frontend (backlog B-16): the browser never
// fetches pm-core directly; the session cookie is forwarded and pm-core's budget
// read decides. Headers that name the file and its hash pass through.
export async function GET(_req: Request, ctx: { params: Promise<{ id: string }> }): Promise<Response> {
  const { id } = await ctx.params;
  if (!/^[0-9a-f-]{36}$/i.test(id)) return new Response("bad id", { status: 400 });
  const session = (await cookies()).get("pm_session")?.value;
  if (!session) return new Response("sign in", { status: 401 });
  const res = await fetch(`${coreURL}/api/v1/cars/${id}/signed-artifact`, { headers: { cookie: `pm_session=${session}` }, cache: "no-store" });
  const headers = new Headers();
  for (const name of ["content-type", "content-length", "content-disposition", "etag", "x-artifact-sha256"]) {
    const v = res.headers.get(name);
    if (v) headers.set(name, v);
  }
  headers.set("cache-control", "no-store");
  return new Response(res.body, { status: res.status, headers });
}
