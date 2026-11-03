import type { NextConfig } from "next";

// Baseline response hardening for the authenticated UI (FSD §05 pm-frontend, backlog B-16).
// Everything the pages need is same-origin: pm-core is reached from the server, the live
// streams and the signed PDF come through this app's own route handlers, and the only
// off-origin navigation is the sign-in link, which CSP does not govern. Next.js hydration
// still needs inline scripts and styles, hence 'unsafe-inline' on those two directives;
// a nonce-based policy is the follow-up once the pages stop being server-rendered only.
const csp = [
  "default-src 'self'",
  "script-src 'self' 'unsafe-inline'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "font-src 'self'",
  "connect-src 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "object-src 'none'",
].join("; ");

const nextConfig: NextConfig = {
  // Standalone output keeps the container image small (FSD §05 deployment).
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          { key: "Content-Security-Policy", value: csp },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "no-referrer" },
          { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
        ],
      },
    ];
  },
};

export default nextConfig;
