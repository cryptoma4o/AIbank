"use client";

// /audit — журнал аудита тенанта.  Доступен только bank.admin и
// platform.admin (см. AUDIT_ROLES).
//
// bff-admin поддерживает фильтры filter.actor (substr-match по actor_id),
// filter.action (action.code) и limit.  Дату и тип сущности сужаем
// клиентски, потому что на v0.1 схема не отдаёт subjectType-фильтр.

import { useMemo, useState } from "react";
import { useQuery } from "@apollo/client";
import { format } from "date-fns";
import { ru } from "date-fns/locale";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { QUERY_AUDIT_EVENTS } from "@/lib/graphql-operations";
import { AUDIT_ROLES } from "@/lib/roles";
import type { AuditEvent } from "@/types";

interface QueryData {
  auditEvents: AuditEvent[];
}

const ACTOR_TYPES = ["", "user", "applicant", "system", "agent"];
const ENTITY_TYPES = ["", "application", "document", "decision", "tenant", "user"];

function formatStamp(v: string): string {
  try {
    return format(new Date(v), "dd.MM.yyyy HH:mm:ss", { locale: ru });
  } catch {
    return v;
  }
}

function ExpandableJSON({
  data,
}: {
  data: Record<string, unknown> | null | undefined;
}) {
  const [open, setOpen] = useState(false);
  if (!data || Object.keys(data).length === 0) {
    return <span className="text-xs text-gray-400">—</span>;
  }
  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="text-xs text-primary hover:underline"
      >
        показать ({Object.keys(data).length})
      </button>
    );
  }
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen(false)}
        className="text-xs text-primary hover:underline"
      >
        скрыть
      </button>
      <pre className="mt-1 max-w-md overflow-x-auto rounded bg-gray-50 p-2 font-mono text-[11px] leading-snug text-gray-700">
        {JSON.stringify(data, null, 2)}
      </pre>
    </div>
  );
}

function AuditContent() {
  const [actor, setActor] = useState("");
  const [action, setAction] = useState("");
  const [actorType, setActorType] = useState("");
  const [entityType, setEntityType] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [limit, setLimit] = useState<number>(50);

  const { data, loading, error, refetch } = useQuery<QueryData>(
    QUERY_AUDIT_EVENTS,
    {
      variables: {
        filter: {
          actor: actor || undefined,
          action: action || undefined,
          limit,
        },
      },
    }
  );

  const filtered = useMemo(() => {
    const all = data?.auditEvents ?? [];
    const fromTs = dateFrom ? new Date(`${dateFrom}T00:00:00`).getTime() : null;
    const toTs = dateTo ? new Date(`${dateTo}T23:59:59`).getTime() : null;
    return all.filter((e) => {
      if (actorType && e.actorType !== actorType) return false;
      if (entityType && e.subjectType !== entityType) return false;
      if (fromTs || toTs) {
        const ts = new Date(e.occurredAt).getTime();
        if (fromTs && ts < fromTs) return false;
        if (toTs && ts > toTs) return false;
      }
      return true;
    });
  }, [data, actorType, entityType, dateFrom, dateTo]);

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Аудит-журнал</h1>
        <p className="text-sm text-gray-500">
          Хеш-связные события тенанта (см. требования по 115-ФЗ и журнал ЦБ РФ).
        </p>
      </div>

      <form
        className="grid grid-cols-2 gap-3 rounded-md border border-gray-200 bg-white p-4 md:grid-cols-6"
        onSubmit={(e) => {
          e.preventDefault();
          void refetch();
        }}
      >
        <div className="col-span-2">
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Actor (substr)
          </label>
          <input
            type="text"
            value={actor}
            onChange={(e) => setActor(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm"
          />
        </div>
        <div className="col-span-2">
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Action
          </label>
          <input
            type="text"
            value={action}
            onChange={(e) => setAction(e.target.value)}
            placeholder="application.state_changed"
            className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Actor type
          </label>
          <select
            value={actorType}
            onChange={(e) => setActorType(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm"
          >
            {ACTOR_TYPES.map((t) => (
              <option key={t} value={t}>
                {t || "любой"}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Entity type
          </label>
          <select
            value={entityType}
            onChange={(e) => setEntityType(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm"
          >
            {ENTITY_TYPES.map((t) => (
              <option key={t} value={t}>
                {t || "любая"}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Дата с
          </label>
          <input
            type="date"
            value={dateFrom}
            onChange={(e) => setDateFrom(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Дата по
          </label>
          <input
            type="date"
            value={dateTo}
            onChange={(e) => setDateTo(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">
            Limit
          </label>
          <input
            type="number"
            min={10}
            max={500}
            step={10}
            value={limit}
            onChange={(e) =>
              setLimit(Math.max(10, Number(e.target.value) || 50))
            }
            className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm"
          />
        </div>
        <div className="flex items-end">
          <button
            type="submit"
            className="w-full rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-primary-hover"
          >
            Применить
          </button>
        </div>
      </form>

      {error ? <ErrorBanner error={error} /> : null}

      <div className="overflow-hidden rounded-md border border-gray-200 bg-white">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
            <tr>
              <th className="px-3 py-2">Время</th>
              <th className="px-3 py-2">Actor</th>
              <th className="px-3 py-2">Action</th>
              <th className="px-3 py-2">Subject</th>
              <th className="px-3 py-2">Hash</th>
              <th className="px-3 py-2">Payload</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {loading && !data ? (
              <tr>
                <td colSpan={6} className="px-3 py-6 text-center">
                  <LoadingSpinner label="Загружаем события…" />
                </td>
              </tr>
            ) : filtered.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="px-3 py-8 text-center text-sm text-gray-400"
                >
                  Нет событий по выбранным фильтрам.
                </td>
              </tr>
            ) : (
              filtered.map((e) => (
                <tr key={e.id} className="align-top hover:bg-gray-50">
                  <td className="whitespace-nowrap px-3 py-2 text-xs text-gray-700">
                    {formatStamp(e.occurredAt)}
                  </td>
                  <td className="px-3 py-2 text-xs">
                    <span className="font-medium text-gray-700">
                      {e.actorType}
                    </span>
                    <span className="ml-1 font-mono text-gray-500">
                      {e.actorId.slice(0, 12)}
                      {e.actorId.length > 12 ? "…" : ""}
                    </span>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-primary">
                    {e.action}
                  </td>
                  <td className="px-3 py-2 text-xs text-gray-700">
                    {e.subjectType}
                    <span className="ml-1 font-mono text-gray-500">
                      {e.subjectId.slice(0, 8)}
                      {e.subjectId.length > 8 ? "…" : ""}
                    </span>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-500">
                    {e.id.slice(0, 8)}…
                  </td>
                  <td className="px-3 py-2">
                    <ExpandableJSON data={e.data ?? null} />
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default function AuditPage() {
  return (
    <AuthGuard allowRoles={AUDIT_ROLES}>
      <AuditContent />
    </AuthGuard>
  );
}
