// DocumentList — таблица загруженных документов.
//
// На текущий момент bff-admin не возвращает documents через GraphQL
// (см. ADR-0003 — тонкая обёртка), поэтому компонент работает с
// произвольным интерфейсом DocumentRef и используется страницей детали
// заявки только когда документы реально пришли.  Когда добавим
// `documents` в схему bff-admin — компонент уже готов.

export interface AdminDocument {
  id: string;
  type: string;
  state: string;
  filename?: string;
  sha256?: string;
  uploadedAt?: string;
  downloadUrl?: string;
}

const TYPE_LABELS: Record<string, string> = {
  PASSPORT: "Паспорт",
  CHARTER: "Устав",
  PROTOCOL: "Протокол / решение",
  AGREEMENT: "Договор",
  EGRUL_EXTRACT: "Выписка ЕГРЮЛ",
  POWER_OF_ATTORNEY: "Доверенность",
  ACCOUNTING_REPORT: "Бух. отчётность",
  OTHER: "Прочее",
};

export function DocumentList({ documents }: { documents: AdminDocument[] }) {
  if (!documents.length) {
    return (
      <div className="rounded-md border border-dashed border-gray-300 bg-white p-6 text-center text-sm text-gray-500">
        Документы не загружены или не отдаются текущей версией bff-admin.
      </div>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-gray-200 bg-white">
      <table className="min-w-full divide-y divide-gray-200 text-sm">
        <thead className="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
          <tr>
            <th className="px-3 py-2">Тип</th>
            <th className="px-3 py-2">Файл</th>
            <th className="px-3 py-2">Статус</th>
            <th className="px-3 py-2">SHA-256</th>
            <th className="px-3 py-2"> </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {documents.map((d) => (
            <tr key={d.id}>
              <td className="px-3 py-2 font-medium text-gray-900">
                {TYPE_LABELS[d.type] ?? d.type}
              </td>
              <td className="px-3 py-2 text-gray-700">{d.filename ?? "—"}</td>
              <td className="px-3 py-2 text-gray-700">{d.state}</td>
              <td className="px-3 py-2 font-mono text-xs text-gray-500">
                {d.sha256 ? `${d.sha256.slice(0, 12)}…` : "—"}
              </td>
              <td className="px-3 py-2 text-right">
                {d.downloadUrl ? (
                  <a
                    href={d.downloadUrl}
                    className="text-primary hover:underline"
                    rel="noopener noreferrer"
                    target="_blank"
                  >
                    Скачать
                  </a>
                ) : (
                  <span className="text-gray-400">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
