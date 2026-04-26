import { StatCard } from "@/components/ui/StatCard";

export default function DashboardPage() {
  return (
    <div>
      <h1 className="text-xl font-bold text-gray-900 mb-6">Дашборд</h1>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <StatCard label="Заявок сегодня"   value="—" sub="загрузка..." />
        <StatCard label="На проверке"       value="—" sub="загрузка..." />
        <StatCard label="Одобрено (месяц)" value="—" sub="загрузка..." />
        <StatCard label="Активных тенантов" value="—" sub="загрузка..." />
      </div>
      <div className="bg-white rounded-xl border border-gray-200 p-5">
        <h2 className="text-sm font-semibold text-gray-700 mb-4">Последние заявки</h2>
        <p className="text-sm text-gray-400">Подключите GraphQL BFF для отображения данных.</p>
      </div>
    </div>
  );
}
