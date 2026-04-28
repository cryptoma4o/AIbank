"use client";

// /applications/[id] — детальная карточка заявки для оператора.
//
// На этой странице оператор:
//   - видит все ключевые атрибуты заявки и риск-оценку
//   - может принять решение (Approve / Approve+EDD / Decline / Escalate),
//     если состояние manual_review/requires_more_info, и роль это разрешает
//   - смотрит timeline аудит-событий по заявке
//
// Документы и подробные данные заявителя пока не отдаются bff-admin
// (см. ADR-0003 и комментарии в graph/resolver.go), поэтому соответствующие
// блоки умеют рендериться без данных.

import Link from "next/link";
import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@apollo/client";

import { ApplicationStateBadge } from "@/components/ApplicationStateBadge";
import { AuditTimeline } from "@/components/AuditTimeline";
import { AuthGuard } from "@/components/AuthGuard";
import { DecisionModal } from "@/components/DecisionModal";
import { DocumentList } from "@/components/DocumentList";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { RiskBadge } from "@/components/RiskBadge";
import { hasAnyRole } from "@/lib/auth";
import { isDecisionState } from "@/lib/application-states";
import {
  MUTATION_UPDATE_DECISION,
  QUERY_APPLICATION,
  QUERY_AUDIT_EVENTS,
} from "@/lib/graphql-operations";
import { DECISION_ROLES } from "@/lib/roles";
import type {
  ApplicationDetail,
  AuditEvent,
  Decision,
  DecisionKind,
  RiskFactor,
} from "@/types";

const LEGAL_TYPE_LABELS: Record<string, string> = {
  IP: "ИП",
  LLC: "ООО",
  JSC: "АО",
  INDIVIDUAL: "Физлицо",
};

function formatDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString("ru-RU", { dateStyle: "short", timeStyle: "medium" });
}

interface ApplicationData {
  application: ApplicationDetail | null;
}

interface AuditData {
  auditEvents: AuditEvent[];
}

function HeaderSection({
  app,
  canDecide,
  onOpen,
}: {
  app: ApplicationDetail;
  canDecide: boolean;
  onOpen: (kind: DecisionKind) => void;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 rounded-md border border-gray-200 bg-white p-4">
      <div>
        <div className="flex items-center gap-3">
          <h1 className="font-mono text-base text-gray-900">{app.id}</h1>
          <ApplicationStateBadge state={app.state} />
          {app.riskAssessment ? (
            <RiskBadge category={app.riskAssessment.category} />
          ) : null}
        </div>
        <p className="mt-1 text-sm text-gray-500">
          Создана {formatDate(app.createdAt)} · обновлена{" "}
          {formatDate(app.updatedAt)} · канал {app.channel}
        </p>
      </div>
      {canDecide ? (
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => onOpen("APPROVED")}
            className="rounded-md bg-success px-3 py-1.5 text-sm font-medium text-white hover:bg-green-700"
          >
            Одобрить
          </button>
          <button
            type="button"
            onClick={() => onOpen("APPROVED_WITH_EDD")}
            className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-primary-hover"
          >
            Одобрить с EDD
          </button>
          <button
            type="button"
            onClick={() => onOpen("DECLINED")}
            className="rounded-md bg-danger px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700"
          >
            Отказать
          </button>
          <button
            type="button"
            onClick={() => onOpen("ESCALATED")}
            className="rounded-md border border-amber-300 bg-amber-50 px-3 py-1.5 text-sm font-medium text-amber-800 hover:bg-amber-100"
            title="Эскалация старшему офицеру"
          >
            Эскалировать
          </button>
        </div>
      ) : null}
    </header>
  );
}

function ApplicantSection({ app }: { app: ApplicationDetail }) {
  return (
    <section className="rounded-md border border-gray-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-gray-900">Заявитель</h2>
      <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
        <div>
          <dt className="text-xs uppercase tracking-wider text-gray-500">
            Applicant ID
          </dt>
          <dd className="font-mono text-gray-800">
            {app.applicantId ?? "—"}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wider text-gray-500">
            Юр. форма
          </dt>
          <dd className="text-gray-800">
            {LEGAL_TYPE_LABELS[app.legalEntityType] ?? app.legalEntityType}
          </dd>
        </div>
        <div className="col-span-2">
          <dt className="text-xs uppercase tracking-wider text-gray-500">
            Продукты
          </dt>
          <dd className="text-gray-800">
            {app.productCodes.length ? app.productCodes.join(", ") : "—"}
          </dd>
        </div>
      </dl>
      <p className="mt-3 text-xs text-gray-500">
        Расширенные данные заявителя (ФИО, ИНН, контакты) и юрлица будут
        доступны после расширения схемы bff-admin.
      </p>
    </section>
  );
}

function FactorList({
  title,
  tone,
  items,
}: {
  title: string;
  tone: "success" | "danger" | "neutral";
  items: RiskFactor[];
}) {
  if (!items.length) return null;
  const toneCls =
    tone === "success"
      ? "border-green-200 bg-green-50"
      : tone === "danger"
      ? "border-red-200 bg-red-50"
      : "border-gray-200 bg-gray-50";
  return (
    <div className={`rounded-md border p-3 text-sm ${toneCls}`}>
      <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-700">
        {title}
      </h3>
      <ul className="mt-2 space-y-1.5">
        {items.map((f, i) => (
          <li
            key={`${f.code ?? "factor"}-${i}`}
            className="flex items-baseline justify-between gap-3"
          >
            <span className="text-gray-800">
              {f.label ?? f.code ?? "—"}
              {f.details ? (
                <span className="ml-1 text-xs text-gray-500">— {f.details}</span>
              ) : null}
            </span>
            {typeof f.weight === "number" ? (
              <span className="font-mono text-xs text-gray-600">
                {f.weight > 0 ? "+" : ""}
                {f.weight.toFixed(2)}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

function RiskSection({ app }: { app: ApplicationDetail }) {
  if (!app.riskAssessment) {
    return (
      <section className="rounded-md border border-gray-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-gray-900">Риск-оценка</h2>
        <p className="mt-2 text-sm text-gray-500">
          Риск ещё не рассчитан или risk-scoring-service недоступен.
        </p>
      </section>
    );
  }
  const ra = app.riskAssessment;
  const factors: RiskFactor[] = (ra.factors as RiskFactor[] | null) ?? [];
  const positive = factors.filter((f) => f.direction === "positive");
  const negative = factors.filter((f) => f.direction === "negative");
  const other = factors.filter(
    (f) => f.direction !== "positive" && f.direction !== "negative"
  );

  return (
    <section className="rounded-md border border-gray-200 bg-white p-4">
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-gray-900">Риск-оценка</h2>
        <span className="text-xs text-gray-500">
          обновлено {formatDate(ra.computedAt)}
        </span>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-3 text-sm">
        <div className="rounded-md border border-gray-100 bg-gray-50 p-3">
          <p className="text-xs uppercase tracking-wider text-gray-500">Score</p>
          <p className="mt-1 text-2xl font-semibold text-gray-900">
            {ra.score.toFixed(2)}
          </p>
        </div>
        <div className="rounded-md border border-gray-100 bg-gray-50 p-3">
          <p className="text-xs uppercase tracking-wider text-gray-500">
            Категория
          </p>
          <p className="mt-1">
            <RiskBadge category={ra.category} />
          </p>
        </div>
        <div className="col-span-1 rounded-md border border-gray-100 bg-gray-50 p-3">
          <p className="text-xs uppercase tracking-wider text-gray-500">
            Рекомендация
          </p>
          <p className="mt-1 text-sm text-gray-800">{ra.recommendation}</p>
        </div>
      </div>

      {factors.length > 0 ? (
        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <FactorList title="Положительные" tone="success" items={positive} />
          <FactorList title="Отрицательные" tone="danger" items={negative} />
          {other.length > 0 ? (
            <FactorList title="Другие правила" tone="neutral" items={other} />
          ) : null}
        </div>
      ) : (
        <p className="mt-3 text-xs text-gray-500">
          Детализация факторов отсутствует.
        </p>
      )}
    </section>
  );
}

function DecisionSection({
  decision,
}: {
  decision: Decision | null | undefined;
}) {
  if (!decision) return null;
  return (
    <section className="rounded-md border border-gray-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-gray-900">Принятое решение</h2>
      <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
        <div>
          <dt className="text-xs uppercase tracking-wider text-gray-500">Вид</dt>
          <dd className="font-medium text-gray-800">{decision.decision}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wider text-gray-500">
            Принято
          </dt>
          <dd className="text-gray-800">{formatDate(decision.decidedAt)}</dd>
        </div>
        <div className="col-span-2">
          <dt className="text-xs uppercase tracking-wider text-gray-500">
            Reasoning
          </dt>
          <dd className="text-gray-800 whitespace-pre-wrap">
            {decision.reasoning}
          </dd>
        </div>
      </dl>
    </section>
  );
}

function ApplicationContent({ id }: { id: string }) {
  const { data, loading, error, refetch } = useQuery<ApplicationData>(
    QUERY_APPLICATION,
    { variables: { id } }
  );
  const auditQuery = useQuery<AuditData>(QUERY_AUDIT_EVENTS, {
    // bff-admin фильтрует только по actor/action; «по applicationId» не
    // умеет — подгружаем последние 50 событий тенанта и фильтруем на
    // клиенте.  Когда схема расширится — переключимся.
    variables: { filter: { limit: 50 } },
  });
  const [updateDecision] = useMutation(MUTATION_UPDATE_DECISION);

  const [modal, setModal] = useState<DecisionKind | null>(null);

  const canDecide = hasAnyRole(DECISION_ROLES);
  const app = data?.application ?? null;
  const showDecisionButtons = !!app && isDecisionState(app.state) && canDecide;

  const events = useMemo(() => {
    const all = auditQuery.data?.auditEvents ?? [];
    return all.filter(
      (e) => e.subjectType === "application" && e.subjectId === id
    );
  }, [auditQuery.data, id]);

  if (loading && !data) {
    return (
      <div className="flex justify-center py-10">
        <LoadingSpinner label="Загружаем заявку…" />
      </div>
    );
  }
  if (error) return <ErrorBanner error={error} />;
  if (!app) {
    return (
      <div className="rounded-md border border-dashed border-gray-300 bg-white p-10 text-center text-sm text-gray-500">
        Заявка не найдена.{" "}
        <Link href="/applications" className="text-primary hover:underline">
          Вернуться к списку
        </Link>
      </div>
    );
  }

  async function handleDecisionSubmit(input: {
    applicationId: string;
    decision: DecisionKind;
    reasoning: string;
  }) {
    await updateDecision({ variables: { input } });
    await refetch();
    await auditQuery.refetch();
  }

  return (
    <div className="space-y-4">
      <div>
        <Link
          href="/applications"
          className="text-xs text-primary hover:underline"
        >
          ← К списку заявок
        </Link>
      </div>

      <HeaderSection
        app={app}
        canDecide={showDecisionButtons}
        onOpen={(k) => setModal(k)}
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <ApplicantSection app={app} />
        <DecisionSection decision={app.decision} />
      </div>

      <RiskSection app={app} />

      <section className="rounded-md border border-gray-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-gray-900">Документы</h2>
        <div className="mt-3">
          <DocumentList documents={[]} />
        </div>
      </section>

      <section className="rounded-md border border-gray-200 bg-white p-4">
        <div className="flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-gray-900">Аудит-события</h2>
          {auditQuery.loading ? (
            <LoadingSpinner />
          ) : (
            <span className="text-xs text-gray-500">
              {events.length} событий
            </span>
          )}
        </div>
        <div className="mt-4">
          {auditQuery.error ? <ErrorBanner error={auditQuery.error} /> : null}
          <AuditTimeline events={events} />
        </div>
      </section>

      <DecisionModal
        open={modal !== null}
        decision={modal ?? "APPROVED"}
        applicationId={app.id}
        onClose={() => setModal(null)}
        onSubmit={handleDecisionSubmit}
      />
    </div>
  );
}

export default function ApplicationDetailPage({
  params,
}: {
  params: { id: string };
}) {
  return (
    <AuthGuard>
      <ApplicationContent id={params.id} />
    </AuthGuard>
  );
}
