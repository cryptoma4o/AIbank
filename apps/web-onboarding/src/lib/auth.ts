// auth.ts — хранилище JWT в localStorage и базовые помощники для access/refresh.
//
// Решение хранить токены в localStorage — осознанный компромисс ради
// простоты skeleton-приложения (Apollo Link читает их синхронно).  В
// production-варианте имеет смысл переходить на httpOnly-cookie и
// silent-refresh через api-gateway: см. docs/security-architecture.md.
//
// Все API безопасны для SSR — обращения к window/localStorage защищены
// проверкой `typeof window`.

const ACCESS_KEY = "aibank.access_token";
const REFRESH_KEY = "aibank.refresh_token";
const TENANT_KEY = "aibank.tenant_id";

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in?: number;
}

function isBrowser(): boolean {
  return typeof window !== "undefined";
}

export function setTokens(tokens: TokenPair, tenantId: string): void {
  if (!isBrowser()) return;
  window.localStorage.setItem(ACCESS_KEY, tokens.access_token);
  window.localStorage.setItem(REFRESH_KEY, tokens.refresh_token);
  if (tenantId) {
    window.localStorage.setItem(TENANT_KEY, tenantId);
  }
}

export function getAccessToken(): string | null {
  if (!isBrowser()) return null;
  return window.localStorage.getItem(ACCESS_KEY);
}

export function getRefreshToken(): string | null {
  if (!isBrowser()) return null;
  return window.localStorage.getItem(REFRESH_KEY);
}

export function getTenantId(): string | null {
  if (!isBrowser()) return null;
  return window.localStorage.getItem(TENANT_KEY);
}

/** Грубая проверка валидности access-токена: payload есть и не просрочен. */
export function isAuthenticated(): boolean {
  const token = getAccessToken();
  if (!token) return false;
  const payload = decodeJwtPayload(token);
  if (!payload) return false;
  if (typeof payload.exp !== "number") return true; // нет exp — доверяем серверу
  const nowSec = Math.floor(Date.now() / 1000);
  return payload.exp > nowSec;
}

export function logout(): void {
  if (!isBrowser()) return;
  window.localStorage.removeItem(ACCESS_KEY);
  window.localStorage.removeItem(REFRESH_KEY);
  window.localStorage.removeItem(TENANT_KEY);
}

export interface JwtPayload {
  sub?: string;
  tenant_id?: string;
  role?: string;
  exp?: number;
  iat?: number;
  type?: string;
}

/** Безопасный base64url-декод JWT-payload без проверки подписи. */
export function decodeJwtPayload(token: string): JwtPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64 + "===".slice(0, (4 - (base64.length % 4)) % 4);
    if (typeof atob !== "function") return null;
    const json = atob(padded);
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}
