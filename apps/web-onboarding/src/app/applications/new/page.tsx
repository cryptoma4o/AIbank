"use client";

// /applications/new — двухфазная форма создания заявки.
//
// Фаза 1 (этап 1 формы онбординга, см. docs/onboarding-form-spec.md §1):
//   ввод ИНН / ОГРН / короткого наименования + кнопка «Проверить ИНН».
//   Параллельно дёргается ext-egrul / ext-rosfinmon / ext-fssp через
//   bff-onboarding mutation prequalify(...). При decision === "proceed"
//   фаза 2 разблокируется. На "manual_review"/"reject" Submit заблокирован
//   и показано сообщение.
//
// Фаза 2:
//   legalEntityType (IP/LLC/JSC) + productCodes — как раньше. applicant_id
//   из JWT (см. resolveSubmitApplication в bff-onboarding/graph/resolver.go).

import { useRouter } from "next/navigation";
import { useState } from "react";
import { useMutation } from "@apollo/client";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import {
  MUTATION_PREQUALIFY,
  MUTATION_SUBMIT_APPLICATION,
} from "@/lib/graphql-operations";

const PRODUCTS: { code: string; label: string }[] = [
  { code: "current_account_rub", label: "Расчётный счёт (RUB)" },
  { code: "current_account_usd", label: "Расчётный счёт (USD)" },
  { code: "acquiring", label: "Эквайринг" },
  { code: "salary_project", label: "Зарплатный проект" },
];

const schema = z.object({
  legalEntityType: z.enum(["IP", "LLC", "JSC"], {
    errorMap: () => ({ message: "Выберите юридическую форму" }),
  }),
  productCodes: z.array(z.string()).min(1, "Выберите хотя бы один продукт"),
});

type FormValues = z.infer<typeof schema>;

interface SubmitMutationData {
  submitApplication: { id: string };
}

interface PrequalifyResult {
  decision: "proceed" | "manual_review" | "reject";
  decisionReason?: string | null;
  egrulStatus?: string | null;
  egrulFullName?: string | null;
  egrulCeoName?: string | null;
  egrulRegistrationDate?: string | null;
  egrulAddress?: string | null;
  nameMatchesEgrul: boolean;
  rosfinmonPresent: boolean;
  fsspProceedingsCount: number;
  unavailableSources: string[];
  checkedAt: string;
}

interface PrequalifyMutationData {
  prequalify: PrequalifyResult;
}

const INN_RE = /^\d{10}$/;
const OGRN_RE = /^\d{13}$/;

function NewApplicationContent() {
  const router = useRouter();
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Фаза 1 state.
  const [inn, setInn] = useState("");
  const [ogrn, setOgrn] = useState("");
  const [shortName, setShortName] = useState("");
  const [prequalifyError, setPrequalifyError] = useState<string | null>(null);
  const [prequalifyResult, setPrequalifyResult] = useState<PrequalifyResult | null>(null);

  const [prequalify, { loading: prequalifyLoading }] = useMutation<PrequalifyMutationData>(
    MUTATION_PREQUALIFY
  );
  const [submitApplication, { loading: submitLoading }] = useMutation<SubmitMutationData>(
    MUTATION_SUBMIT_APPLICATION
  );

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    defaultValues: { legalEntityType: "LLC", productCodes: ["current_account_rub"] },
  });

  async function onPrequalify() {
    setPrequalifyError(null);
    setPrequalifyResult(null);
    if (!INN_RE.test(inn)) {
      setPrequalifyError("ИНН должен состоять из 10 цифр (юрлицо)");
      return;
    }
    if (!OGRN_RE.test(ogrn)) {
      setPrequalifyError("ОГРН должен состоять из 13 цифр");
      return;
    }
    if (shortName.trim() === "") {
      setPrequalifyError("Укажите короткое наименование для сверки с ЕГРЮЛ");
      return;
    }
    try {
      const res = await prequalify({ variables: { input: { inn, ogrn, shortName } } });
      const r = res.data?.prequalify;
      if (!r) throw new Error("Не получилось получить ответ скоринга");
      setPrequalifyResult(r);
    } catch (err) {
      setPrequalifyError(err instanceof Error ? err.message : "Не удалось выполнить проверку");
    }
  }

  async function onSubmit(raw: FormValues) {
    setSubmitError(null);
    if (!prequalifyResult || prequalifyResult.decision !== "proceed") {
      setSubmitError("Сначала пройдите предварительную проверку (этап 1)");
      return;
    }
    const parsed = schema.safeParse(raw);
    if (!parsed.success) {
      setSubmitError(parsed.error.issues[0]?.message ?? "Проверьте поля формы");
      return;
    }
    try {
      const res = await submitApplication({
        variables: {
          input: {
            legalEntityType: parsed.data.legalEntityType,
            channel: "web",
            productCodes: parsed.data.productCodes,
          },
        },
      });

      const existingFromRes = extractExistingApplicationId(res?.errors);
      if (existingFromRes) {
        router.replace(`/applications/${existingFromRes}`);
        return;
      }

      const id = res.data?.submitApplication?.id;
      if (!id) {
        throw new Error("BFF вернул пустой идентификатор заявки");
      }
      router.replace(`/applications/${id}`);
    } catch (err) {
      const errObj = err as {
        graphQLErrors?: unknown;
        networkError?: { result?: { errors?: unknown } };
      };
      const existingId =
        extractExistingApplicationId(errObj?.graphQLErrors) ||
        extractExistingApplicationId(errObj?.networkError?.result?.errors);
      if (existingId) {
        router.replace(`/applications/${existingId}`);
        return;
      }
      setSubmitError(err instanceof Error ? err.message : "Не удалось создать заявку");
    }
  }

  // extractExistingApplicationId — те же три места, что и раньше.
  function extractExistingApplicationId(errors: unknown): string | null {
    if (!Array.isArray(errors)) return null;
    for (const gerr of errors) {
      const ext = (gerr as { extensions?: Record<string, unknown> })?.extensions;
      if (
        ext &&
        ext.code === "APPLICANT_HAS_APPLICATION" &&
        typeof ext.existingApplicationId === "string"
      ) {
        return ext.existingApplicationId;
      }
    }
    return null;
  }

  const submitAllowed = prequalifyResult?.decision === "proceed";

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <h1 className="text-2xl font-semibold text-gray-900">Новая заявка</h1>
      <p className="mt-2 text-sm text-gray-500">
        Сначала проверьте ИНН/ОГРН по ЕГРЮЛ, перечню Росфинмониторинга и ФССП. После
        одобрения предварительной проверки — выберите форму бизнеса и продукты.
      </p>

      {/* ── Фаза 1: предварительный скоринг ───────────────────────────── */}
      <section className="rounded-xl border border-gray-200 bg-white p-6 shadow-sm space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Этап 1 · Предварительная проверка</h2>
          <p className="text-xs text-gray-500">
            Пробивает юрлицо в ЕГРЮЛ, проверяет санкционные перечни и реестр исполнительных производств.
          </p>
        </header>
        {prequalifyError ? <ErrorBanner error={prequalifyError} /> : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">ИНН (10 цифр)</span>
            <input
              value={inn}
              onChange={(e) => setInn(e.target.value.replace(/\D/g, ""))}
              maxLength={10}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="7707083893"
            />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">ОГРН (13 цифр)</span>
            <input
              value={ogrn}
              onChange={(e) => setOgrn(e.target.value.replace(/\D/g, ""))}
              maxLength={13}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="1027700132195"
            />
          </label>
          <label className="text-sm sm:col-span-2">
            <span className="mb-1 block font-medium text-gray-700">Короткое наименование</span>
            <input
              value={shortName}
              onChange={(e) => setShortName(e.target.value)}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="ООО «Ромашка»"
            />
          </label>
        </div>
        <button
          type="button"
          onClick={onPrequalify}
          disabled={prequalifyLoading}
          className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          {prequalifyLoading ? <LoadingSpinner label="Проверяем…" /> : "Проверить ИНН"}
        </button>

        {prequalifyResult ? <PrequalifyResultCard result={prequalifyResult} /> : null}
      </section>

      {/* ── Фаза 2: создание заявки ──────────────────────────────────── */}
      <form
        onSubmit={handleSubmit(onSubmit)}
        noValidate
        className={`rounded-xl border bg-white p-6 shadow-sm space-y-6 ${submitAllowed ? "border-gray-200" : "border-gray-100 opacity-60"}`}
      >
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Этап 2 · Параметры заявки</h2>
          <p className="text-xs text-gray-500">
            {submitAllowed
              ? "Предварительная проверка пройдена — можно подавать заявку."
              : "Доступно после успешного скоринга на этапе 1."}
          </p>
        </header>
        {submitError ? <ErrorBanner error={submitError} /> : null}

        <fieldset disabled={!submitAllowed}>
          <legend className="mb-2 text-sm font-medium text-gray-700">Юридическая форма</legend>
          <div className="grid gap-2 sm:grid-cols-3">
            {(["IP", "LLC", "JSC"] as const).map((value) => (
              <label
                key={value}
                className="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-300 px-3 py-2 text-sm hover:border-primary"
              >
                <input type="radio" value={value} {...register("legalEntityType")} className="text-primary" />
                <span>
                  {value === "IP" && "ИП"}
                  {value === "LLC" && "ООО"}
                  {value === "JSC" && "АО"}
                </span>
              </label>
            ))}
          </div>
          {errors.legalEntityType ? (
            <p className="mt-1 text-xs text-danger">{errors.legalEntityType.message}</p>
          ) : null}
        </fieldset>

        <fieldset disabled={!submitAllowed}>
          <legend className="mb-2 text-sm font-medium text-gray-700">Банковские продукты</legend>
          <div className="space-y-2">
            {PRODUCTS.map((p) => (
              <label
                key={p.code}
                className="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm hover:border-primary"
              >
                <input type="checkbox" value={p.code} {...register("productCodes")} className="text-primary" />
                <span>{p.label}</span>
              </label>
            ))}
          </div>
          {errors.productCodes ? (
            <p className="mt-1 text-xs text-danger">{errors.productCodes.message}</p>
          ) : null}
        </fieldset>

        <p className="text-xs text-gray-500">
          Канал подачи: <strong>web</strong>. Заявитель определяется автоматически по сессии.
        </p>

        <div className="flex items-center justify-end gap-3">
          <button
            type="button"
            onClick={() => router.replace("/applications")}
            className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:border-gray-400"
          >
            Отмена
          </button>
          <button
            type="submit"
            disabled={submitLoading || !submitAllowed}
            className="flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitLoading ? <LoadingSpinner label="Создаём…" /> : "Создать заявку"}
          </button>
        </div>
      </form>
    </div>
  );
}

function PrequalifyResultCard({ result }: { result: PrequalifyResult }) {
  const tone =
    result.decision === "proceed"
      ? "border-green-200 bg-green-50 text-green-900"
      : result.decision === "manual_review"
        ? "border-amber-200 bg-amber-50 text-amber-900"
        : "border-red-200 bg-red-50 text-red-900";
  const label =
    result.decision === "proceed"
      ? "Можно продолжать"
      : result.decision === "manual_review"
        ? "Требуется ручная проверка"
        : "Отказ";
  return (
    <div className={`rounded-lg border p-4 text-sm space-y-2 ${tone}`}>
      <p className="font-semibold">{label}</p>
      {result.decisionReason ? <p>{result.decisionReason}</p> : null}
      <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-2 text-xs">
        {result.egrulFullName ? (
          <>
            <dt className="text-gray-600">Полное наименование</dt>
            <dd>{result.egrulFullName}</dd>
          </>
        ) : null}
        {result.egrulCeoName ? (
          <>
            <dt className="text-gray-600">ЕИО</dt>
            <dd>{result.egrulCeoName}</dd>
          </>
        ) : null}
        {result.egrulRegistrationDate ? (
          <>
            <dt className="text-gray-600">Дата регистрации</dt>
            <dd>{result.egrulRegistrationDate}</dd>
          </>
        ) : null}
        {result.egrulStatus ? (
          <>
            <dt className="text-gray-600">Статус ЕГРЮЛ</dt>
            <dd>{result.egrulStatus}</dd>
          </>
        ) : null}
        <dt className="text-gray-600">Имя совпадает с ЕГРЮЛ</dt>
        <dd>{result.nameMatchesEgrul ? "Да" : "Нет"}</dd>
        <dt className="text-gray-600">Перечень Росфинмона</dt>
        <dd>{result.rosfinmonPresent ? "В списке" : "Не найден"}</dd>
        <dt className="text-gray-600">Производства ФССП</dt>
        <dd>{result.fsspProceedingsCount}</dd>
      </dl>
      {result.unavailableSources.length > 0 ? (
        <p className="text-xs text-gray-700">
          Недоступные источники: {result.unavailableSources.join(", ")}
        </p>
      ) : null}
    </div>
  );
}

export default function NewApplicationPage() {
  return (
    <AuthGuard>
      <NewApplicationContent />
    </AuthGuard>
  );
}
