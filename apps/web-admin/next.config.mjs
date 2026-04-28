// next.config.mjs — экспонируем env-переменные с префиксом NEXT_PUBLIC_,
// чтобы клиентский код мог обратиться к bff-admin и identity-service по
// корректным URL. Значения по умолчанию рассчитаны на локальный стенд
// docker-compose, где bff-admin слушает :8092, а identity-service :8082.

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  env: {
    NEXT_PUBLIC_BFF_ADMIN_URL:
      process.env.NEXT_PUBLIC_BFF_ADMIN_URL ?? "http://localhost:8092/graphql",
    NEXT_PUBLIC_IDENTITY_SERVICE_URL:
      process.env.NEXT_PUBLIC_IDENTITY_SERVICE_URL ?? "http://localhost:8082",
  },
};

export default nextConfig;
