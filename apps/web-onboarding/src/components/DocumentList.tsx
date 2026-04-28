// DocumentList — таблица загруженных документов с типом и статусом.

import type { DocumentRef, DocumentType } from "@/types";

const TYPE_LABELS: Record<DocumentType, string> = {
  PASSPORT: "Паспорт",
  CHARTER: "Устав",
  PROTOCOL: "Протокол / решение",
  AGREEMENT: "Договор",
  EGRUL_EXTRACT: "Выписка ЕГРЮЛ",
  POWER_OF_ATTORNEY: "Доверенность",
  ACCOUNTING_REPORT: "Бух. отчётность",
  OTHER: "Прочее",
};

function formatDate(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString("ru-RU", { dateStyle: "short", timeStyle: "short" });
}

export function DocumentList({ documents }: { documents: DocumentRef[] }) {
  if (!documents.length) {
    return (
      <div className="rounded-lg border border-dashed border-gray-300 bg-white p-6 text-center text-sm text-gray-500">
        Документы пока не загружены.
      </div>
    );
  }
  return (
    <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
      <table className="min-w-full divide-y divide-gray-200 text-sm">
        <thead className="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
          <tr>
            <th className="px-4 py-3">Тип</th>
            <th className="px-4 py-3">Файл</th>
            <th className="px-4 py-3">Статус</th>
            <th className="px-4 py-3">Загружен</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {documents.map((d) => (
            <tr key={d.id}>
              <td className="px-4 py-3 font-medium text-gray-900">
                {TYPE_LABELS[d.type] ?? d.type}
              </td>
              <td className="px-4 py-3 text-gray-700">{d.filename}</td>
              <td className="px-4 py-3 text-gray-700">{d.state}</td>
              <td className="px-4 py-3 text-gray-500">{formatDate(d.uploadedAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
