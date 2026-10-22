import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Standalone output keeps the container image small (FSD §05 deployment).
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
};

export default nextConfig;
