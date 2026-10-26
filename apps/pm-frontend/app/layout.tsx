import type { Metadata } from "next";
import type { ReactNode } from "react";
import Link from "next/link";
import "./globals.css";
import { loginURL, whoAmI } from "./lib/api";
import { signOut } from "./actions";

export const metadata: Metadata = {
  title: "pro_manage",
  description: "Owner-side capital-project management for a TPM-driven process industry.",
};

export const dynamic = "force-dynamic";

export default async function RootLayout({ children }: { children: ReactNode }) {
  const me = await whoAmI();
  return (
    <html lang="en">
      <body>
        <header className="nav">
          <nav aria-label="Primary">
            <Link href="/" className="brand">
              pro_manage
            </Link>
            <Link href="/projects">Projects</Link>
            <Link href="/inbox">Inbox</Link>
          </nav>
          <div className="who">
            {me ? (
              <>
                <span title={me.platform_roles.join(", ") || "no platform roles"}>
                  {me.user.display_name || me.user.username} <span className="muted">({me.user.username})</span>
                </span>
                <form action={signOut}>
                  <button type="submit" className="linklike">
                    Sign out
                  </button>
                </form>
              </>
            ) : (
              <a href={loginURL("/inbox")}>Sign in</a>
            )}
          </div>
        </header>
        {children}
      </body>
    </html>
  );
}
