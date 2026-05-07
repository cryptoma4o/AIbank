"use client";

// /login — единственная публичная страница.
//
// Поля формы соответствуют loginRequest identity-service:
//   email, password, tenant_id.  Tenant_id в production будет
//   автоматически выводиться из subdomain (bank-alpha.aibank.ru →
//   tenant_id=bank_alpha) на уровне api-gateway, но на skeleton-этапе мы
//   принимаем его явно.

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { isAuthenticated, setTokens } from "@/lib/auth";
import { login } from "@/lib/identity";

const schema = z.object({
  email: z
    .string()
    .min(1, "Введите email")
    .email("Некорректный email"),
  password: z.string().min(1, "Введите пароль"),
  tenant_id: z
    .string()
    .min(1, "Введите идентификатор банка")
    .regex(/^[a-z][a-z0-9_]{1,31}$/, "Идентификатор банка содержит только строчные латиницу, цифры и _"),
});

type FormValues = z.infer<typeof schema>;

export default function LoginPage() {
  const router = useRouter();
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    defaultValues: { email: "", password: "", tenant_id: "" },
  });

  // Если пользователь уже авторизован — пропускаем сразу в /applications.
  useEffect(() => {
    if (isAuthenticated()) {
      router.replace("/applications");
    }
  }, [router]);

  async function onSubmit(raw: FormValues) {
    setSubmitError(null);
    const parsed = schema.safeParse(raw);
    if (!parsed.success) {
      setSubmitError(parsed.error.issues[0]?.message ?? "Проверьте поля формы");
      return;
    }
    setSubmitting(true);
    try {
      const tokens = await login(parsed.data);
      setTokens(tokens, parsed.data.tenant_id);
      router.replace("/applications");
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Не удалось войти");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-md">
      <h1 className="text-2xl font-semibold text-gray-900">Вход в кабинет</h1>
      <p className="mt-2 text-sm text-gray-500">
        Войдите, чтобы продолжить онбординг или открыть новую заявку.
      </p>

      <form
        className="mt-8 space-y-5 rounded-xl border border-gray-200 bg-white p-6 shadow-sm"
        onSubmit={handleSubmit(onSubmit)}
        noValidate
      >
        {submitError ? <ErrorBanner error={submitError} /> : null}

        <div>
          <label htmlFor="email" className="mb-1 block text-sm font-medium text-gray-700">
            Email
          </label>
          <input
            id="email"
            type="email"
            autoComplete="username"
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-primary"
            {...register("email")}
          />
          {errors.email ? (
            <p className="mt-1 text-xs text-danger">{errors.email.message}</p>
          ) : null}
        </div>

        <div>
          <label htmlFor="password" className="mb-1 block text-sm font-medium text-gray-700">
            Пароль
          </label>
          <input
            id="password"
            type="password"
            autoComplete="current-password"
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-primary"
            {...register("password")}
          />
          {errors.password ? (
            <p className="mt-1 text-xs text-danger">{errors.password.message}</p>
          ) : null}
        </div>

        <div>
          <label htmlFor="tenant_id" className="mb-1 block text-sm font-medium text-gray-700">
            Идентификатор банка
          </label>
          <input
            id="tenant_id"
            type="text"
            placeholder="bank_alpha"
            autoCapitalize="off"
            spellCheck={false}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-primary"
            {...register("tenant_id")}
          />
          <p className="mt-1 text-xs text-gray-500">
            В production определяется автоматически по поддомену.
          </p>
          {errors.tenant_id ? (
            <p className="mt-1 text-xs text-danger">{errors.tenant_id.message}</p>
          ) : null}
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="flex w-full items-center justify-center rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? <LoadingSpinner label="Входим…" /> : "Войти"}
        </button>

        <p className="text-center text-sm text-gray-600">
          Нет аккаунта?{" "}
          <Link href="/register" className="text-primary hover:underline">
            Зарегистрироваться
          </Link>
        </p>
      </form>
    </div>
  );
}
