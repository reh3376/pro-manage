import Link from "next/link";
import { notFound } from "next/navigation";
import { tryApi, type Project } from "../../../../lib/api";
import { CARForm } from "./car-form";

export const dynamic = "force-dynamic";

// The CAR form (roadmap: one screen per persona). Drafting is the owner's own;
// the governed action is the submit on the CAR page, where the guidance rail lives.
export default async function NewCARPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const project = await tryApi<Project>(`/api/v1/projects/${id}`, { returnTo: `/projects/${id}/cars/new` });
  if (!project) notFound();
  return (
    <main>
      <p className="crumbs">
        <Link href="/projects">Projects</Link> / <Link href={`/projects/${id}`}>{project.code}</Link> / new CAR
      </p>
      <h1>New capital authorization request</h1>
      <p className="muted">
        {project.code} {project.name}, {project.fel_stage}, {project.currency}. A full CAR needs a Class 3 estimate with a basis-of-estimate document and every promise committed; a FEL-funding CAR needs a Class 5 estimate and stays under the FEL cap (FSD §08).
      </p>
      <CARForm projectID={id} currency={project.currency} />
    </main>
  );
}
