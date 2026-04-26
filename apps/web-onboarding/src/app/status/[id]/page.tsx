import { Card } from "@/components/ui/Card";
import { gqlFetch, GET_APPLICATION } from "@/lib/graphql";

const STATUS_LABELS: Record<string, { label: string; color: string }> = {
  draft:           { label: "Черновик",          color: "text-gray-500" },
  identifying:     { label: "Идентификация",     color: "text-blue-500" },
  collecting:      { label: "Сбор документов",   color: "text-blue-500" },
  validating:      { label: "Проверка",           color: "text-yellow-600" },
  auto_approved:   { label: "Одобрено",           color: "text-green-600" },
  manual_review:   { label: "На проверке",        color: "text-orange-500" },
  account_opening: { label: "Открытие счёта",     color: "text-blue-600" },
  completed:       { label: "Счёт открыт ✓",      color: "text-green-700" },
  rejected:        { label: "Отклонено",          color: "text-red-600" },
};

export default async function StatusPage({ params }: { params: { id: string } }) {
  let application: { id: string; status: string; inn: string } | null = null;
  try {
    const data = await gqlFetch<{ application: typeof application }>(GET_APPLICATION, { id: params.id });
    application = data.application;
  } catch { /* show error state */ }

  if (!application) {
    return (
      <Card>
        <p className="text-gray-500">Заявка не найдена или сервис временно недоступен.</p>
      </Card>
    );
  }

  const { label, color } = STATUS_LABELS[application.status] ?? { label: application.status, color: "text-gray-600" };

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Статус заявки</h1>
      <Card>
        <dl className="grid grid-cols-2 gap-4">
          <div><dt className="text-sm text-gray-500">Номер заявки</dt><dd className="font-mono text-sm mt-0.5">{application.id}</dd></div>
          <div><dt className="text-sm text-gray-500">ИНН</dt><dd className="font-mono text-sm mt-0.5">{application.inn}</dd></div>
          <div className="col-span-2">
            <dt className="text-sm text-gray-500">Статус</dt>
            <dd className={`text-lg font-semibold mt-0.5 ${color}`}>{label}</dd>
          </div>
        </dl>
      </Card>
    </div>
  );
}
