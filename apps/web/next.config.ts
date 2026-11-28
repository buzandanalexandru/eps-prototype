import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./src/i18n/request.ts");

const nextConfig: NextConfig = {
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  poweredByHeader: false,
  experimental: {
    // Shop pages are static (cached until publish), so they cannot carry a per-request nonce:
    // their scripts are pinned by Subresource Integrity instead (design D3.6).
    sri: { algorithm: "sha256" },
    // Two root layouts (platform and shop): one 404 for addresses that match no route.
    globalNotFound: true,
  },
};

export default withNextIntl(nextConfig);
