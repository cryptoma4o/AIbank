"use client";

// /applications/[id]/wizard/3 — этап 3 формы онбординга.
//
// AML-сведения о деятельности (см. docs/onboarding-form-spec.md §3 и
// bff-onboarding/graph/schema.graphqls типы ApplicationActivity /
// SubmitApplicationActivityInput).
//
// Глубоко-вложенные поля (top-suppliers, top-buyers, operational-model,
// funds-source) BFF принимает как scalar JSON — собираем их на фронте и
// шлём в той же форме, что ждёт onboarding-orchestrator.

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery } from "@apollo/client";
import { useForm, useFieldArray } from "react-hook-form";
import { z } from "zod";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { WizardProgress } from "@/components/WizardProgress";
import {
  MUTATION_SUBMIT_APPLICATION_ACTIVITY,
  QUERY_APPLICATION_ACTIVITY,
} from "@/lib/graphql-operations";
import type { ApplicationActivity } from "@/types";

const ISO_3166_RE = /^[A-Z]{2}$/;
const ISO_4217_RE = /^[A-Z]{3}$/;

const counterpartySchema = z.object({
  name: z.string().min(1, "Укажите наименование контрагента"),
  inn: z.string().optional().or(z.literal("")),
  country: z
    .string()
    .regex(ISO_3166_RE, "Код страны в формате ISO 3166 alpha-2 (например, RU)"),
  sharePercent: z
    .number({ invalid_type_error: "Доля — число от 0 до 100" })
    .min(0)
    .max(100, "Доля не больше 100%"),
  relationshipType: z.enum(["permanent", "occasional"], {
    errorMap: () => ({ message: "Выберите тип отношений" }),
  }),
});

const schema = z
  .object({
    businessDescription: z
      .string()
      .min(20, "Опишите бизнес не короче 20 символов")
      .max(2000, "Описание не длиннее 2000 символов"),
    businessCategory: z.enum(["low", "medium", "high"], {
      errorMap: () => ({ message: "Выберите риск-категорию ОКВЭД" }),
    }),
    topSuppliers: z.array(counterpartySchema),
    topBuyers: z.array(counterpartySchema),
    monthlyTurnoverAmount: z
      .number({ invalid_type_error: "Месячный оборот — число" })
      .min(0, "Оборот не может быть отрицательным"),
    monthlyTurnoverCurrency: z
      .string()
      .regex(ISO_4217_RE, "Валюта в формате ISO 4217 (например, RUB)"),
    annualTurnoverAmount: z
      .number({ invalid_type_error: "Годовой оборот — число" })
      .min(0, "Оборот не может быть отрицательным"),
    annualTurnoverCurrency: z
      .string()
      .regex(ISO_4217_RE, "Валюта в формате ISO 4217 (например, RUB)"),
    cashSharePercent: z
      .number({ invalid_type_error: "Доля наличных — число от 0 до 100" })
      .min(0)
      .max(100, "Доля наличных не больше 100%"),
    foreignEconomicActivity: z.boolean(),
    foreignCountries: z.string().optional(),
    currencyOperations: z.string().optional(),
    geography: z.string().optional(),
    fundsSourceCategory: z.enum(
      ["revenue", "founder_contribution", "loan", "investments", "other"],
      { errorMap: () => ({ message: "Выберите источник средств" }) }
    ),
    fundsSourceDescription: z
      .string()
      .min(10, "Опишите источник средств не короче 10 символов")
      .max(2000, "Описание не длиннее 2000 символов"),
  })
  .superRefine((val, ctx) => {
    if (val.foreignEconomicActivity) {
      if (!val.foreignCountries || val.foreignCountries.trim() === "") {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["foreignCountries"],
          message: "Укажите страны ВЭД (через запятую), если ВЭД=да",
        });
      }
    }
  });

type FormValues = z.infer<typeof schema>;

function parseCsv(raw: string | undefined): string[] {
  if (!raw) return [];
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

const FUNDS_SOURCE_OPTIONS: { value: FormValues["fundsSourceCategory"]; label: string }[] = [
  { value: "revenue", label: "Выручка" },
  { value: "founder_contribution", label: "Учредительский взнос" },
  { value: "loan", label: "Займ" },
  { value: "investments", label: "Инвестиции" },
  { value: "other", label: "Другое" },
];

function ActivityForm({ applicationId }: { applicationId: string }) {
  const router = useRouter();
  const { data: existing, loading: loadingExisting } = useQuery<{
    applicationActivity: ApplicationActivity | null;
  }>(QUERY_APPLICATION_ACTIVITY, {
    variables: { applicationId },
    fetchPolicy: "cache-and-network",
  });

  const [submit, { loading: submitting, error: submitError }] = useMutation(
    MUTATION_SUBMIT_APPLICATION_ACTIVITY
  );

  const prior = existing?.applicationActivity ?? null;

  const {
    register,
    handleSubmit,
    control,
    watch,
    formState: { errors, isSubmitted },
  } = useForm<FormValues>({
    // Чтобы не дёргать reset при первом ответе сервера — явный mode и
    // defaultValues, собранные из последней сохранённой версии.
    mode: "onTouched",
    defaultValues: {
      businessDescription: prior?.businessDescription ?? "",
      businessCategory:
        (prior?.businessCategory as FormValues["businessCategory"]) ?? "low",
      topSuppliers:
        prior?.topSuppliers?.map((c) => ({
          name: c.name,
          inn: c.inn ?? "",
          country: c.country,
          sharePercent: c.sharePercent,
          relationshipType:
            (c.relationshipType as "permanent" | "occasional") ?? "permanent",
        })) ?? [],
      topBuyers:
        prior?.topBuyers?.map((c) => ({
          name: c.name,
          inn: c.inn ?? "",
          country: c.country,
          sharePercent: c.sharePercent,
          relationshipType:
            (c.relationshipType as "permanent" | "occasional") ?? "permanent",
        })) ?? [],
      monthlyTurnoverAmount:
        prior?.operationalModel?.monthlyTurnoverPlanned?.amount ?? 0,
      monthlyTurnoverCurrency:
        prior?.operationalModel?.monthlyTurnoverPlanned?.currency ?? "RUB",
      annualTurnoverAmount:
        prior?.operationalModel?.annualTurnoverPlanned?.amount ?? 0,
      annualTurnoverCurrency:
        prior?.operationalModel?.annualTurnoverPlanned?.currency ?? "RUB",
      cashSharePercent: prior?.operationalModel?.cashSharePercent ?? 0,
      foreignEconomicActivity:
        prior?.operationalModel?.foreignEconomicActivity ?? false,
      foreignCountries:
        prior?.operationalModel?.foreignCountries?.join(", ") ?? "",
      currencyOperations:
        prior?.operationalModel?.currencyOperations?.join(", ") ?? "",
      geography: prior?.operationalModel?.geography?.join(", ") ?? "",
      fundsSourceCategory:
        (prior?.fundsSource?.category as FormValues["fundsSourceCategory"]) ??
        "revenue",
      fundsSourceDescription: prior?.fundsSource?.description ?? "",
    },
  });

  const suppliers = useFieldArray({ control, name: "topSuppliers" });
  const buyers = useFieldArray({ control, name: "topBuyers" });
  const vedEnabled = watch("foreignEconomicActivity");

  async function onSubmit(values: FormValues) {
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      // Заполняется через formState.errors — здесь просто прерываем.
      return;
    }
    const v = parsed.data;
    await submit({
      variables: {
        input: {
          applicationId,
          businessDescription: v.businessDescription,
          businessCategory: v.businessCategory,
          topSuppliers: v.topSuppliers.map((c) => ({
            name: c.name,
            inn: c.inn || null,
            country: c.country,
            share_percent: c.sharePercent,
            relationship_type: c.relationshipType,
          })),
          topBuyers: v.topBuyers.map((c) => ({
            name: c.name,
            inn: c.inn || null,
            country: c.country,
            share_percent: c.sharePercent,
            relationship_type: c.relationshipType,
          })),
          operationalModel: {
            geography: parseCsv(v.geography),
            monthly_turnover_planned: {
              amount: v.monthlyTurnoverAmount,
              currency: v.monthlyTurnoverCurrency,
            },
            annual_turnover_planned: {
              amount: v.annualTurnoverAmount,
              currency: v.annualTurnoverCurrency,
            },
            cash_share_percent: v.cashSharePercent,
            foreign_economic_activity: v.foreignEconomicActivity,
            foreign_countries: v.foreignEconomicActivity
              ? parseCsv(v.foreignCountries)
              : [],
            currency_operations: parseCsv(v.currencyOperations),
          },
          fundsSource: {
            category: v.fundsSourceCategory,
            description: v.fundsSourceDescription,
          },
        },
      },
    });
    router.push(`/applications/${applicationId}/wizard/4`);
  }

  if (loadingExisting && !existing) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner label="Загружаем сохранённые данные…" />
      </div>
    );
  }

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      noValidate
      className="space-y-6"
      data-testid="stage-3-form"
    >
      {submitError ? <ErrorBanner error={submitError} /> : null}

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Описание бизнеса</h2>
          <p className="text-xs text-gray-500">
            Чем именно занимается юрлицо и риск-категория ОКВЭД.
          </p>
        </header>
        <label className="block text-sm">
          <span className="mb-1 block font-medium text-gray-700">
            Описание деятельности
          </span>
          <textarea
            rows={4}
            {...register("businessDescription")}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            placeholder="Например: оптовая торговля канцелярскими товарами B2B…"
          />
          {errors.businessDescription ? (
            <p className="mt-1 text-xs text-danger">
              {errors.businessDescription.message}
            </p>
          ) : null}
        </label>

        <fieldset>
          <legend className="mb-2 text-sm font-medium text-gray-700">
            Риск-категория ОКВЭД
          </legend>
          <div className="grid gap-2 sm:grid-cols-3">
            {(["low", "medium", "high"] as const).map((cat) => (
              <label
                key={cat}
                className="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-300 px-3 py-2 text-sm hover:border-primary"
              >
                <input
                  type="radio"
                  value={cat}
                  {...register("businessCategory")}
                  className="text-primary"
                />
                <span>
                  {cat === "low" && "Низкий риск"}
                  {cat === "medium" && "Средний риск"}
                  {cat === "high" && "Высокий риск"}
                </span>
              </label>
            ))}
          </div>
          {errors.businessCategory ? (
            <p className="mt-1 text-xs text-danger">
              {errors.businessCategory.message}
            </p>
          ) : null}
        </fieldset>
      </section>

      <CounterpartyEditor
        title="Топ-5 поставщиков"
        registerPrefix="topSuppliers"
        fields={suppliers.fields}
        onAppend={() =>
          suppliers.append({
            name: "",
            inn: "",
            country: "RU",
            sharePercent: 0,
            relationshipType: "permanent",
          })
        }
        onRemove={(i) => suppliers.remove(i)}
        register={register}
        errors={errors.topSuppliers}
      />

      <CounterpartyEditor
        title="Топ-5 покупателей"
        registerPrefix="topBuyers"
        fields={buyers.fields}
        onAppend={() =>
          buyers.append({
            name: "",
            inn: "",
            country: "RU",
            sharePercent: 0,
            relationshipType: "permanent",
          })
        }
        onRemove={(i) => buyers.remove(i)}
        register={register}
        errors={errors.topBuyers}
      />

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Операционная модель</h2>
          <p className="text-xs text-gray-500">
            Планируемые обороты, география, ВЭД и валютные операции.
          </p>
        </header>

        <div className="grid gap-3 sm:grid-cols-2">
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Месячный оборот (план)
            </span>
            <input
              type="number"
              step="0.01"
              {...register("monthlyTurnoverAmount", { valueAsNumber: true })}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.monthlyTurnoverAmount ? (
              <p className="mt-1 text-xs text-danger">
                {errors.monthlyTurnoverAmount.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Валюта мес. оборота
            </span>
            <input
              {...register("monthlyTurnoverCurrency")}
              maxLength={3}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
              placeholder="RUB"
            />
            {errors.monthlyTurnoverCurrency ? (
              <p className="mt-1 text-xs text-danger">
                {errors.monthlyTurnoverCurrency.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Годовой оборот (план)
            </span>
            <input
              type="number"
              step="0.01"
              {...register("annualTurnoverAmount", { valueAsNumber: true })}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.annualTurnoverAmount ? (
              <p className="mt-1 text-xs text-danger">
                {errors.annualTurnoverAmount.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Валюта год. оборота
            </span>
            <input
              {...register("annualTurnoverCurrency")}
              maxLength={3}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
              placeholder="RUB"
            />
            {errors.annualTurnoverCurrency ? (
              <p className="mt-1 text-xs text-danger">
                {errors.annualTurnoverCurrency.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Доля наличных, %
            </span>
            <input
              type="number"
              step="0.1"
              {...register("cashSharePercent", { valueAsNumber: true })}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.cashSharePercent ? (
              <p className="mt-1 text-xs text-danger">
                {errors.cashSharePercent.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              География операций (ISO 3166, через запятую)
            </span>
            <input
              {...register("geography")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
              placeholder="RU, KZ, BY"
            />
          </label>
        </div>

        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            {...register("foreignEconomicActivity")}
            className="text-primary"
          />
          <span className="font-medium text-gray-700">
            Планируется внешнеэкономическая деятельность (ВЭД)
          </span>
        </label>

        {vedEnabled ? (
          <label className="block text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Страны ВЭД (ISO 3166, через запятую)
            </span>
            <input
              {...register("foreignCountries")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
              placeholder="CN, TR"
            />
            {errors.foreignCountries ? (
              <p className="mt-1 text-xs text-danger">
                {errors.foreignCountries.message}
              </p>
            ) : null}
          </label>
        ) : null}

        <label className="block text-sm">
          <span className="mb-1 block font-medium text-gray-700">
            Валюты операций (ISO 4217, через запятую)
          </span>
          <input
            {...register("currencyOperations")}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
            placeholder="RUB, USD, EUR"
          />
        </label>
      </section>

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Источник средств</h2>
          <p className="text-xs text-gray-500">
            Происхождение средств для пополнения счёта.
          </p>
        </header>
        <fieldset>
          <legend className="mb-2 text-sm font-medium text-gray-700">
            Категория источника
          </legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {FUNDS_SOURCE_OPTIONS.map((opt) => (
              <label
                key={opt.value}
                className="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-300 px-3 py-2 text-sm hover:border-primary"
              >
                <input
                  type="radio"
                  value={opt.value}
                  {...register("fundsSourceCategory")}
                  className="text-primary"
                />
                <span>{opt.label}</span>
              </label>
            ))}
          </div>
          {errors.fundsSourceCategory ? (
            <p className="mt-1 text-xs text-danger">
              {errors.fundsSourceCategory.message}
            </p>
          ) : null}
        </fieldset>
        <label className="block text-sm">
          <span className="mb-1 block font-medium text-gray-700">Обоснование</span>
          <textarea
            rows={3}
            {...register("fundsSourceDescription")}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            placeholder="Краткое пояснение, откуда поступают средства"
          />
          {errors.fundsSourceDescription ? (
            <p className="mt-1 text-xs text-danger">
              {errors.fundsSourceDescription.message}
            </p>
          ) : null}
        </label>
      </section>

      {isSubmitted && Object.keys(errors).length > 0 ? (
        <ErrorBanner error="Проверьте подсвеченные поля выше" />
      ) : null}

      <div className="flex items-center justify-between gap-3">
        <Link
          href={`/applications/${applicationId}`}
          className="text-sm text-gray-600 hover:text-primary"
        >
          ← К заявке
        </Link>
        <button
          type="submit"
          disabled={submitting}
          className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? (
            <LoadingSpinner label="Сохраняем…" />
          ) : (
            "Сохранить и перейти к этапу 4"
          )}
        </button>
      </div>
    </form>
  );
}

interface CounterpartyEditorProps {
  title: string;
  registerPrefix: "topSuppliers" | "topBuyers";
  fields: { id: string }[];
  onAppend: () => void;
  onRemove: (idx: number) => void;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  register: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  errors: any;
}

function CounterpartyEditor({
  title,
  registerPrefix,
  fields,
  onAppend,
  onRemove,
  register,
  errors,
}: CounterpartyEditorProps) {
  return (
    <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">{title}</h2>
          <p className="text-xs text-gray-500">
            До 5 контрагентов с долей оборота и типом отношений.
          </p>
        </div>
        <button
          type="button"
          onClick={onAppend}
          disabled={fields.length >= 5}
          className="rounded-lg border border-primary px-3 py-1 text-xs font-medium text-primary hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-60"
          data-testid={`add-${registerPrefix}`}
        >
          + Добавить
        </button>
      </header>
      {fields.length === 0 ? (
        <p className="text-xs text-gray-500">Контрагенты не добавлены.</p>
      ) : null}
      <ul className="space-y-3">
        {fields.map((f, i) => (
          <li key={f.id} className="rounded-lg border border-gray-200 p-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="text-sm">
                <span className="mb-1 block font-medium text-gray-700">
                  Наименование
                </span>
                <input
                  {...register(`${registerPrefix}.${i}.name` as const)}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
                />
                {errors?.[i]?.name ? (
                  <p className="mt-1 text-xs text-danger">
                    {errors[i].name.message}
                  </p>
                ) : null}
              </label>
              <label className="text-sm">
                <span className="mb-1 block font-medium text-gray-700">ИНН</span>
                <input
                  {...register(`${registerPrefix}.${i}.inn` as const)}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
                  placeholder="опционально"
                />
              </label>
              <label className="text-sm">
                <span className="mb-1 block font-medium text-gray-700">
                  Страна (ISO 3166)
                </span>
                <input
                  {...register(`${registerPrefix}.${i}.country` as const)}
                  maxLength={2}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
                />
                {errors?.[i]?.country ? (
                  <p className="mt-1 text-xs text-danger">
                    {errors[i].country.message}
                  </p>
                ) : null}
              </label>
              <label className="text-sm">
                <span className="mb-1 block font-medium text-gray-700">
                  Доля оборота, %
                </span>
                <input
                  type="number"
                  step="0.1"
                  {...register(`${registerPrefix}.${i}.sharePercent` as const, {
                    valueAsNumber: true,
                  })}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
                />
                {errors?.[i]?.sharePercent ? (
                  <p className="mt-1 text-xs text-danger">
                    {errors[i].sharePercent.message}
                  </p>
                ) : null}
              </label>
              <label className="text-sm sm:col-span-2">
                <span className="mb-1 block font-medium text-gray-700">
                  Тип отношений
                </span>
                <select
                  {...register(`${registerPrefix}.${i}.relationshipType` as const)}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
                >
                  <option value="permanent">Постоянный</option>
                  <option value="occasional">Разовый</option>
                </select>
              </label>
            </div>
            <div className="mt-2 text-right">
              <button
                type="button"
                onClick={() => onRemove(i)}
                className="text-xs text-danger hover:underline"
              >
                Удалить
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Stage3Content() {
  const params = useParams<{ id: string }>();
  const applicationId = params?.id ?? "";

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <header className="space-y-3">
        <Link
          href="/applications"
          className="text-xs text-gray-500 hover:text-primary"
        >
          ← К списку заявок
        </Link>
        <WizardProgress currentStep={3} applicationId={applicationId} />
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">
            Этап 3 · Сведения о деятельности
          </h1>
          <p className="mt-1 text-sm text-gray-500">
            Заполните AML-сведения: контрагенты, обороты, ВЭД и источник средств.
          </p>
        </div>
      </header>
      {applicationId ? (
        <ActivityForm applicationId={applicationId} />
      ) : (
        <ErrorBanner error="В URL не передан идентификатор заявки." />
      )}
    </div>
  );
}

export default function Stage3Page() {
  return (
    <AuthGuard>
      <Stage3Content />
    </AuthGuard>
  );
}
