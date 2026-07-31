import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: 'export',
  // Emit method/index.html so /method resolves on any static host
  // (and on deep-link/refresh), not just via client-side nav.
  trailingSlash: true,
  reactCompiler: true,
  images: {
    unoptimized: true, // Required for static export
  },
};

export default nextConfig;
