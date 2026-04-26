import { notFound } from "next/navigation";
import { StepIndicator } from "@/components/ui/StepIndicator";
import { Card } from "@/components/ui/Card";
import { DocumentUpload } from "@/components/forms/DocumentUpload";

const STEP_META: Record<
  string,
  { title: string; subtitle: string; stepIndex: number; content: React.ReactNode }
> = {
  "2": {
    title: "Загрузка документов",
    subtitle: "Шаг 2 из 5 — Документы организации",
    stepIndex: 1,
    content: (
      <Card>
        <h2 className="text-xl font-semibold text-gray-900 mb-6">Документы организации</h2>
        <div className="flex flex-col gap-6">
          <DocumentUpload label="Устав / учредительный договор" />
          <DocumentUpload label="Свидетельство о регистрации (ОГРН)" />
          <DocumentUpload label="Свидетельство о постановке на учёт (ИНН)" />
          <DocumentUpload label="Приказ о назначении руководителя" />
        </div>
      </Card>
    ),
  },
  "3": {
    title: "Проверка данных",
    subtitle: "Шаг 3 из 5 — Автоматическая верификация",
    stepIndex: 2,
    content: (
      <Card>
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Проверка данных</h2>
        <p className="text-gray-500 text-sm mb-6">
          Мы проверяем данные в реестрах ФНС, Росфинмониторинга и ФССП. Обычно занимает 1–3 минуты.
        </p>
        <div className="flex flex-col gap-3">
          {["Проверка в реестре ФНС", "AML-проверка", "Проверка руководителя", "Анализ деловой репутации"].map((item) => (
            <div key={item} className="flex items-center gap-3 py-2 border-b border-gray-100 last:border-0">
              <div className="w-5 h-5 rounded-full border-2 border-primary border-t-transparent animate-spin" />
              <span className="text-sm text-gray-700">{item}</span>
            </div>
          ))}
        </div>
      </Card>
    ),
  },
  "4": {
    title: "Подписание договора",
    subtitle: "Шаг 4 из 5 — Электронная подпись",
    stepIndex: 3,
    content: (
      <Card>
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Подписание договора</h2>
        <p className="text-gray-500 text-sm mb-6">
          Ознакомьтесь с договором на открытие расчётного счёта и подпишите его с помощью SMS-кода.
        </p>
        <div className="bg-gray-50 rounded-lg p-4 text-xs text-gray-500 h-48 overflow-y-auto mb-6">
          Договор банковского счёта № ___<br /><br />
          Публичное акционерное общество «Банк», именуемое в дальнейшем «Банк», в лице Председателя Правления,
          действующего на основании Устава, с одной стороны, и Клиент, с другой стороны, заключили настоящий
          Договор о нижеследующем…
        </div>
        <div className="flex gap-3 items-center">
          <input className="border border-gray-300 rounded-lg px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-primary w-40"
            placeholder="Код из SMS" maxLength={6} />
          <button className="px-6 py-2.5 bg-primary text-white rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors">
            Подписать
          </button>
        </div>
      </Card>
    ),
  },
  "5": {
    title: "Счёт открыт",
    subtitle: "Шаг 5 из 5 — Готово",
    stepIndex: 4,
    content: (
      <Card>
        <div className="flex flex-col items-center text-center py-8 gap-4">
          <div className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center text-3xl">
            ✓
          </div>
          <h2 className="text-2xl font-bold text-gray-900">Счёт успешно открыт!</h2>
          <p className="text-gray-500 max-w-sm">
            Реквизиты расчётного счёта отправлены на указанный email. Войдите в интернет-банк, чтобы начать работу.
          </p>
          <a href="https://bank.example.com/login"
            className="mt-4 px-8 py-3 bg-primary text-white rounded-lg font-medium hover:bg-primary-hover transition-colors">
            Перейти в интернет-банк
          </a>
        </div>
      </Card>
    ),
  },
};

export default function StepPage({ params }: { params: { step: string } }) {
  const meta = STEP_META[params.step];
  if (!meta) notFound();

  return (
    <div>
      <StepIndicator current={meta.stepIndex} />
      <h1 className="text-2xl font-bold text-gray-900 mb-2">{meta.title}</h1>
      <p className="text-gray-500 mb-8">{meta.subtitle}</p>
      {meta.content}
    </div>
  );
}

export function generateStaticParams() {
  return [{ step: "2" }, { step: "3" }, { step: "4" }, { step: "5" }];
}
