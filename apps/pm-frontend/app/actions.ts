"use server";

// Server actions: the browser posts a form to Next.js, which calls pm-core as
// the signed-in user and re-renders. Errors come back as a message the page
// shows beside the form; governance denials carry their codes.
import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { ApiError, api, type CARView } from "./lib/api";

export type ActionState = { error?: string; code?: string; ok?: string } | undefined;

function fail(e: unknown): ActionState {
  if (e instanceof ApiError) {
    const gov = (e.body as { governance?: { violated_codes?: string[]; tier?: string; user_response?: string } } | undefined)?.governance;
    const extra = gov?.violated_codes?.length ? ` (${gov.violated_codes.join(", ")})` : "";
    return { error: e.message + extra, code: e.code };
  }
  if (e && typeof e === "object" && "digest" in e) throw e; // Next.js redirects pass through
  return { error: e instanceof Error ? e.message : String(e) };
}

export async function createCAR(_prev: ActionState, form: FormData): Promise<ActionState> {
  const projectID = String(form.get("project_id") ?? "");
  const returnTo = `/projects/${projectID}/cars/new`;
  try {
    let estimateID = String(form.get("estimate_id") ?? "").trim() || undefined;
    const tic = String(form.get("estimate_tic") ?? "").trim();
    if (!estimateID && tic) {
      const est = await api<{ estimate_id: string }>(`/api/v1/projects/${projectID}/estimates`, {
        method: "POST",
        returnTo,
        body: JSON.stringify({
          aace_class: Number(form.get("estimate_class") ?? 3),
          as_of: String(form.get("estimate_as_of") ?? new Date().toISOString().slice(0, 10)),
          tic,
          basis_of_estimate_doc_ref: String(form.get("estimate_boe") ?? "").trim() || null,
        }),
      });
      estimateID = est.estimate_id;
    }
    const v = await api<CARView>(`/api/v1/projects/${projectID}/cars`, {
      method: "POST",
      returnTo,
      body: JSON.stringify({
        code: String(form.get("code") ?? "").trim() || null,
        kind: String(form.get("kind") ?? "full"),
        amount_requested: String(form.get("amount_requested") ?? "").trim(),
        business_case: String(form.get("business_case") ?? ""),
        estimate_id: estimateID ?? null,
      }),
    });
    revalidatePath(`/projects/${projectID}`);
    redirect(`/cars/${v.car.car_id}`);
  } catch (e) {
    return fail(e);
  }
}

export async function createPromise(_prev: ActionState, form: FormData): Promise<ActionState> {
  const projectID = String(form.get("project_id") ?? "");
  try {
    await api(`/api/v1/projects/${projectID}/promises`, {
      method: "POST",
      returnTo: `/projects/${projectID}`,
      body: JSON.stringify({
        kind: String(form.get("kind") ?? "loss_recovery"),
        metric_id: String(form.get("metric_id") ?? "").trim(),
        baseline_value: String(form.get("baseline_value") ?? "").trim() || null,
        baseline_period: String(form.get("baseline_period") ?? "").trim() || null,
        promised_value: String(form.get("promised_value") ?? "").trim() || null,
        dollar_value_per_year: String(form.get("dollar_value_per_year") ?? "").trim() || null,
        measurement_basis: String(form.get("measurement_basis") ?? ""),
        status: form.get("committed") ? "committed" : "proposed",
      }),
    });
    revalidatePath(`/projects/${projectID}`);
    return { ok: "Promise recorded." };
  } catch (e) {
    return fail(e);
  }
}

export async function preclassifyCAR(_prev: ActionState, form: FormData): Promise<ActionState> {
  const carID = String(form.get("car_id") ?? "");
  try {
    const r = await api<{ tier: string; verdict?: string; cached: boolean; unavailable: boolean; error?: string }>(`/api/v1/cars/${carID}/preclassify`, { method: "POST", returnTo: `/cars/${carID}` });
    revalidatePath(`/cars/${carID}`);
    if (r.unavailable) return { error: `Governance unavailable (tier ${r.tier}): ${r.error ?? ""}`.trim(), code: "governance_unavailable" };
    return { ok: `Pre-classified: ${r.verdict ?? "n/a"} (tier ${r.tier}${r.cached ? ", cached" : ""}).` };
  } catch (e) {
    return fail(e);
  }
}

export async function submitCAR(_prev: ActionState, form: FormData): Promise<ActionState> {
  const carID = String(form.get("car_id") ?? "");
  const guidanceIDs = String(form.get("guidance_ids") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  const dismissReason = String(form.get("dismiss_reason") ?? "").trim();
  try {
    await api(`/api/v1/cars/${carID}/submit`, {
      method: "POST",
      returnTo: `/cars/${carID}`,
      body: JSON.stringify({ guidance_ids: guidanceIDs, dismissed: !!form.get("dismissed"), dismiss_reason: dismissReason }),
    });
    revalidatePath(`/cars/${carID}`);
    revalidatePath("/inbox");
    return { ok: "Submitted. The approval chain is open." };
  } catch (e) {
    return fail(e);
  }
}

export async function decideCAR(_prev: ActionState, form: FormData): Promise<ActionState> {
  const carID = String(form.get("car_id") ?? "");
  const decision = String(form.get("decision") ?? "approve");
  try {
    await api(`/api/v1/cars/${carID}/decide`, {
      method: "POST",
      returnTo: `/cars/${carID}`,
      body: JSON.stringify({ decision, comment: String(form.get("comment") ?? ""), guidance_id: String(form.get("guidance_id") ?? "").trim() || null }),
    });
    revalidatePath(`/cars/${carID}`);
    revalidatePath("/inbox");
    return { ok: decision === "approve" ? "Approved." : "Rejected." };
  } catch (e) {
    return fail(e);
  }
}

// signOut ends the pm-core session (which cascades to the IdP), drops the cookie
// from the browser, and lands on the IdP's end-session page when there is one.
export async function signOut(): Promise<void> {
  let endSession = "";
  try {
    const r = await api<{ end_session_url?: string }>("/auth/logout", { method: "POST", returnTo: "/" });
    endSession = r.end_session_url ?? "";
  } catch (e) {
    if (e && typeof e === "object" && "digest" in e) throw e;
    // the session may already be gone; land on the home page either way
  }
  (await cookies()).delete("pm_session");
  redirect(endSession || "/");
}
