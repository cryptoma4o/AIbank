const STEPS = ["Компания", "Документы", "Проверка", "Подписание", "Счёт"];

export function StepIndicator({ current }: { current: number }) {
  return (
    <div className="flex items-center gap-2 mb-8">
      {STEPS.map((label, i) => (
        <div key={i} className="flex items-center gap-2">
          <div className={`flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium transition-colors
            ${i < current ? "bg-green-500 text-white" : i === current ? "bg-primary text-white" : "bg-gray-100 text-gray-400"}`}>
            {i < current ? "✓" : i + 1}
          </div>
          <span className={`text-sm hidden sm:block ${i === current ? "text-gray-900 font-medium" : "text-gray-400"}`}>{label}</span>
          {i < STEPS.length - 1 && <div className={`h-0.5 w-6 ${i < current ? "bg-green-500" : "bg-gray-200"}`} />}
        </div>
      ))}
    </div>
  );
}
