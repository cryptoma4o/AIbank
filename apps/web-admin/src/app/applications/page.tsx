"use client";

// /applications — список заявок текущего тенанта с фильтрами.
//
// bff-admin принимает только filter.state и filter.legalEntityType, а
// поиск по INN и диапазон дат — отсутствуют в schema.  Поэтому строку
// поиска и даты мы применяем КЛИЕНТСКИ, после получения списка.
// (Когда схема расширится — переключимся на серверную фильтрацию.)

import Link from "next/link";
import { useMemo, useState } from "react";
import { useQuery } from "@apollo/client";

import { ApplicationFilters, EMPTY_FILTERS, type FiltersValue } from "@/components/ApplicationFilters";
import { ApplicationStateBadge } from "@/components/ApplicationStateBadge";
import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { RiskBadge } from "@/components/RiskBadge";
import { QUERY_APPLICATIONS } from "@/lib/graphql-operations";
import type { ApplicationSummary } from "@/types";

const LEGAL_TYPE_LABELS: Record<string, string> = {
  IP: "ИП",
  LLC: "ООО",
  JSC: "АО",
  INDIVIDUAL: "Физлицо",
};

interface QueryData {
  applications: ApplicationSummary[];
}

function formatDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString("ru-RU", { dateStyle: "short", timeStyle: "short" });
}

function ApplicationsContent() {
  const [filters, setFilters] = useState<FiltersValue>(EMPTY_FILTERS);

  // Серверная фильтрация: только если выбрано РОВНО одно состояние.
  // Если выбраны 2+, серверу всё равно отправляем без state, и фильтруем
  // на клиенте (схема не поддерживает массив).
  const serverFilter = useMemo(() => {
    if (filters.states.size === 1) {
      const [only] = Array.from(filters.states);
      return { state: only };
    }
    return undefined;
  }, [filters.states]);

  const { data, loading, error, refetch } = useQuery<QueryData>(
    QUERY_APPLICATIONS,
    { variables: { filter: serverFilter } }
  );

  const items = useMemo(() => {
    const all = data?.applications ?? [];
    const q = filters.query.trim();
    const dateFromTs = filters.dateFrom
      ? new Date(`${filters.dateFrom}T00:00:00`).getTime()
      : null;
    const dateToTs = filters.dateTo
      ? new Date(`${filters.dateTo}T23:59:59`).getTime()
      : null;
    return all.filter((app) => {
      if (filters.states.size > 0 && !filters.states.has(app.state)) {
        return false;
      }
      if (q && !app.id.includes(q) && !(app.applicantId ?? "").includes(q)) {
        // INN не возвращается схемой → фильтр по applicantId/id, как
        // приближение.  Когда BFF отдаст inn — здесь сравним с ним.
        return false;
      }
      if (dateFromTs || dateToTs) {
        const ts = new Date(app.createdAt).getTime();
        if (dateFromTs && ts < dateFromTs) return false;
        if (dateToTs && ts > dateToTs) return false;
      }
      return true;
    });
  }, [data, filters]);

  return (
    <div className="flex gap-6">
      <ApplicationFilters
        value={filters}
        onChange={setFilters}
        onReset={() => {
          setFilters({ ...EMPTY_FILTERS, states: new Set() });
          void refetch({ filter: undefined });
        }}
      />

      <section className="flex-1">
        <div className="mb-4 flex items-baseline justify-between">
          <div>
            <h1 className="text-xl font-semibold text-gray-900">Заявки</h1>
            <p className="text-sm text-gray-500">
              {loading && !data
                ? "Загружаем…"
                : `Показано ${items.length} из ${data?.applications.length ?? 0}`}
            </p>
          </div>
        </div>

        {error ? <ErrorBanner error={error} /> : null}

        {loading && !data ? (
          <div className="flex justify-center py-10">
            <LoadingSpinner label="Загружаем заявки…" />
          </div>
        ) : (
          <div className="overflow-hidden rounded-md border border-gray-200 bg-white">
            <table className="min-w-full divide-y divide-gray-200 text-sm">
              <thead className="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                <tr>
                  <th className="px-3 py-2">№ заявки</th>
                  <th className="px-3 py-2">Состояние</th>
                  <th className="px-3 py-2">Юр. форма</th>
                  <th className="px-3 py-2">Заявитель</th>
                  <th className="px-3 py-2">Риск</th>
                  <th className="px-3 py-2">Создана</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {items.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-8 text-center text-sm text-gray-400"
                    >
                      Заявок по выбранным фильтрам нет.
                    </td>
                  </tr>
                ) : (
                  items.map((app) => (
                    <tr
                      key={app.id}
                      className="cursor-pointer hover:bg-gray-50"
                      onClick={() => {
                        window.location.href = `/applications/${app.id}`;
                      }}
                    >
                      <td className="px-3 py-2 font-mono text-xs text-primary">
                        <Link
                          href={`/applications/${app.id}`}
                          onClick={(e) => e.stopPropagation()}
                          className="hover:underline"
                        >
                          {app.id.slice(0, 8)}…
                        </Link>
                      </td>
                      <td className="px-3 py-2">
                        <ApplicationStateBadge state={app.state} />
                      </td>
                      <td className="px-3 py-2 text-gray-700">
                        {LEGAL_TYPE_LABELS[app.legalEntityType] ??
                          app.legalEntityType}
                      </td>
                      <td className="px-3 py-2 font-mono text-xs text-gray-500">
                        {app.applicantId
                          ? `${app.applicantId.slice(0, 8)}…`
                          : "—"}
                      </td>
                      <td className="px-3 py-2">
                        {app.riskAssessment ? (
                          <RiskBadge category={app.riskAssessment.category} />
                        ) : (
                          <span className="text-xs text-gray-400">—</span>
                        )}
                      </td>
                      <td className="px-3 py-2 text-xs text-gray-500">
                        {formatDate(app.createdAt)}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

export default function ApplicationsPage() {
  return (
    <AuthGuard>
      <ApplicationsContent />
    </AuthGuard>
  );
}
