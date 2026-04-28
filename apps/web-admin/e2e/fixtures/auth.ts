// fixtures/auth.ts — operator JWT seeding for web-admin.
//
// Operator roles per identity-service contract:
//   - bank.admin    (full access)
//   - bank.operator (review queue)
//   - bank.applicant (NOT allowed in admin → must be denied)

import type { Page } from "@playwright/test";

export type OperatorRole = "bank.admin" | "bank.operator" | "bank.applicant";

export interface OperatorClaims {
  sub: string;
  tenant_id: string;
  role: OperatorRole;
  email?: string;
  type?: "access" | "refresh";
  exp?: number;
  iat?: number;
}

function base64url(input: string): string {
  return Buffer.from(input, "utf8")
    .toString("base64")
    .replace(/=+$/g, "")
    .replace(/\+/g, "-")
    .replace(/\//g, "_");
}

export function makeOperatorJwt(claims: Partial<OperatorClaims> = {}): string {
  const nowSec = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = base64url(
    JSON.stringify({
      sub: claims.sub ?? "00000000-0000-0000-0000-0000000000aa",
      tenant_id: claims.tenant_id ?? "bank_alpha",
      role: claims.role ?? "bank.admin",
      email: claims.email ?? "admin@example.com",
      type: claims.type ?? "access",
      iat: claims.iat ?? nowSec,
      exp: claims.exp ?? nowSec + 3600,
    })
  );
  return `${header}.${payload}.${base64url("stub-signature")}`;
}

export interface SeedAdminAuthOptions {
  baseURL: string;
  tenantId?: string;
  role?: OperatorRole;
  email?: string;
}

export async function seedAdminAuth(
  page: Page,
  opts: SeedAdminAuthOptions
): Promise<void> {
  const tenantId = opts.tenantId ?? "bank_alpha";
  const role = opts.role ?? "bank.admin";
  const access = makeOperatorJwt({ tenant_id: tenantId, role, email: opts.email });
  const refresh = makeOperatorJwt({
    tenant_id: tenantId,
    role,
    email: opts.email,
    type: "refresh",
    exp: Math.floor(Date.now() / 1000) + 7 * 24 * 3600,
  });

  await page.addInitScript(
    ([a, r, t, role]) => {
      try {
        window.localStorage.setItem("aibank.access_token", a);
        window.localStorage.setItem("aibank.refresh_token", r);
        window.localStorage.setItem("aibank.tenant_id", t);
        window.localStorage.setItem("aibank.role", role);
      } catch {
        /* ignore */
      }
    },
    [access, refresh, tenantId, role] as const
  );
}

/** Decode the role from a JWT payload (no verification). */
export function decodeRole(jwt: string): OperatorRole | null {
  try {
    const parts = jwt.split(".");
    if (parts.length !== 3) return null;
    const json = Buffer.from(
      parts[1].replace(/-/g, "+").replace(/_/g, "/"),
      "base64"
    ).toString("utf8");
    const payload = JSON.parse(json) as { role?: OperatorRole };
    return payload.role ?? null;
  } catch {
    return null;
  }
}
