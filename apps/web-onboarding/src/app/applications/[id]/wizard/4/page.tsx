"use client";

// /applications/[id]/wizard/4 — этап 4 формы онбординга.
//
// ЕИО и представители юрлица (см. docs/onboarding-form-spec.md §4 и
// bff-onboarding/graph/schema.graphqls типы Representative /
// UpsertRepresentativeInput).
//
// На этой итерации форма добавляет/обновляет одного представителя за
// сабмит (это и нужно: на верхнем уровне страница показывает уже
// сохранённых представителей и форму на следующего).
//
// legalEntityId на UI приходит из uri-сегмента (упрощение: используем
// applicationId как legalEntityId до тех пор, пока bff не вернёт
// дискретный legalEntityId на Application).  См. блокер в отчёте.

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery } from "@apollo/client";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { WizardProgress } from "@/components/WizardProgress";
import {
  MUTATION_UPSERT_REPRESENTATIVE,
  QUERY_REPRESENTATIVES,
} from "@/lib/graphql-operations";
import type { Representative } from "@/types";

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const INN_PERSON_RE = /^\d{12}$/;
const SNILS_RE = /^\d{3}-\d{3}-\d{3} \d{2}$/;
const PASSPORT_SERIES_RE = /^\d{4}$/;
const PASSPORT_NUMBER_RE = /^\d{6}$/;
const DEPARTMENT_CODE_RE = /^\d{3}-\d{3}$/;

const schema = z
  .object({
    lastName: z.string().min(1, "Укажите фамилию"),
    firstName: z.string().min(1, "Укажите имя"),
    middleName: z.string().optional(),
    birthDate: z
      .string()
      .regex(DATE_RE, "Дата рождения в формате YYYY-MM-DD"),
    birthPlace: z.string().optional(),
    citizenship: z.string().min(2, "Укажите гражданство (ISO 3166, RU/KZ/…)"),
    inn: z
      .string()
      .optional()
      .refine((v) => !v || INN_PERSON_RE.test(v), {
        message: "ИНН физлица — 12 цифр",
      }),
    snils: z
      .string()
      .optional()
      .refine((v) => !v || SNILS_RE.test(v), {
        message: "СНИЛС в формате XXX-XXX-XXX XX",
      }),
    // Паспорт.
    docType: z.enum(
      ["ru_passport", "foreign_passport", "national_passport", "refugee_id"],
      { errorMap: () => ({ message: "Выберите тип документа" }) }
    ),
    docSeries: z.string().optional(),
    docNumber: z.string().min(1, "Укажите номер документа"),
    docIssueDate: z
      .string()
      .regex(DATE_RE, "Дата выдачи в формате YYYY-MM-DD"),
    docExpiryDate: z
      .string()
      .optional()
      .refine((v) => !v || DATE_RE.test(v), {
        message: "Дата окончания в формате YYYY-MM-DD",
      }),
    docIssuedBy: z.string().min(3, "Укажите, кем выдан документ"),
    docDepartmentCode: z.string().optional(),
    // Полномочия.
    position: z.string().min(1, "Укажите должность"),
    authorityBasis: z.enum(["charter", "protocol", "power_of_attorney", "order"], {
      errorMap: () => ({ message: "Выберите основание полномочий" }),
    }),
    authorityDocNumber: z.string().optional(),
    authorityDocDate: z
      .string()
      .optional()
      .refine((v) => !v || DATE_RE.test(v), {
        message: "Дата документа в формате YYYY-MM-DD",
      }),
    // ПДЛ.
    isPdl: z.boolean(),
    pdlCategory: z.enum(["foreign", "russian", "international"]).optional(),
    pdlPosition: z.string().optional(),
    pdlRelation: z.enum(["self", "relative", "representative"]).optional(),
    isPrimary: z.boolean(),
    isSignatory: z.boolean(),
  })
  .superRefine((val, ctx) => {
    if (val.docType === "ru_passport") {
      if (!val.docSeries || !PASSPORT_SERIES_RE.test(val.docSeries)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["docSeries"],
          message: "Серия паспорта РФ — 4 цифры",
        });
      }
      if (!PASSPORT_NUMBER_RE.test(val.docNumber)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["docNumber"],
          message: "Номер паспорта РФ — 6 цифр",
        });
      }
      if (
        val.docDepartmentCode &&
        !DEPARTMENT_CODE_RE.test(val.docDepartmentCode)
      ) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["docDepartmentCode"],
          message: "Код подразделения в формате XXX-XXX",
        });
      }
    }
    if (val.isPdl) {
      if (!val.pdlCategory) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["pdlCategory"],
          message: "Выберите категорию ПДЛ",
        });
      }
      if (!val.pdlRelation) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["pdlRelation"],
          message: "Укажите тип связи с ПДЛ",
        });
      }
    }
  });

type FormValues = z.infer<typeof schema>;

function RepresentativeForm({
  applicationId,
  legalEntityId,
  onSaved,
}: {
  applicationId: string;
  legalEntityId: string;
  onSaved: () => void;
}) {
  const router = useRouter();
  const [upsert, { loading: submitting, error: submitError }] = useMutation(
    MUTATION_UPSERT_REPRESENTATIVE
  );

  const {
    register,
    handleSubmit,
    watch,
    reset,
    formState: { errors, isSubmitted },
  } = useForm<FormValues>({
    mode: "onTouched",
    defaultValues: {
      lastName: "",
      firstName: "",
      middleName: "",
      birthDate: "",
      birthPlace: "",
      citizenship: "RU",
      inn: "",
      snils: "",
      docType: "ru_passport",
      docSeries: "",
      docNumber: "",
      docIssueDate: "",
      docExpiryDate: "",
      docIssuedBy: "",
      docDepartmentCode: "",
      position: "Генеральный директор",
      authorityBasis: "charter",
      authorityDocNumber: "",
      authorityDocDate: "",
      isPdl: false,
      isPrimary: true,
      isSignatory: true,
    },
  });

  const pdl = watch("isPdl");
  const docType = watch("docType");

  async function onSubmit(values: FormValues) {
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      return;
    }
    const v = parsed.data;
    await upsert({
      variables: {
        input: {
          applicationId,
          legalEntityId,
          lastName: v.lastName,
          firstName: v.firstName,
          middleName: v.middleName || null,
          birthDate: v.birthDate,
          birthPlace: v.birthPlace || null,
          citizenship: [v.citizenship.toUpperCase()],
          inn: v.inn || null,
          snils: v.snils || null,
          idDocument: {
            doc_type: v.docType,
            series: v.docSeries || null,
            number: v.docNumber,
            issue_date: v.docIssueDate,
            expiry_date: v.docExpiryDate || null,
            issued_by: v.docIssuedBy,
            department_code: v.docDepartmentCode || null,
          },
          authority: {
            position: v.position,
            authority_basis: v.authorityBasis,
            authority_doc_number: v.authorityDocNumber || null,
            authority_doc_date: v.authorityDocDate || null,
          },
          pdlDeclaration: {
            is_pdl: v.isPdl,
            category: v.isPdl ? v.pdlCategory : null,
            position: v.isPdl ? v.pdlPosition || null : null,
            relation: v.isPdl ? v.pdlRelation : null,
          },
          isPrimary: v.isPrimary,
          isSignatory: v.isSignatory,
        },
      },
    });
    reset();
    onSaved();
  }

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      noValidate
      className="space-y-6"
      data-testid="stage-4-form"
    >
      {submitError ? <ErrorBanner error={submitError} /> : null}

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Персональные данные</h2>
        </header>
        <div className="grid gap-3 sm:grid-cols-3">
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Фамилия</span>
            <input
              {...register("lastName")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.lastName ? (
              <p className="mt-1 text-xs text-danger">{errors.lastName.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Имя</span>
            <input
              {...register("firstName")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.firstName ? (
              <p className="mt-1 text-xs text-danger">{errors.firstName.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Отчество</span>
            <input
              {...register("middleName")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="опционально"
            />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Дата рождения</span>
            <input
              type="date"
              {...register("birthDate")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.birthDate ? (
              <p className="mt-1 text-xs text-danger">{errors.birthDate.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Место рождения</span>
            <input
              {...register("birthPlace")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Гражданство (ISO 3166)</span>
            <input
              {...register("citizenship")}
              maxLength={2}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm uppercase focus:border-primary focus:outline-none"
            />
            {errors.citizenship ? (
              <p className="mt-1 text-xs text-danger">{errors.citizenship.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">ИНН (12 цифр)</span>
            <input
              {...register("inn")}
              maxLength={12}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="опционально"
            />
            {errors.inn ? (
              <p className="mt-1 text-xs text-danger">{errors.inn.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">СНИЛС</span>
            <input
              {...register("snils")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="XXX-XXX-XXX XX"
            />
            {errors.snils ? (
              <p className="mt-1 text-xs text-danger">{errors.snils.message}</p>
            ) : null}
          </label>
        </div>
      </section>

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">
            Документ, удостоверяющий личность
          </h2>
        </header>
        <fieldset>
          <legend className="mb-2 text-sm font-medium text-gray-700">Тип документа</legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {(
              [
                ["ru_passport", "Паспорт РФ"],
                ["foreign_passport", "Загранпаспорт"],
                ["national_passport", "Нац. паспорт иностранца"],
                ["refugee_id", "Удостоверение беженца"],
              ] as const
            ).map(([value, label]) => (
              <label
                key={value}
                className="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-300 px-3 py-2 text-sm hover:border-primary"
              >
                <input
                  type="radio"
                  value={value}
                  {...register("docType")}
                  className="text-primary"
                />
                <span>{label}</span>
              </label>
            ))}
          </div>
          {errors.docType ? (
            <p className="mt-1 text-xs text-danger">{errors.docType.message}</p>
          ) : null}
        </fieldset>
        <div className="grid gap-3 sm:grid-cols-3">
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Серия</span>
            <input
              {...register("docSeries")}
              maxLength={4}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder={docType === "ru_passport" ? "4502" : "опционально"}
            />
            {errors.docSeries ? (
              <p className="mt-1 text-xs text-danger">{errors.docSeries.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Номер</span>
            <input
              {...register("docNumber")}
              maxLength={20}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder={docType === "ru_passport" ? "123456" : ""}
            />
            {errors.docNumber ? (
              <p className="mt-1 text-xs text-danger">{errors.docNumber.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Код подразделения</span>
            <input
              {...register("docDepartmentCode")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="770-001"
            />
            {errors.docDepartmentCode ? (
              <p className="mt-1 text-xs text-danger">
                {errors.docDepartmentCode.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Дата выдачи</span>
            <input
              type="date"
              {...register("docIssueDate")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.docIssueDate ? (
              <p className="mt-1 text-xs text-danger">{errors.docIssueDate.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Дата окончания</span>
            <input
              type="date"
              {...register("docExpiryDate")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.docExpiryDate ? (
              <p className="mt-1 text-xs text-danger">{errors.docExpiryDate.message}</p>
            ) : null}
          </label>
          <label className="text-sm sm:col-span-3">
            <span className="mb-1 block font-medium text-gray-700">Кем выдан</span>
            <input
              {...register("docIssuedBy")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.docIssuedBy ? (
              <p className="mt-1 text-xs text-danger">{errors.docIssuedBy.message}</p>
            ) : null}
          </label>
        </div>
      </section>

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">Полномочия</h2>
          <p className="text-xs text-gray-500">Должность и основание полномочий.</p>
        </header>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">Должность</span>
            <input
              {...register("position")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.position ? (
              <p className="mt-1 text-xs text-danger">{errors.position.message}</p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Основание полномочий
            </span>
            <select
              {...register("authorityBasis")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            >
              <option value="charter">Устав</option>
              <option value="protocol">Протокол / решение</option>
              <option value="power_of_attorney">Доверенность</option>
              <option value="order">Приказ</option>
            </select>
            {errors.authorityBasis ? (
              <p className="mt-1 text-xs text-danger">
                {errors.authorityBasis.message}
              </p>
            ) : null}
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Номер документа полномочий
            </span>
            <input
              {...register("authorityDocNumber")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder="опционально"
            />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-gray-700">
              Дата документа полномочий
            </span>
            <input
              type="date"
              {...register("authorityDocDate")}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
            />
            {errors.authorityDocDate ? (
              <p className="mt-1 text-xs text-danger">
                {errors.authorityDocDate.message}
              </p>
            ) : null}
          </label>
        </div>
        <div className="flex gap-6">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              {...register("isPrimary")}
              className="text-primary"
            />
            <span>ЕИО / основной представитель</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              {...register("isSignatory")}
              className="text-primary"
            />
            <span>Право первой подписи</span>
          </label>
        </div>
      </section>

      <section className="rounded-xl border border-gray-200 bg-white p-6 space-y-4">
        <header>
          <h2 className="text-lg font-semibold text-gray-900">
            Декларация ПДЛ
          </h2>
          <p className="text-xs text-gray-500">
            Признак публичного должностного лица и связь с ним.
          </p>
        </header>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            {...register("isPdl")}
            className="text-primary"
          />
          <span className="font-medium text-gray-700">
            Является ПДЛ или связан с ПДЛ
          </span>
        </label>
        {pdl ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="text-sm">
              <span className="mb-1 block font-medium text-gray-700">Категория</span>
              <select
                {...register("pdlCategory")}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              >
                <option value="">— выберите —</option>
                <option value="foreign">Иностранное ПДЛ</option>
                <option value="russian">Российское ПДЛ</option>
                <option value="international">ПДЛ международной организации</option>
              </select>
              {errors.pdlCategory ? (
                <p className="mt-1 text-xs text-danger">
                  {errors.pdlCategory.message}
                </p>
              ) : null}
            </label>
            <label className="text-sm">
              <span className="mb-1 block font-medium text-gray-700">Тип связи</span>
              <select
                {...register("pdlRelation")}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              >
                <option value="">— выберите —</option>
                <option value="self">Сам</option>
                <option value="relative">Родственник</option>
                <option value="representative">Представитель</option>
              </select>
              {errors.pdlRelation ? (
                <p className="mt-1 text-xs text-danger">
                  {errors.pdlRelation.message}
                </p>
              ) : null}
            </label>
            <label className="text-sm sm:col-span-2">
              <span className="mb-1 block font-medium text-gray-700">
                Занимаемая должность ПДЛ
              </span>
              <input
                {...register("pdlPosition")}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary focus:outline-none"
              />
            </label>
          </div>
        ) : null}
      </section>

      {isSubmitted && Object.keys(errors).length > 0 ? (
        <ErrorBanner error="Проверьте подсвеченные поля выше" />
      ) : null}

      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          onClick={() => router.push(`/applications/${applicationId}/wizard/3`)}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:border-gray-400"
        >
          ← Назад к этапу 3
        </button>
        <button
          type="submit"
          disabled={submitting}
          className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? (
            <LoadingSpinner label="Сохраняем…" />
          ) : (
            "Сохранить представителя"
          )}
        </button>
      </div>
    </form>
  );
}

function RepresentativesList({
  representatives,
}: {
  representatives: Representative[];
}) {
  if (representatives.length === 0) {
    return (
      <p className="text-sm text-gray-500">
        Представители ещё не добавлены — заполните форму ниже.
      </p>
    );
  }
  return (
    <ul className="space-y-3" data-testid="representatives-list">
      {representatives.map((r) => (
        <li
          key={r.id}
          className="rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm"
        >
          <div className="flex items-start justify-between">
            <div>
              <p className="font-medium text-gray-900">
                {r.lastName} {r.firstName} {r.middleName ?? ""}
              </p>
              <p className="text-xs text-gray-500">
                {r.authority.position} ·{" "}
                {r.authority.authorityBasis === "charter"
                  ? "по уставу"
                  : r.authority.authorityBasis === "protocol"
                    ? "по протоколу"
                    : r.authority.authorityBasis === "power_of_attorney"
                      ? "по доверенности"
                      : "по приказу"}
              </p>
            </div>
            <div className="flex gap-2 text-xs">
              {r.isPrimary ? (
                <span className="rounded-full bg-primary-50 px-2 py-0.5 text-primary">
                  ЕИО
                </span>
              ) : null}
              {r.isSignatory ? (
                <span className="rounded-full bg-green-50 px-2 py-0.5 text-green-700">
                  подпись
                </span>
              ) : null}
              {r.pdlDeclaration?.isPdl ? (
                <span className="rounded-full bg-amber-50 px-2 py-0.5 text-amber-800">
                  ПДЛ
                </span>
              ) : null}
            </div>
          </div>
        </li>
      ))}
    </ul>
  );
}

function Stage4Content() {
  const params = useParams<{ id: string }>();
  const applicationId = params?.id ?? "";

  // legalEntityId на данный момент не выдаётся отдельным полем Application
  // в схеме BFF (см. блокер в отчёте). Используем applicationId как
  // безопасный плейсхолдер — orchestrator делает upsert по
  // (applicationId, legalEntityId) и сам разруливает дубликаты.
  const legalEntityId = applicationId;

  const { data, loading, error, refetch } = useQuery<{
    representatives: Representative[];
  }>(QUERY_REPRESENTATIVES, {
    variables: { applicationId },
    fetchPolicy: "cache-and-network",
    skip: !applicationId,
  });

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <header className="space-y-3">
        <Link
          href="/applications"
          className="text-xs text-gray-500 hover:text-primary"
        >
          ← К списку заявок
        </Link>
        <WizardProgress currentStep={4} applicationId={applicationId} />
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">
            Этап 4 · ЕИО и представители
          </h1>
          <p className="mt-1 text-sm text-gray-500">
            Добавьте ЕИО и других представителей юрлица. Каждое лицо
            сохраняется отдельной формой.
          </p>
        </div>
      </header>

      {!applicationId ? (
        <ErrorBanner error="В URL не передан идентификатор заявки." />
      ) : (
        <>
          <section className="rounded-xl border border-gray-200 bg-white p-6">
            <h2 className="mb-3 text-base font-semibold text-gray-900">
              Уже сохранены
            </h2>
            {loading && !data ? (
              <LoadingSpinner label="Загружаем представителей…" />
            ) : error ? (
              <ErrorBanner error={error} />
            ) : (
              <RepresentativesList
                representatives={data?.representatives ?? []}
              />
            )}
          </section>
          <RepresentativeForm
            applicationId={applicationId}
            legalEntityId={legalEntityId}
            onSaved={() => {
              refetch();
            }}
          />
        </>
      )}
    </div>
  );
}

export default function Stage4Page() {
  return (
    <AuthGuard>
      <Stage4Content />
    </AuthGuard>
  );
}
