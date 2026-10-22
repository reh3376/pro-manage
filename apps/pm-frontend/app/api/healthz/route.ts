export const dynamic = "force-dynamic";

export function GET() {
  return Response.json({ status: "ok", service: "pm-frontend", version: process.env.PM_VERSION ?? "dev" });
}
