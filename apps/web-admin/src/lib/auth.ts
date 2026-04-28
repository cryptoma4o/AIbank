// auth.ts — хранилище JWT в localStorage и базовые помощники.
//
// Скопировано (с минимальными правками) из web-onboarding, чтобы не
// плодить разные форматы хранения токенов между приложениями.  В
// production на bank.* поддоменах ключи будут одинаковыми, но т.к. оба
// SPA крутятся на разных origin (admin.bank-x.aibank.ru и
// onboarding.bank-x.aibank.ru) — конфликта по localStorage не будет.

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
  if (typeof payload.exp !== "number") return true;
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
  // identity-service кладёт roles массивом в часть будущих токенов; в
  // текущем формате (см. domain.JWTClaims) — единственное поле `role`.
  // Поддерживаем оба варианта, чтобы клиент не ломался при миграции.
  roles?: string[];
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

/** Извлекает плоский набор ролей пользователя из текущего access-токена. */
export function getRoles(): string[] {
  const token = getAccessToken();
  if (!token) return [];
  const payload = decodeJwtPayload(token);
  if (!payload) return [];
  if (Array.isArray(payload.roles)) return payload.roles;
  if (typeof payload.role === "string" && payload.role) return [payload.role];
  return [];
}

/** Проверка: одна из ролей пользователя входит в allowed-список. */
export function hasAnyRole(allowed: readonly string[]): boolean {
  const roles = getRoles();
  return roles.some((r) => allowed.includes(r));
}
