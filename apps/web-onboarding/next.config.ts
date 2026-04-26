import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  env: {
    BFF_URL: process.env.BFF_URL ?? "http://bff-onboarding:8094",
    CHAT_URL: process.env.CHAT_URL ?? "http://agent-conversational:8106",
  },
};

export default nextConfig;
