// Auth-helpers для UI E2E.
//
// Два режима:
//   1. seedFakeAuth() — JWT-shape в localStorage. UI рендерится (frontend
//      декодирует JWT без проверки подписи), но реальные API-запросы упадут с
//      401. Подходит для smoke-проверки структуры страниц.
//   2. loginViaApi() — реальный POST /v1/login на identity-service. Требует
//      существующего пользователя в БД (например, после register-flow).
//      Подходит для full-flow тестов.

import type { Page } from "@playwright/test";

export type RoleClaim =
  | "bank.applicant"
  | "bank.operator"
  | "bank.compliance_officer"
  | "bank.admin"
  | "platform.admin";

function base64url(s: string): string {
  return Buffer.from(s, "utf8")
    .toString("base64")
    .replace(/=+$/g, "")
    .replace(/\+/g, "-")
    .replace(/\//g, "_");
}

export function makeFakeJwt(opts: {
  sub?: string;
  tenant_id?: string;
  role?: RoleClaim;
  email?: string;
  type?: "access" | "refresh";
  expSec?: number;
}): string {
  const now = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = base64url(
    JSON.stringify({
      sub: opts.sub ?? "00000000-0000-0000-0000-000000000001",
      tenant_id: opts.tenant_id ?? "demo",
      role: opts.role ?? "bank.applicant",
      email: opts.email ?? "applicant@example.com",
      type: opts.type ?? "access",
      iat: now,
      exp: opts.expSec ?? now + 3600,
    })
  );
  return `${header}.${payload}.${base64url("stub-signature")}`;
}

export interface SeedAuthOptions {
  tenantId?: string;
  role?: RoleClaim;
  email?: string;
}

export async function seedFakeAuth(page: Page, opts: SeedAuthOptions = {}): Promise<void> {
  const tenantId = opts.tenantId ?? "demo";
  const role = opts.role ?? "bank.applicant";
  const access = makeFakeJwt({ tenant_id: tenantId, role, email: opts.email });
  const refresh = makeFakeJwt({
    tenant_id: tenantId,
    role,
    email: opts.email,
    type: "refresh",
    expSec: Math.floor(Date.now() / 1000) + 7 * 24 * 3600,
  });
  await page.addInitScript(
    ([a, r, t]) => {
      try {
        window.localStorage.setItem("aibank.access_token", a);
        window.localStorage.setItem("aibank.refresh_token", r);
        window.localStorage.setItem("aibank.tenant_id", t);
      } catch {
        /* SSR/private */
      }
    },
    [access, refresh, tenantId] as const
  );
}

export interface LoginViaApiOptions {
  email: string;
  password: string;
  tenantId: string;
  identityBaseUrl: string; // e.g. http://206.204.106.28:8082 или http://localhost:8082
}

/** Реальный логин через identity-service. Возвращает access_token. */
export async function loginViaApi(page: Page, opts: LoginViaApiOptions): Promise<string> {
  const resp = await page.request.post(`${opts.identityBaseUrl}/v1/auth/login`, {
    data: { email: opts.email, password: opts.password, tenant_id: opts.tenantId },
  });
  if (!resp.ok()) {
    throw new Error(`loginViaApi: HTTP ${resp.status()} ${await resp.text()}`);
  }
  const body = await resp.json();
  const access = body.access_token || body.token;
  const refresh = body.refresh_token || access;
  if (!access) throw new Error(`loginViaApi: no access_token in ${JSON.stringify(body)}`);

  await page.addInitScript(
    ([a, r, t]) => {
      try {
        window.localStorage.setItem("aibank.access_token", a);
        window.localStorage.setItem("aibank.refresh_token", r);
        window.localStorage.setItem("aibank.tenant_id", t);
      } catch {
        /* */
      }
    },
    [access, refresh, opts.tenantId] as const
  );
  return access;
}

export async function readStoredToken(page: Page): Promise<string | null> {
  return page.evaluate(() => window.localStorage.getItem("aibank.access_token"));
}
