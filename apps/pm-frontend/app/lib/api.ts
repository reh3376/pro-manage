// Server-side access to pm-core (FSD §05: REST to the frontend). Every call
// forwards the browser's pm_session cookie, so pm-core resolves the actor and
// the §04 ACL decides what comes back; the frontend never holds a token.
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export const coreURL = process.env.PM_CORE_URL ?? "http://127.0.0.1:8080";
export const corePublicURL = process.env.PM_CORE_PUBLIC_URL ?? "http://localhost:8080";
// Where the browser reaches this app; pm-core's callback lands here after sign-in (its return_to
// allow-list holds this origin and its own).
export const frontendURL = (process.env.PM_FRONTEND_URL ?? "http://localhost:3100").replace(/\/$/, "");

/** loginURL is pm-core's sign-in entry that lands back on the given frontend path. */
export function loginURL(returnTo: string): string {
  return `${corePublicURL}/auth/login?return_to=${encodeURIComponent(frontendURL + returnTo)}`;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public body?: unknown,
  ) {
    super(message);
  }
}

async function sessionHeader(): Promise<Record<string, string>> {
  const jar = await cookies();
  const session = jar.get("pm_session")?.value;
  return session ? { cookie: `pm_session=${session}` } : {};
}

/** api calls pm-core as the signed-in user. 401 redirects to sign-in; other errors throw ApiError. */
export async function api<T>(path: string, init: RequestInit & { returnTo?: string } = {}): Promise<T> {
  const headers: Record<string, string> = { accept: "application/json", ...(await sessionHeader()), ...(init.headers as Record<string, string> | undefined) };
  if (init.body && !("content-type" in headers)) headers["content-type"] = "application/json";
  const res = await fetch(`${coreURL}${path}`, { ...init, headers, cache: "no-store", signal: init.signal ?? AbortSignal.timeout(15000) });
  if (res.status === 401) {
    redirect(loginURL(init.returnTo ?? "/"));
  }
  const text = await res.text();
  const body = text ? safeJSON(text) : undefined;
  if (!res.ok) {
    const err = (body as { error?: { code?: string; message?: string } } | undefined)?.error;
    throw new ApiError(res.status, err?.code ?? `http_${res.status}`, err?.message ?? res.statusText, body);
  }
  return body as T;
}

/** tryApi returns null instead of throwing on a 403 or 404 (sections the caller cannot see). */
export async function tryApi<T>(path: string, init: RequestInit & { returnTo?: string } = {}): Promise<T | null> {
  try {
    return await api<T>(path, init);
  } catch (e) {
    if (e instanceof ApiError && (e.status === 403 || e.status === 404)) return null;
    throw e;
  }
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

// ---------------------------------------------------------------- shapes (pm-core JSON)

export type Me = {
  user: { user_id: string; username: string; subject: string; email: string; display_name: string };
  platform_roles: string[];
  pools: string[];
  attributes: Record<string, string>;
  session: { created_at: string; expires_at: string; last_seen_at: string };
};

export async function whoAmI(): Promise<Me | null> {
  const h = await sessionHeader();
  if (!h.cookie) return null;
  try {
    const res = await fetch(`${coreURL}/auth/me`, { headers: h, cache: "no-store", signal: AbortSignal.timeout(3000) });
    return res.ok ? ((await res.json()) as Me) : null;
  } catch {
    return null;
  }
}

export type Project = {
  project_id: string;
  code: string;
  name: string;
  description: string;
  site_id: string;
  currency: string;
  owner_user_id: string;
  lifecycle_state: string;
  fel_stage: string;
  project_type: string;
  onboarding_level: string;
  baseline_budget_total: string | null;
  start_date: string | null;
  target_end_date: string | null;
};

export type Access = {
  user_id: string;
  base_role: string;
  platform_roles: string[];
  stage_assignee: boolean;
  sections: Record<string, string>;
};

export type Member = { project_id: string; user_id: string; base_role: string; invited_by: string; invited_at: string; expires_at: string | null };

export type CAR = {
  car_id: string;
  project_id: string;
  code: string;
  kind: string;
  amount_requested: string;
  currency: string;
  basis: string;
  estimate_id: string | null;
  business_case: string;
  status: string;
  workflow_id: string | null;
  authorized_amount: string | null;
  authorized_at: string | null;
  signed_artifact_ref: string | null;
  created_at: string;
  created_by: string;
};

export type Stage = {
  stage_id: string;
  stage_order: number;
  role_required: string;
  assignees: string[];
  sla_minutes: number;
  entered_at: string | null;
  stage_due_at: string | null;
  completed_at: string | null;
  completed_by: string | null;
  decision: string | null;
  comment: string | null;
  nudged_at: string | null;
  overdue_at: string | null;
  escalated_at: string | null;
};

export type Workflow = { workflow_id: string; kind: string; status: string; current_stage: number; initiated_by: string; initiated_at: string; completed_at: string | null };

export type CARView = {
  car: CAR;
  estimate?: { estimate_id: string; aace_class: number; as_of: string; tic: string; currency: string; basis_of_estimate_doc_ref: string | null };
  workflow?: Workflow;
  stages: Stage[];
  cumulative_authorized: string;
  budget_release?: BudgetRelease;
};

export type GuidanceItem = { type: string; priority: string; content: string; confidence: number; constraint_code?: string; escalation_level?: string; tier?: number };

export type GuidanceView = {
  guidance: { available: boolean; warm: boolean; items: GuidanceItem[]; guidance_ids: string[]; error?: string };
  preview: { routing_amount: string; chain: { order: number; role_required: string; pool: string; sla_hours: number }[]; tier: string; policy: Record<string, unknown> };
  session_id: string;
};

export type InboxItem = Stage & { workflow_kind: string; subject_type: string; subject_id: string; project_id: string | null; overdue: boolean; sla_state: string };

export type AuditRow = {
  audit_id: string;
  seq: number;
  actor_user_id: string;
  action_type: string;
  subject_type: string;
  subject_id: string;
  at: string;
  guidance_id?: string | null;
  classification_verdict?: string | null;
  before_state?: unknown;
  after_state?: unknown;
};

export type Signature = {
  car_id: string;
  status: string;
  signed_artifact_ref: string | null;
  mode: "none" | "attestation" | "esignature";
  envelope?: { envelope_id: string; provider: string; provider_ref: string | null; status: string; signers: { user_id: string; role: string; status: string; signed_at?: string }[]; artifact_sha256: string | null; completed_at: string | null };
  artifact_available?: boolean;
};

export type BudgetVersion = { version_id: string; kind: string; version_ref: string; name: string; status: string; total: string; currency: string; as_of: string | null; approved_at: string | null; pulled_at: string };
export type BudgetRelease = { release_id: string; car_id: string; project_ref: string; amount: string; currency: string; status: string; erp_budget_ref: string | null; requested_at: string; released_at: string | null; last_error: string | null };
export type BudgetSummary = {
  erp_project_ref: string | null;
  erp_instance: string;
  synced_at: string | null;
  sync_error: string | null;
  currency: string;
  baseline: BudgetVersion | null;
  forecast: BudgetVersion | null;
  eac: BudgetVersion | null;
  versions: BudgetVersion[];
  actuals_total: string;
  actuals_rows: number;
  commitments_open: string;
  invoiced_total: string;
  commitments_rows: number;
  authorized_total: string;
  releases: BudgetRelease[];
  released_total: string;
  remaining_to_spend: string;
};

export type Promise_ = { promise_id: string; kind: string; metric_id: string; status: string; promised_value: string | null; measurement_basis: string };

export function money(v: string | null | undefined, currency?: string): string {
  if (!v) return "";
  const n = Number(v);
  const s = Number.isFinite(n) ? n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : v;
  return currency ? `${s} ${currency}` : s;
}

export function when(v: string | null | undefined): string {
  if (!v) return "";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("en-US", { dateStyle: "medium", timeStyle: "short" });
}
