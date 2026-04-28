// next.config.mjs — экспонируем GraphQL- и identity-эндпоинты в браузер.
//
// NEXT_PUBLIC_* переменные доступны в client-bundle (Apollo Link, fetch для
// логина). Server-only фолбэки оставлены на случай SSR или e2e-инструментария.

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  env: {
    NEXT_PUBLIC_BFF_ONBOARDING_URL:
      process.env.NEXT_PUBLIC_BFF_ONBOARDING_URL ?? "http://localhost:8091/graphql",
    NEXT_PUBLIC_IDENTITY_SERVICE_URL:
      process.env.NEXT_PUBLIC_IDENTITY_SERVICE_URL ?? "http://localhost:8082",
  },
};

export default nextConfig;
