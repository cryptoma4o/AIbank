// identity.ts — REST-вызовы identity-service.
//
// GraphQL-схема BFF не содержит /auth/login — это сознательный выбор
// (mutation login в публичном GraphQL — антипаттерн), поэтому логин и
// /v1/me мы делаем напрямую к identity-service по REST.

import { getAccessToken, type TokenPair } from "./auth";

const DEFAULT_IDENTITY_URL = "http://localhost:8082";

export function identityUrl(): string {
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
      return "Учётная запись заблокирована. Обратитесь в банк.";
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
      /* пустое тело — оставляем fallback ниже */
    }
    const code = payload.error?.code;
    const message = payload.error?.message;
    throw new Error(loginErrorMessage(code, message ?? `HTTP ${res.status}`));
  }
  return (await res.json()) as TokenPair;
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
