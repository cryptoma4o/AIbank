// identity.ts — REST-вызовы identity-service из admin-панели.
//
// Контракт совпадает с web-onboarding: POST /v1/auth/login и GET /v1/me,
// но банковский оператор в форме обязан указать tenant_id (банк, в котором
// он работает).  В production tenant_id будет автоматически определяться
// по поддомену (admin.bank-alpha.aibank.ru → bank_alpha) на api-gateway.

import { getAccessToken, type TokenPair } from "./auth";

const DEFAULT_IDENTITY_URL = "http://localhost:8082";

function identityUrl(): string {
  const fromEnv = process.env.NEXT_PUBLIC_IDENTITY_SERVICE_URL;
  return fromEnv && fromEnv.length > 0 ? fromEnv : DEFAULT_IDENTITY_URL;
}

export interface LoginRequest {
  email: string;
  password: string;
  tenant_id: string;
}

export interface LoginErrorPayload {
  error?: { code?: string; message?: string };
}

/** Маппит коды ошибок identity-service в человекочитаемые сообщения. */
export function loginErrorMessage(code: string | undefined, fallback: string): string {
  switch (code) {
    case "invalid_credentials":
      return "Неверный email, пароль или идентификатор банка";
    case "user_inactive":
      return "Учётная запись заблокирована. Обратитесь к администратору банка.";
    case "validation_failed":
      return "Проверьте правильность заполнения полей";
    case "invalid_body":
      return "Некорректный формат запроса";
    default:
      return fallback || "Не удалось войти. Попробуйте позже.";
  }
}

export async function login(req: LoginRequest): Promise<TokenPair> {
  const res = await fetch(`${identityUrl()}/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
    cache: "no-store",
  });
  if (!res.ok) {
    let payload: LoginErrorPayload = {};
    try {
      payload = (await res.json()) as LoginErrorPayload;
    } catch {
      /* пустое тело — оставляем fallback */
    }
    const code = payload.error?.code;
    const message = payload.error?.message;
    throw new Error(loginErrorMessage(code, message ?? `HTTP ${res.status}`));
  }
  return (await res.json()) as TokenPair;
}

export interface RefreshResponse {
  access_token: string;
  expires_in?: number;
}

export async function refresh(refreshToken: string): Promise<RefreshResponse> {
  const res = await fetch(`${identityUrl()}/v1/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`Не удалось обновить сессию (HTTP ${res.status})`);
  }
  return (await res.json()) as RefreshResponse;
}

export interface MePayload {
  id: string;
  tenant_id?: string | null;
  email: string;
  role: string;
  is_active: boolean;
}

export async function fetchMe(): Promise<MePayload> {
  const token = getAccessToken();
  if (!token) throw new Error("Не авторизован");
  const res = await fetch(`${identityUrl()}/v1/me`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`Не удалось загрузить профиль (HTTP ${res.status})`);
  }
  return (await res.json()) as MePayload;
}
