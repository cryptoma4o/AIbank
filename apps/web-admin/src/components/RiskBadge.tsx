// RiskBadge — визуальный индикатор риск-категории заявки.

import type { RiskCategory } from "@/types";

const STYLES: Record<RiskCategory, { label: string; classes: string }> = {
  LOW: {
    label: "Низкий риск",
    classes: "bg-green-50 text-success border-green-200",
  },
  MEDIUM: {
    label: "Средний риск",
    classes: "bg-amber-50 text-warning border-amber-200",
  },
  HIGH: {
    label: "Высокий риск",
    classes: "bg-red-50 text-danger border-red-200",
  },
};

export function RiskBadge({ category }: { category: RiskCategory }) {
  const style = STYLES[category];
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${style.classes}`}
    >
      {style.label}
    </span>
  );
}
