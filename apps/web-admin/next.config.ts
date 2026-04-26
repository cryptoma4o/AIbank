import type { NextConfig } from "next";
const nextConfig: NextConfig = {
  output: "standalone",
  env: {
    BFF_ADMIN_URL: process.env.BFF_ADMIN_URL ?? "http://bff-admin:8095",
  },
};
export default nextConfig;
