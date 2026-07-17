import type { NextConfig } from "next";

const rawAPIOrigin = (
  process.env.API_ORIGIN ??
  (process.env.NODE_ENV === "development" ? process.env.NEXT_PUBLIC_API_URL : undefined)
)?.replace(/\/$/, "");

const configuredAPIOrigin = rawAPIOrigin && !/^https?:\/\//.test(rawAPIOrigin)
  ? `http://${rawAPIOrigin}`
  : rawAPIOrigin;

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    if (!configuredAPIOrigin) {
      // Production deployments can route /api through their reverse proxy.
      // Local development must set API_ORIGIN in .env.
      return [];
    }
    return [
      {
        source: "/api/:path*",
        destination: `${configuredAPIOrigin}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
