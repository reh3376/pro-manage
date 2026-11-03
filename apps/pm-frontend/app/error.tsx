"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { startTransition, useEffect } from "react";

// The route error boundary (backlog B-16): when pm-core does not answer or a
// page throws, the shell (header, navigation) stays and the reader gets a plain
// explanation and a retry instead of a blank page.
export default function RouteError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const router = useRouter();
  // reset() alone re-renders the client boundary with the same server payload; the server
  // components have to be fetched again for a recovered pm-core to show, hence the refresh.
  const retry = () => startTransition(() => { router.refresh(); reset(); });
  useEffect(() => {
    console.error("page error", error);
  }, [error]);
  const digest = error.digest ?? "";
  const unreachable = digest === "PM_CORE_UNREACHABLE" || /fetch failed|ECONNREFUSED|timeout|aborted/i.test(error.message);
  const coreFailed = digest.startsWith("PM_CORE_") && !unreachable;
  return (
    <main>
      <h1>{unreachable ? "pm-core did not answer" : coreFailed ? "pm-core reported an error" : "Something went wrong on this page"}</h1>
      <section className="card">
        <p>
          {unreachable
            ? "The page could not reach the core service. Your session and your work on the server are unaffected; try again in a moment."
            : coreFailed
              ? `The core service answered with a server error (${digest.replace("PM_CORE_", "HTTP ")}). The request id is in pm-core's logs; try again, and tell the platform admin if it persists.`
              : "The page hit an error while rendering. Nothing you submitted was lost: every change is written on the server before the page re-renders."}
        </p>
        <p className="muted small">{error.message}{error.digest ? ` (ref ${error.digest})` : ""}</p>
        <div className="actions">
          <button type="button" onClick={retry}>
            Try again
          </button>
          <Link href="/">Home</Link>
          <Link href="/inbox">Inbox</Link>
        </div>
      </section>
    </main>
  );
}
