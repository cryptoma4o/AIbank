// fixtures/auth.ts — helpers to seed a fake JWT into localStorage.
//
// We never verify the signature (frontend only decodes payload via base64),
// so we just emit a JWT-shaped string with the claims expected by
// identity-service JWTClaims and lib/auth.ts decodeJwtPayload().

import type { Page } from "@playwright/test";

export interface FakeJwtClaims {
  sub: string;
  tenant_id: string;
  role: "bank.applicant" | "bank.operator" | "bank.admin" | string;
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

/** Build a JWT-shaped string (header.payload.signature) — signature is a stub. */
export function makeFakeJwt(claims: Partial<FakeJwtClaims> = {}): string {
  const nowSec = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = base64url(
    JSON.stringify({
      sub: claims.sub ?? "00000000-0000-0000-0000-000000000001",
      tenant_id: claims.tenant_id ?? "bank_alpha",
      role: claims.role ?? "bank.applicant",
      email: claims.email ?? "applicant@example.com",
      type: claims.type ?? "access",
      iat: claims.iat ?? nowSec,
      exp: claims.exp ?? nowSec + 3600,
    })
  );
  // Stub signature — payload is decoded without verification.
  const signature = base64url("stub-signature");
  return `${header}.${payload}.${signature}`;
}

export interface SeedAuthOptions {
  baseURL: string;
  tenantId?: string;
  role?: FakeJwtClaims["role"];
  email?: string;
}

/**
 * Pre-seeds localStorage with valid-shaped tokens BEFORE any page navigation.
 * Uses page.addInitScript so the values exist when AuthGuard mounts.
 */
export async function seedAuth(page: Page, opts: SeedAuthOptions): Promise<void> {
  const tenantId = opts.tenantId ?? "bank_alpha";
  const role = opts.role ?? "bank.applicant";
  const accessToken = makeFakeJwt({ tenant_id: tenantId, role, email: opts.email });
  const refreshToken = makeFakeJwt({
    tenant_id: tenantId,
    role,
    email: opts.email,
    type: "refresh",
    exp: Math.floor(Date.now() / 1000) + 7 * 24 * 3600,
  });

  await page.addInitScript(
    ([access, refresh, tenant]) => {
      try {
        window.localStorage.setItem("aibank.access_token", access);
        window.localStorage.setItem("aibank.refresh_token", refresh);
        window.localStorage.setItem("aibank.tenant_id", tenant);
      } catch {
        /* SSR / private mode — ignored */
      }
    },
    [accessToken, refreshToken, tenantId] as const
  );
}

/** Reads the stored access token from localStorage (post-navigation). */
export async function readStoredToken(page: Page): Promise<string | null> {
  return page.evaluate(() => window.localStorage.getItem("aibank.access_token"));
}
