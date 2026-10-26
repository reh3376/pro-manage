"use client";

import { useActionState } from "react";
import { createCAR, type ActionState } from "../../../../actions";

export function CARForm({ projectID, currency }: { projectID: string; currency: string }) {
  const [state, action, pending] = useActionState<ActionState, FormData>(createCAR, undefined);
  const today = new Date().toISOString().slice(0, 10);
  return (
    <form action={action} className="form card">
      <input type="hidden" name="project_id" value={projectID} />
      <label>
        Kind
        <select name="kind" defaultValue="full">
          <option value="fel_funding">fel_funding (FEL2 and FEL3 work)</option>
          <option value="full">full (execution)</option>
          <option value="supplemental">supplemental (cost growth)</option>
        </select>
      </label>
      <label>
        Code <span className="muted">(blank for CR-n)</span>
        <input name="code" placeholder="CR-2457" />
      </label>
      <label>
        Amount requested ({currency})
        <input name="amount_requested" required placeholder="42000000.00" inputMode="decimal" />
      </label>
      <label className="full">
        Business case
        <textarea name="business_case" rows={4} required placeholder="What the money buys, the priced loss or benefit, the payback." />
      </label>
      <fieldset className="full">
        <legend>Estimate behind the request</legend>
        <p className="muted">Give an existing estimate id, or enter one here and it is recorded with the CAR.</p>
        <label>
          Existing estimate id
          <input name="estimate_id" placeholder="uuid" />
        </label>
        <label>
          AACE class
          <select name="estimate_class" defaultValue="3">
            {[5, 4, 3, 2, 1].map((c) => (
              <option key={c} value={c}>
                Class {c}
              </option>
            ))}
          </select>
        </label>
        <label>
          As of
          <input name="estimate_as_of" type="date" defaultValue={today} />
        </label>
        <label>
          TIC ({currency})
          <input name="estimate_tic" placeholder="42000000.00" inputMode="decimal" />
        </label>
        <label>
          Basis-of-estimate document
          <input name="estimate_boe" placeholder="DOC-BOE-R3" />
        </label>
      </fieldset>
      <div className="actions full">
        <button type="submit" disabled={pending}>
          {pending ? "Saving" : "Save draft"}
        </button>
        {state?.error && <span className="fail">{state.error}</span>}
      </div>
    </form>
  );
}
