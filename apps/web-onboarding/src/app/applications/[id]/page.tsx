"use client";

// /applications/[id] — детали конкретной заявки.
//
// На этом экране сходится почти весь контракт BFF: state machine,
// applicant, documents и риск-оценка.  Действия зависят от текущего
// состояния — пока stub'ы (см. TODO в README).

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@apollo/client";

import { ApplicationStateTimeline } from "@/components/ApplicationStateTimeline";
import { AuthGuard } from "@/components/AuthGuard";
import { DocumentList } from "@/components/DocumentList";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { RiskBadge } from "@/components/RiskBadge";
import { StateBadge } from "@/components/StateBadge";
import { QUERY_APPLICATION } from "@/lib/graphql-operations";
import type { ApplicationDetail } from "@/types";

interface QueryData {
  application: ApplicationDetail | null;
}

function formatDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString("ru-RU", { dateStyle: "short", timeStyle: "short" });
}

function ApplicationActions({ state }: { state: string }) {
  if (state === "collecting_documents") {
    return (
      <button
        type="button"
        // TODO: открыть модальное окно загрузки документа (mutation uploadDocument).
        onClick={() =>
          // eslint-disable-next-line no-alert
          alert("Загрузка документов будет добавлена в следующей итерации.")
        }
        className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover"
      >
        Загрузить документ
      </button>
    );
  }
  if (state === "waiting_for_client") {
    return (
      <button
        type="button"
        // TODO: открыть форму ответа банку (sendDocumentsUploadedSignal или free-form).
        onClick={() =>
          // eslint-disable-next-line no-alert
          alert("Форма ответа банку будет добавлена в следующей итерации.")
        }
        className="rounded-lg border border-primary px-4 py-2 text-sm font-medium text-primary hover:bg-primary-50"
      >
        Ответить
      </button>
    );
  }
  return null;
}

function ApplicationContent() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? "";
  const { data, loading, error } = useQuery<QueryData>(QUERY_APPLICATION, {
    variables: { id },
    skip: !id,
  });

  if (!id) {
    return <ErrorBanner error="В URL не передан идентификатор заявки." />;
  }
  if (loading && !data) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner label="Загружаем заявку…" />
      </div>
    );
  }
  if (error) {
    return <ErrorBanner error={error} />;
  }
  const app = data?.application;
  if (!app) {
    return (
      <div className="rounded-xl border border-gray-200 bg-white p-6 text-sm text-gray-600">
        Заявка не найдена.{" "}
        <Link href="/applications" className="text-primary hover:underline">
          Вернуться к списку
        </Link>
        .
      </div>
    );
  }

  const risk = app.riskAssessment;
  return (
    <div className="space-y-8">
      <div>
        <Link
          href="/applications"
          className="text-sm text-gray-500 hover:text-primary"
        >
          ← К списку заявок
        </Link>
        <div className="mt-2 flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold text-gray-900">Заявка {app.id.slice(0, 8)}…</h1>
            <p className="mt-1 text-sm text-gray-500">
              Создана {formatDate(app.createdAt)} · обновлена {formatDate(app.updatedAt)}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <StateBadge state={app.state} />
            <ApplicationActions state={app.state} />
          </div>
        </div>
      </div>

      <section className="rounded-xl border border-gray-200 bg-white p-6">
        <h2 className="mb-4 text-base font-semibold text-gray-900">Этапы прохождения</h2>
        <ApplicationStateTimeline currentState={app.state} />
      </section>

      <section className="grid gap-6 md:grid-cols-2">
        <div className="rounded-xl border border-gray-200 bg-white p-6">
          <h2 className="mb-4 text-base font-semibold text-gray-900">Параметры заявки</h2>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between">
              <dt className="text-gray-500">Юр. форма</dt>
              <dd className="font-medium text-gray-900">{app.legalEntityType}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Канал</dt>
              <dd className="font-medium text-gray-900">{app.channel}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Продукты</dt>
              <dd className="font-medium text-gray-900">
                {app.productCodes.join(", ") || "—"}
              </dd>
            </div>
          </dl>
        </div>

        <div className="rounded-xl border border-gray-200 bg-white p-6">
          <h2 className="mb-4 text-base font-semibold text-gray-900">Заявитель</h2>
          {app.applicant ? (
            <dl className="space-y-2 text-sm">
              <div className="flex justify-between">
                <dt className="text-gray-500">ID</dt>
                <dd className="font-mono text-xs text-gray-700">{app.applicant.id}</dd>
              </div>
              {app.applicant.fullName ? (
                <div className="flex justify-between">
                  <dt className="text-gray-500">ФИО</dt>
                  <dd className="font-medium text-gray-900">{app.applicant.fullName}</dd>
                </div>
              ) : null}
              {app.applicant.inn ? (
                <div className="flex justify-between">
                  <dt className="text-gray-500">ИНН</dt>
                  <dd className="font-medium text-gray-900">{app.applicant.inn}</dd>
                </div>
              ) : null}
            </dl>
          ) : (
            <p className="text-sm text-gray-500">Данные заявителя ещё не получены.</p>
          )}
        </div>
      </section>

      <section>
        <h2 className="mb-4 text-base font-semibold text-gray-900">Документы</h2>
        <DocumentList documents={app.documents ?? []} />
      </section>

      <section className="rounded-xl border border-gray-200 bg-white p-6">
        <h2 className="mb-4 text-base font-semibold text-gray-900">Оценка риска</h2>
        {risk ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3">
              <RiskBadge category={risk.category} />
              <span className="text-sm text-gray-600">
                Скоринг: <strong className="text-gray-900">{risk.score.toFixed(2)}</strong>
              </span>
              <span className="text-xs text-gray-500">
                Рассчитано {formatDate(risk.computedAt)}
              </span>
            </div>
            <p className="text-sm text-gray-700">{risk.recommendation}</p>
          </div>
        ) : (
          <p className="text-sm text-gray-500">Оценка риска ещё не выполнена.</p>
        )}
      </section>
    </div>
  );
}

export default function ApplicationDetailPage() {
  return (
    <AuthGuard>
      <ApplicationContent />
    </AuthGuard>
  );
}
