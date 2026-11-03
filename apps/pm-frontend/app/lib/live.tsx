"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";

// LiveRefresh subscribes to a pm-core change stream through this app's /api/events
// proxy and re-renders the current server component tree when something relevant
// changed (FSD §05 SSE, §09 "the frontend fetches the fresh tile"). Hints only:
// the refresh re-fetches through the normal routes, so the ACL still decides.
// Bursts are coalesced; a dropped connection reconnects with EventSource's own
// backoff and the page stays usable without it.
export function LiveRefresh({ scope, projectId, label }: { scope?: "inbox"; projectId?: string; label?: string }) {
  const router = useRouter();
  const [state, setState] = useState<"connecting" | "live" | "offline">("connecting");
  const [lastChange, setLastChange] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const qs = projectId ? `project=${encodeURIComponent(projectId)}` : scope === "inbox" ? "scope=inbox" : "";
    if (!qs || typeof EventSource === "undefined") return;
    const es = new EventSource(`/api/events?${qs}`);
    es.onopen = () => setState("live");
    es.onerror = () => setState("offline");
    es.addEventListener("change", (ev) => {
      try {
        const data = JSON.parse((ev as MessageEvent).data) as { action_type?: string };
        setLastChange(data.action_type ?? "change");
      } catch {
        setLastChange("change");
      }
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => router.refresh(), 400);
    });
    return () => {
      es.close();
      if (timer.current) clearTimeout(timer.current);
    };
  }, [scope, projectId, router]);

  return (
    <span className={`live live-${state}`} title={lastChange ? `last change: ${lastChange}` : "live refresh"} aria-live="polite">
      <span className="dot" aria-hidden="true" /> {state === "live" ? (label ?? "live") : state === "offline" ? "offline, refresh manually" : "connecting"}
    </span>
  );
}
