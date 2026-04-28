// ApplicationStateTimeline — горизонтальная цепочка success-pipeline шагов.
//
// Текущее состояние подсвечивается primary-цветом; пройденные — зелёным;
// будущие — серым.  Терминальные ветки (declined/abandoned) показываются
// отдельным баннером, чтобы не ломать визуальный поток pipeline.

import { TIMELINE_STATES, stateLabel } from "@/lib/application-states";

interface Props {
  currentState: string;
}

const TERMINAL_NEGATIVE = new Set(["declined", "abandoned"]);

export function ApplicationStateTimeline({ currentState }: Props) {
  if (TERMINAL_NEGATIVE.has(currentState)) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-danger">
        Заявка завершена со статусом «{stateLabel(currentState)}».
      </div>
    );
  }

  const currentIdx = TIMELINE_STATES.indexOf(currentState as (typeof TIMELINE_STATES)[number]);

  return (
    <ol className="flex flex-wrap items-center gap-2">
      {TIMELINE_STATES.map((s, idx) => {
        const isCurrent = idx === currentIdx;
        const isDone = currentIdx >= 0 && idx < currentIdx;
        const base = "flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium";
        const tone = isCurrent
          ? "border-primary bg-primary text-white shadow-sm"
          : isDone
            ? "border-green-200 bg-green-50 text-success"
            : "border-gray-200 bg-white text-gray-500";
        return (
          <li key={s} className={`${base} ${tone}`}>
            <span
              className={`flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-semibold ${
                isCurrent
                  ? "bg-white text-primary"
                  : isDone
                    ? "bg-success text-white"
                    : "bg-gray-100 text-gray-500"
              }`}
              aria-hidden="true"
            >
              {idx + 1}
            </span>
            {stateLabel(s)}
          </li>
        );
      })}
    </ol>
  );
}
