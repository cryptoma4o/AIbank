"use client";

// /applications/new — форма создания заявки.
//
// Поля соответствуют SubmitApplicationInput из schema.graphqls:
//   - legalEntityType (IP/LLC/JSC),
//   - channel (фиксировано "web"),
//   - productCodes (multi-checkbox).
//
// applicant_id явно не передаём: BFF берёт его из JWT (ac.UserID), как
// и должно быть для applicant-токенов.

import { useRouter } from "next/navigation";
import { useState } from "react";
import { useMutation } from "@apollo/client";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { MUTATION_SUBMIT_APPLICATION } from "@/lib/graphql-operations";

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

interface MutationData {
  submitApplication: { id: string };
}

function NewApplicationContent() {
  const router = useRouter();
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitApplication, { loading }] = useMutation<MutationData>(
    MUTATION_SUBMIT_APPLICATION
  );

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    defaultValues: { legalEntityType: "LLC", productCodes: ["current_account_rub"] },
  });

  async function onSubmit(raw: FormValues) {
    setSubmitError(null);
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
      const id = res.data?.submitApplication?.id;
      if (!id) {
        throw new Error("BFF вернул пустой идентификатор заявки");
      }
      router.replace(`/applications/${id}`);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Не удалось создать заявку");
    }
  }

  return (
    <div className="mx-auto max-w-2xl">
      <h1 className="text-2xl font-semibold text-gray-900">Новая заявка</h1>
      <p className="mt-2 text-sm text-gray-500">
        Укажите форму бизнеса и выберите банковские продукты — далее мы запросим документы.
      </p>

      <form
        className="mt-8 space-y-6 rounded-xl border border-gray-200 bg-white p-6 shadow-sm"
        onSubmit={handleSubmit(onSubmit)}
        noValidate
      >
        {submitError ? <ErrorBanner error={submitError} /> : null}

        <fieldset>
          <legend className="mb-2 text-sm font-medium text-gray-700">Юридическая форма</legend>
          <div className="grid gap-2 sm:grid-cols-3">
            {(["IP", "LLC", "JSC"] as const).map((value) => (
              <label
                key={value}
                className="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-300 px-3 py-2 text-sm hover:border-primary"
              >
                <input
                  type="radio"
                  value={value}
                  {...register("legalEntityType")}
                  className="text-primary"
                />
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

        <fieldset>
          <legend className="mb-2 text-sm font-medium text-gray-700">Банковские продукты</legend>
          <div className="space-y-2">
            {PRODUCTS.map((p) => (
              <label
                key={p.code}
                className="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm hover:border-primary"
              >
                <input
                  type="checkbox"
                  value={p.code}
                  {...register("productCodes")}
                  className="text-primary"
                />
                <span>{p.label}</span>
              </label>
            ))}
          </div>
          {errors.productCodes ? (
            <p className="mt-1 text-xs text-danger">{errors.productCodes.message}</p>
          ) : null}
        </fieldset>

        <div>
          <p className="text-xs text-gray-500">
            Канал подачи: <strong>web</strong>. Заявитель определяется автоматически по сессии.
          </p>
        </div>

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
            disabled={loading}
            className="flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
          >
            {loading ? <LoadingSpinner label="Создаём…" /> : "Создать заявку"}
          </button>
        </div>
      </form>
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
