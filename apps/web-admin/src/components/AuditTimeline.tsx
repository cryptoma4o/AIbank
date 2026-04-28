// AuditTimeline — вертикальный таймлайн событий аудита.

import { format } from "date-fns";
import { ru } from "date-fns/locale";

import type { AuditEvent } from "@/types";

function formatStamp(iso: string): string {
  try {
    return format(new Date(iso), "dd MMM yyyy, HH:mm:ss", { locale: ru });
  } catch {
    return iso;
  }
}

const ACTION_LABELS: Record<string, string> = {
  "application.created": "Создана заявка",
  "application.state_changed": "Смена состояния",
  "application.submitted": "Отправлена",
  "decision.created": "Принято решение",
  "decision.approved": "Одобрено",
  "decision.declined": "Отказано",
  "decision.escalated": "Эскалация",
  "risk.assessed": "Риск-оценка",
  "document.uploaded": "Документ загружен",
};

function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? action;
}

export function AuditTimeline({ events }: { events: AuditEvent[] }) {
  if (!events.length) {
    return (
      <div className="rounded-md border border-dashed border-gray-300 bg-white p-6 text-center text-sm text-gray-500">
        Аудит-событий по этой заявке пока нет.
      </div>
    );
  }
  return (
    <ol className="relative space-y-4 border-l-2 border-gray-200 pl-5">
      {events.map((event) => (
        <li key={event.id} className="relative">
          <span
            aria-hidden="true"
            className="absolute -left-[27px] top-1.5 h-3 w-3 rounded-full border-2 border-primary bg-white"
          />
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="text-sm font-medium text-gray-900">
              {actionLabel(event.action)}
            </span>
            <span className="text-xs text-gray-500">
              {formatStamp(event.occurredAt)}
            </span>
          </div>
          <div className="mt-0.5 text-xs text-gray-600">
            <span className="font-mono">{event.actorType}</span>
            {event.actorId ? (
              <span className="ml-1 font-mono text-gray-500">
                {event.actorId.slice(0, 12)}
                {event.actorId.length > 12 ? "…" : ""}
              </span>
            ) : null}
            {event.subjectType ? (
              <span className="ml-3 text-gray-500">
                → {event.subjectType}:
                <span className="ml-1 font-mono">
                  {event.subjectId?.slice(0, 8)}
                  {event.subjectId && event.subjectId.length > 8 ? "…" : ""}
                </span>
              </span>
            ) : null}
          </div>
        </li>
      ))}
    </ol>
  );
}
