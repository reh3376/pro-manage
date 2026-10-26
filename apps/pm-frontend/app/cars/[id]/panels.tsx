"use client";

import { useActionState } from "react";
import { decideCAR, preclassifyCAR, submitCAR, type ActionState } from "../../actions";

// The governed submit (FSD §06): pre-classify the settled form, then submit
// carrying the guidance ids the user saw, with an optional dismissal reason.
export function SubmitPanel({ carID, guidanceIDs }: { carID: string; guidanceIDs: string[] }) {
  const [pre, preAction, prePending] = useActionState<ActionState, FormData>(preclassifyCAR, undefined);
  const [sub, subAction, subPending] = useActionState<ActionState, FormData>(submitCAR, undefined);
  return (
    <section className="card">
      <h2>Submit for approval</h2>
      <p className="muted">
        Submission is the governed action: the deterministic preconditions run first, then the governance classify under the request's tier. Pre-classifying now makes the submit instant.
      </p>
      <div className="row">
        <form action={preAction}>
          <input type="hidden" name="car_id" value={carID} />
          <button type="submit" className="secondary" disabled={prePending}>
            {prePending ? "Checking" : "Pre-classify"}
          </button>
        </form>
        <form action={subAction} className="inline">
          <input type="hidden" name="car_id" value={carID} />
          <input type="hidden" name="guidance_ids" value={guidanceIDs.join(",")} />
          <label className="check">
            <input type="checkbox" name="dismissed" /> I read the guidance and proceed anyway
          </label>
          <input name="dismiss_reason" placeholder="reason, if dismissing" />
          <button type="submit" disabled={subPending}>
            {subPending ? "Submitting" : "Submit"}
          </button>
        </form>
      </div>
      {pre?.ok && <p className="ok">{pre.ok}</p>}
      {pre?.error && <p className="fail">{pre.error}</p>}
      {sub?.ok && <p className="ok">{sub.ok}</p>}
      {sub?.error && (
        <p className="fail">
          {sub.code === "governance_denied" ? "Blocked by governance: " : sub.code === "governance_unavailable" ? "Governance unavailable (Tier A refuses): " : ""}
          {sub.error}
        </p>
      )}
    </section>
  );
}

export function DecidePanel({ carID, stage, role, guidanceID }: { carID: string; stage: number; role: string; guidanceID: string }) {
  const [state, action, pending] = useActionState<ActionState, FormData>(decideCAR, undefined);
  return (
    <form action={action} className="decide">
      <input type="hidden" name="car_id" value={carID} />
      <input type="hidden" name="guidance_id" value={guidanceID} />
      <p>
        <strong>Your decision</strong> for stage {stage} ({role}).
      </p>
      <input name="comment" placeholder="comment (optional)" />
      <button type="submit" name="decision" value="approve" disabled={pending}>
        Approve
      </button>
      <button type="submit" name="decision" value="reject" className="danger" disabled={pending}>
        Reject
      </button>
      {state?.ok && <span className="ok">{state.ok}</span>}
      {state?.error && <span className="fail">{state.error}</span>}
    </form>
  );
}
