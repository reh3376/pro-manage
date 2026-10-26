"use client";

import { useActionState } from "react";
import { createPromise, type ActionState } from "../../actions";

export function PromiseForm({ projectID }: { projectID: string }) {
  const [state, action, pending] = useActionState<ActionState, FormData>(createPromise, undefined);
  return (
    <details className="sub">
      <summary>Add a promise</summary>
      <form action={action} className="form">
        <input type="hidden" name="project_id" value={projectID} />
        <label>
          Kind
          <select name="kind" defaultValue="loss_recovery">
            {["loss_recovery", "capacity", "cost_per_unit", "energy_per_unit", "quality", "safety", "compliance", "other"].map((k) => (
              <option key={k}>{k}</option>
            ))}
          </select>
        </label>
        <label>
          Metric id
          <input name="metric_id" placeholder="changeover_hours" />
        </label>
        <label>
          Baseline value
          <input name="baseline_value" placeholder="410" />
        </label>
        <label>
          Baseline period
          <input name="baseline_period" placeholder="FY2025" />
        </label>
        <label>
          Promised value
          <input name="promised_value" placeholder="120" />
        </label>
        <label>
          Dollar value per year
          <input name="dollar_value_per_year" placeholder="1100000.00" />
        </label>
        <label className="full">
          Measurement basis
          <textarea name="measurement_basis" rows={2} placeholder="hours of changeover per year from the line's downtime log" />
        </label>
        <label className="check">
          <input type="checkbox" name="committed" /> Committed (baseline and basis captured at CAR time)
        </label>
        <div className="actions">
          <button type="submit" disabled={pending}>
            {pending ? "Saving" : "Save promise"}
          </button>
          {state?.error && <span className="fail">{state.error}</span>}
          {state?.ok && <span className="ok">{state.ok}</span>}
        </div>
      </form>
    </details>
  );
}
