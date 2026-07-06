/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    return [
      {
        source: "/api/py/:path*",
        destination: `${process.env.NEXT_PUBLIC_PY_PLANNER_URL || "http://localhost:8000"}/:path*`,
      },
      {
        source: "/api/go/:path*",
        destination: `${process.env.NEXT_PUBLIC_GO_ENGINE_URL || "http://localhost:8080"}/:path*`,
      },
    ];
  },
};

module.exports = nextConfig;