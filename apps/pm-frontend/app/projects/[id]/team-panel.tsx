"use client";

import { useActionState } from "react";
import { addGrant, addMember, removeGrant, removeMember, type ActionState } from "../../actions";
import type { Grant, Member } from "../../lib/api";

const sections = ["overview", "team", "budget", "schedule", "scope", "risk", "issues", "change_requests", "documents", "metrics", "approvals", "audit", "governance", "settings"];

// Team and grant management (FSD §04, backlog B-16): the owner, or anyone with a
// team write, invites or re-roles members, removes them (a project keeps at least
// one owner), and adds or removes section grants. Every change is audited by pm-core.
export function TeamPanel({ projectID, members, grants, canWrite }: { projectID: string; members: Member[]; grants: Grant[]; canWrite: boolean }) {
  const [mState, mAction, mPending] = useActionState<ActionState, FormData>(addMember, undefined);
  const [gState, gAction, gPending] = useActionState<ActionState, FormData>(addGrant, undefined);
  const [rmState, rmAction] = useActionState<ActionState, FormData>(removeMember, undefined);
  const [rgState, rgAction] = useActionState<ActionState, FormData>(removeGrant, undefined);
  return (
    <>
      <table>
        <thead>
          <tr>
            <th>User</th>
            <th>Base role</th>
            <th>Since</th>
            <th>Expires</th>
            {canWrite && <th />}
          </tr>
        </thead>
        <tbody>
          {members.map((m) => (
            <tr key={m.user_id}>
              <td>{m.user_id}</td>
              <td>{m.base_role}</td>
              <td className="muted">{new Date(m.invited_at).toLocaleDateString()}</td>
              <td className="muted">{m.expires_at ? new Date(m.expires_at).toLocaleDateString() : ""}</td>
              {canWrite && (
                <td className="num">
                  <form action={rmAction}>
                    <input type="hidden" name="project_id" value={projectID} />
                    <input type="hidden" name="user_id" value={m.user_id} />
                    <button type="submit" className="linklike danger-text" aria-label={`Remove ${m.user_id}`}>
                      remove
                    </button>
                  </form>
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
      {rmState?.error && <p className="fail">{rmState.error}</p>}
      {canWrite && (
        <form action={mAction} className="inline sub">
          <input type="hidden" name="project_id" value={projectID} />
          <label>
            Invite
            <input name="user_id" placeholder="login handle, e.g. fay.ops" required />
          </label>
          <label>
            as
            <select name="base_role" defaultValue="viewer">
              <option value="viewer">viewer</option>
              <option value="editor">editor</option>
              <option value="contributor">contributor</option>
              <option value="owner">owner</option>
            </select>
          </label>
          <label>
            until
            <input name="expires_at" type="date" />
          </label>
          <button type="submit" disabled={mPending}>
            {mPending ? "Saving" : "Add member"}
          </button>
          {mState?.error && <span className="fail">{mState.error}</span>}
          {mState?.ok && <span className="ok">{mState.ok}</span>}
        </form>
      )}

      <h3>Section grants</h3>
      <p className="muted small">A grant adds to or masks the base role on one section; deny wins. Document and field grants are stored but not yet evaluated.</p>
      {grants.length === 0 ? (
        <p className="muted">No grants.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>User</th>
              <th>Scope</th>
              <th>Permission</th>
              <th>Reason</th>
              <th>Expires</th>
              {canWrite && <th />}
            </tr>
          </thead>
          <tbody>
            {grants.map((g) => (
              <tr key={g.grant_id}>
                <td>{g.user_id}</td>
                <td>
                  <code>{g.scope_target}</code>
                </td>
                <td>
                  <span className={`badge ${g.permission}`}>{g.permission}</span>
                </td>
                <td className="muted">{g.reason}</td>
                <td className="muted">{g.expires_at ? new Date(g.expires_at).toLocaleDateString() : ""}</td>
                {canWrite && (
                  <td className="num">
                    <form action={rgAction}>
                      <input type="hidden" name="project_id" value={projectID} />
                      <input type="hidden" name="grant_id" value={g.grant_id} />
                      <button type="submit" className="linklike danger-text" aria-label={`Remove grant ${g.grant_id}`}>
                        remove
                      </button>
                    </form>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {rgState?.error && <p className="fail">{rgState.error}</p>}
      {canWrite && (
        <form action={gAction} className="inline sub">
          <input type="hidden" name="project_id" value={projectID} />
          <label>
            Grant
            <input name="user_id" placeholder="login handle" required />
          </label>
          <label>
            on
            <select name="section" defaultValue="budget">
              {sections.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </label>
          <label>
            permission
            <select name="permission" defaultValue="read">
              <option value="read">read</option>
              <option value="write">write</option>
              <option value="read_full">read_full</option>
              <option value="deny">deny</option>
            </select>
          </label>
          <label>
            reason
            <input name="reason" placeholder="why" required />
          </label>
          <label>
            until
            <input name="expires_at" type="date" />
          </label>
          <button type="submit" disabled={gPending}>
            {gPending ? "Saving" : "Add grant"}
          </button>
          {gState?.error && <span className="fail">{gState.error}</span>}
          {gState?.ok && <span className="ok">{gState.ok}</span>}
        </form>
      )}
    </>
  );
}
