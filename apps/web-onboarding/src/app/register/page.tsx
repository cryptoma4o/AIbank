"use client";

// /register — публичная регистрация applicant'а в кабинет онбординга.
//
// Шаг 1: POST {identity}/v1/applicants — создаёт запись в applicants
// (152-ФЗ ст. 9: согласие data_processing обязательно) И, если переданы
// email/password, создаёт пару в platform.users с role=applicant.
// Шаг 2: POST {identity}/v1/auth/login — обмен email/password на JWT.
// Шаг 3: setTokens + редирект на /applications.
//
// SMS/OTP flow — отдельная задача; здесь email+password как минимальный
// путь #2 (см. сессия 2026-05-07).

import { useRouter } from "next/navigation";
import Link from "next/link";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { setTokens } from "@/lib/auth";
import { identityUrl, login } from "@/lib/identity";

const schema = z
  .object({
    tenant_id: z
      .string()
      .min(1, "Введите идентификатор банка")
      .regex(/^[a-z][a-z0-9_]{1,31}$/, "Идентификатор: латиница, цифры, _; начинается с буквы"),
    full_name: z.string().min(2, "Укажите ФИО как в паспорте"),
    inn: z.string().regex(/^\d{12}$/, "ИНН физлица — ровно 12 цифр"),
    phone: z.string().regex(/^\+7\d{10}$/, "Телефон в формате +7XXXXXXXXXX"),
    email: z.string().email("Некорректный email"),
    password: z.string().min(8, "Минимум 8 символов"),
    password_confirm: z.string(),
    consent_data_processing: z
      .boolean()
      .refine((v) => v === true, "Согласие на обработку ПДн обязательно (152-ФЗ ст. 9)"),
  })
  .refine((d) => d.password === d.password_confirm, {
    message: "Пароли не совпадают",
    path: ["password_confirm"],
  });

type FormValues = z.infer<typeof schema>;

export default function RegisterPage() {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    defaultValues: {
      tenant_id: "demo",
      full_name: "",
      inn: "",
      phone: "+7",
      email: "",
      password: "",
      password_confirm: "",
      consent_data_processing: false,
    },
  });

  async function onSubmit(raw: FormValues) {
    setSubmitError(null);
    const parsed = schema.safeParse(raw);
    if (!parsed.success) {
      setSubmitError(parsed.error.issues[0]?.message ?? "Проверьте поля формы");
      return;
    }
    setSubmitting(true);
    try {
      // Шаг 1: регистрация applicant + users (если email/password передан).
      const regRes = await fetch(`${identityUrl()}/v1/applicants`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        cache: "no-store",
        body: JSON.stringify({
          tenant_id: parsed.data.tenant_id,
          inn: parsed.data.inn,
          phone: parsed.data.phone,
          full_name: parsed.data.full_name,
          email: parsed.data.email,
          password: parsed.data.password,
          consents: [
            { type: "data_processing", granted: true, version: "1.0" },
          ],
        }),
      });
      if (!regRes.ok) {
        const payload = (await regRes.json().catch(() => ({}))) as {
          error?: { message?: string };
        };
        throw new Error(payload.error?.message ?? `HTTP ${regRes.status}`);
      }

      // Шаг 2: логин по только что созданным credentials.
      const tokens = await login({
        email: parsed.data.email,
        password: parsed.data.password,
        tenant_id: parsed.data.tenant_id,
      });
      setTokens(tokens, parsed.data.tenant_id);
      router.replace("/applications");
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Не удалось зарегистрироваться");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-xl py-10">
      <h1 className="text-2xl font-semibold text-gray-900">Регистрация в кабинете</h1>
      <p className="mt-2 text-sm text-gray-500">
        Создайте учётную запись, чтобы подать заявку на открытие расчётного счёта.
      </p>
      <form
        onSubmit={handleSubmit(onSubmit)}
        noValidate
        className="mt-6 rounded-xl border border-gray-200 bg-white p-6 shadow-sm space-y-4"
      >
        {submitError ? <ErrorBanner error={submitError} /> : null}

        <Field label="Идентификатор банка" error={errors.tenant_id?.message}>
          <input
            {...register("tenant_id")}
            placeholder="demo"
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>
        <Field label="ФИО" error={errors.full_name?.message}>
          <input
            {...register("full_name")}
            placeholder="Иванов Иван Иванович"
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>
        <Field label="ИНН (12 цифр)" error={errors.inn?.message}>
          <input
            {...register("inn")}
            placeholder="123456789012"
            inputMode="numeric"
            maxLength={12}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>
        <Field label="Телефон" error={errors.phone?.message}>
          <input
            {...register("phone")}
            placeholder="+79001234567"
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>
        <Field label="Email" error={errors.email?.message}>
          <input
            type="email"
            {...register("email")}
            placeholder="ivanov@example.com"
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>
        <Field label="Пароль (мин. 8 символов)" error={errors.password?.message}>
          <input
            type="password"
            {...register("password")}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>
        <Field label="Подтвердите пароль" error={errors.password_confirm?.message}>
          <input
            type="password"
            {...register("password_confirm")}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </Field>

        <label className="flex items-start gap-2 text-sm text-gray-700">
          <input type="checkbox" {...register("consent_data_processing")} className="mt-0.5" />
          <span>
            Я даю согласие на обработку персональных данных в соответствии с 152-ФЗ.
            {errors.consent_data_processing ? (
              <span className="block text-xs text-danger">
                {errors.consent_data_processing.message}
              </span>
            ) : null}
          </span>
        </label>

        <div className="flex items-center justify-between gap-3 pt-2">
          <Link href="/login" className="text-sm text-primary hover:underline">
            Уже есть аккаунт? Войти
          </Link>
          <button
            type="submit"
            disabled={submitting}
            className="flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? <LoadingSpinner label="Регистрация…" /> : "Зарегистрироваться"}
          </button>
        </div>
      </form>
    </div>
  );
}

function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1 block text-sm font-medium text-gray-700">{label}</label>
      {children}
      {error ? <p className="mt-1 text-xs text-danger">{error}</p> : null}
    </div>
  );
}
