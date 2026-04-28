// ApplicationStateBadge — цветная пилюля состояния заявки.

import { stateLabel, stateTone, toneClasses } from "@/lib/application-states";

export function ApplicationStateBadge({ state }: { state: string }) {
  const tone = stateTone(state);
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${toneClasses(
        tone
      )}`}
    >
      {stateLabel(state)}
    </span>
  );
}
