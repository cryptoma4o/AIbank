"use client";

// /applications — список заявок текущего пользователя.
//
// AuthGuard перенаправит на /login, если нет валидного JWT; Apollo читает
// токен через authLink и BFF возвращает заявки applicant'а
// (resolveMyApplications).

import Link from "next/link";
import { useQuery } from "@apollo/client";

import { AuthGuard } from "@/components/AuthGuard";
import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { StateBadge } from "@/components/StateBadge";
import { QUERY_MY_APPLICATIONS } from "@/lib/graphql-operations";
import type { ApplicationSummary } from "@/types";

interface QueryData {
  myApplications: ApplicationSummary[];
}

const LEGAL_TYPE_LABELS: Record<string, string> = {
  IP: "ИП",
  LLC: "ООО",
  JSC: "АО",
};

function formatDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString("ru-RU", { dateStyle: "short", timeStyle: "short" });
}

function ApplicationsTable({ items }: { items: ApplicationSummary[] }) {
  if (!items.length) {
    return (
      <div className="rounded-xl border border-dashed border-gray-300 bg-white p-10 text-center">
        <p className="text-gray-500">У вас пока нет заявок.</p>
        <Link
          href="/applications/new"
          className="mt-4 inline-block rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover"
        >
          Создать первую заявку
        </Link>
      </div>
    );
  }
  return (
    <div className="overflow-hidden rounded-xl border border-gray-200 bg-white">
      <table className="min-w-full divide-y divide-gray-200 text-sm">
        <thead className="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
          <tr>
            <th className="px-4 py-3">№ заявки</th>
            <th className="px-4 py-3">Статус</th>
            <th className="px-4 py-3">Юр. форма</th>
            <th className="px-4 py-3">Канал</th>
            <th className="px-4 py-3">Создана</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {items.map((app) => (
            <tr key={app.id} className="hover:bg-gray-50">
              <td className="px-4 py-3 font-medium text-primary">
                <Link href={`/applications/${app.id}`} className="hover:underline">
                  {app.id.slice(0, 8)}…
                </Link>
              </td>
              <td className="px-4 py-3">
                <StateBadge state={app.state} />
              </td>
              <td className="px-4 py-3 text-gray-700">
                {LEGAL_TYPE_LABELS[app.legalEntityType] ?? app.legalEntityType}
              </td>
              <td className="px-4 py-3 text-gray-700">{app.channel}</td>
              <td className="px-4 py-3 text-gray-500">{formatDate(app.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ApplicationsContent() {
  const { data, loading, error } = useQuery<QueryData>(QUERY_MY_APPLICATIONS);

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">Мои заявки</h1>
          <p className="mt-1 text-sm text-gray-500">
            История ваших заявок на открытие расчётного счёта.
          </p>
        </div>
        <Link
          href="/applications/new"
          className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-hover"
        >
          Новая заявка
        </Link>
      </div>

      <div className="mt-6 space-y-4">
        {error ? <ErrorBanner error={error} /> : null}
        {loading && !data ? (
          <div className="flex justify-center py-10">
            <LoadingSpinner label="Загружаем заявки…" />
          </div>
        ) : data ? (
          <ApplicationsTable items={data.myApplications} />
        ) : null}
      </div>
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
