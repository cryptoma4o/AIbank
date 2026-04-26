const STATUS_MAP: Record<string, { label: string; cls: string }> = {
  draft:           { label: "Черновик",       cls: "bg-gray-100 text-gray-600" },
  identifying:     { label: "Идентификация", cls: "bg-blue-50 text-blue-700" },
  collecting:      { label: "Документы",     cls: "bg-blue-50 text-blue-700" },
  validating:      { label: "Проверка",      cls: "bg-yellow-50 text-yellow-700" },
  auto_approved:   { label: "Одобрено",      cls: "bg-green-50 text-green-700" },
  manual_review:   { label: "На проверке",   cls: "bg-orange-50 text-orange-700" },
  account_opening: { label: "Открытие",      cls: "bg-indigo-50 text-indigo-700" },
  completed:       { label: "Завершено",     cls: "bg-green-100 text-green-800" },
  rejected:        { label: "Отклонено",     cls: "bg-red-50 text-red-700" },
};

export function Badge({ status }: { status: string }) {
  const { label, cls } = STATUS_MAP[status] ?? { label: status, cls: "bg-gray-100 text-gray-600" };
  return <span className={`inline-flex px-2.5 py-0.5 rounded-full text-xs font-medium ${cls}`}>{label}</span>;
}
