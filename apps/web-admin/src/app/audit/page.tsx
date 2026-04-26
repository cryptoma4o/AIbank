import { gqlFetch, GET_AUDIT_EVENTS } from "@/lib/graphql";
import type { AuditEvent } from "@/types";

export default async function AuditPage({
  searchParams,
}: {
  searchParams: { tenant_id?: string };
}) {
  const tenantId = searchParams.tenant_id ?? "demo-bank";
  let events: AuditEvent[] = [];
  try {
    const data = await gqlFetch<{ auditEvents: AuditEvent[] }>(GET_AUDIT_EVENTS, {
      tenant_id: tenantId,
      limit: 50,
    });
    events = data.auditEvents ?? [];
  } catch { /* offline — show empty */ }

  return (
    <div>
      <h1 className="text-xl font-bold text-gray-900 mb-6">Журнал аудита</h1>
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 border-b border-gray-200">
            <tr>
              {["Событие", "Актор", "Ресурс", "Время"].map((h) => (
                <th key={h} className="px-4 py-3 text-left text-xs font-semibold text-gray-500 uppercase tracking-wide">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {events.length === 0 ? (
              <tr><td colSpan={4} className="px-4 py-8 text-center text-gray-400 text-sm">Нет данных (BFF недоступен)</td></tr>
            ) : (
              events.map((e) => (
                <tr key={e.id} className="hover:bg-gray-50 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-blue-600">{e.event_type}</td>
                  <td className="px-4 py-3 text-gray-600 font-mono text-xs">{e.actor_id}</td>
                  <td className="px-4 py-3 text-gray-600 font-mono text-xs">{e.resource_id}</td>
                  <td className="px-4 py-3 text-gray-400 text-xs">{new Date(e.occurred_at).toLocaleString("ru-RU")}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
