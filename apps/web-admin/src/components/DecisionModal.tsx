"use client";

// DecisionModal — универсальный модал для принятия решения по заявке.
//
// Используется в /applications/[id]/page.tsx из трёх кнопок:
// «Одобрить», «Одобрить с EDD», «Отказать».  Комментарий обязателен и
// должен быть >= 20 символов: это тот reasoning, который попадёт в
// audit log и регулятору при выгрузке.

import { useEffect, useRef, useState } from "react";

import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import type { DecisionKind } from "@/types";

const REASONING_MIN_CHARS = 20;

const DECISION_TITLES: Record<DecisionKind, string> = {
  APPROVED: "Одобрить заявку",
  APPROVED_WITH_EDD: "Одобрить с EDD",
  DECLINED: "Отказать в открытии счёта",
  ESCALATED: "Эскалировать заявку",
};

const DECISION_HINTS: Record<DecisionKind, string> = {
  APPROVED:
    "Подтвердите, что комплаенс-проверки пройдены. Reasoning будет приложен к решению.",
  APPROVED_WITH_EDD:
    "Усиленная идентификация (EDD) запрашивается у клиента. Опишите причину.",
  DECLINED:
    "Откажем в открытии счёта. Укажите формальное основание (ст. 7 115-ФЗ и т. п.).",
  ESCALATED:
    "Эскалация старшему офицеру / руководителю. Кратко опишите проблему.",
};

interface Props {
  open: boolean;
  decision: DecisionKind;
  applicationId: string;
  onClose: () => void;
  onSubmit: (input: {
    applicationId: string;
    decision: DecisionKind;
    reasoning: string;
  }) => Promise<void>;
}

export function DecisionModal({
  open,
  decision,
  applicationId,
  onClose,
  onSubmit,
}: Props) {
  const [reasoning, setReasoning] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const dialogRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (open) {
      setReasoning("");
      setError(null);
      setSubmitting(false);
      // Простой фокус на textarea при открытии.
      setTimeout(() => {
        const ta = dialogRef.current?.querySelector("textarea");
        ta?.focus();
      }, 50);
    }
  }, [open, decision]);

  if (!open) return null;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const trimmed = reasoning.trim();
    if (trimmed.length < REASONING_MIN_CHARS) {
      setError(`Комментарий обязателен и должен содержать не менее ${REASONING_MIN_CHARS} символов.`);
      return;
    }
    setSubmitting(true);
    try {
      await onSubmit({ applicationId, decision, reasoning: trimmed });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось сохранить решение");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="decision-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => {
        // Закрытие по клику на бэкдроп (не на саму карточку).
        if (e.target === e.currentTarget && !submitting) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="w-full max-w-lg rounded-lg border border-gray-200 bg-white p-6 shadow-xl"
      >
        <h2
          id="decision-dialog-title"
          className="text-base font-semibold text-gray-900"
        >
          {DECISION_TITLES[decision]}
        </h2>
        <p className="mt-1 text-sm text-gray-600">{DECISION_HINTS[decision]}</p>

        <form className="mt-4 space-y-4" onSubmit={handleSubmit} noValidate>
          {error ? <ErrorBanner error={error} /> : null}
          <div>
            <label
              htmlFor="decision-reasoning"
              className="mb-1 block text-sm font-medium text-gray-700"
            >
              Комментарий (≥ {REASONING_MIN_CHARS} символов)
            </label>
            <textarea
              id="decision-reasoning"
              rows={5}
              className="w-full resize-vertical rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-primary"
              value={reasoning}
              onChange={(e) => setReasoning(e.target.value)}
              disabled={submitting}
              required
              minLength={REASONING_MIN_CHARS}
            />
            <p className="mt-1 text-xs text-gray-500">
              {reasoning.trim().length}/{REASONING_MIN_CHARS}+
            </p>
          </div>

          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-60"
            >
              Отмена
            </button>
            <button
              type="submit"
              disabled={submitting}
              className={`rounded-md px-3 py-1.5 text-sm font-medium text-white shadow-sm disabled:opacity-60 ${
                decision === "DECLINED" || decision === "ESCALATED"
                  ? "bg-danger hover:bg-red-700"
                  : "bg-primary hover:bg-primary-hover"
              }`}
            >
              {submitting ? <LoadingSpinner label="Сохраняем…" /> : "Подтвердить"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
