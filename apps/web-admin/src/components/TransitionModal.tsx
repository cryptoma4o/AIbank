"use client";

// TransitionModal — ручной перевод заявки в другое состояние state machine.
//
// Открывается из /applications/[id]/page.tsx, если состояние не терминальное.
// Показывает радио-список допустимых переходов из текущего state (источник —
// TRANSITIONS_BY_STATE; финальная валидация на backend через
// `transitionApplicationState`).  Reason — обязательное поле.
//
// Обработка ошибок:
//   - graphQLErrors[].extensions.code === "invalid_transition" →
//     «Переход запрещён state machine».
//   - networkError / прочее → generic-сообщение через ErrorBanner.

import { useEffect, useMemo, useRef, useState } from "react";
import type { ApolloError } from "@apollo/client";

import { ErrorBanner } from "@/components/ErrorBanner";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import {
  stateLabel,
  transitionsFor,
  type ApplicationState,
} from "@/lib/application-states";

const REASON_MIN_CHARS = 10;

interface Props {
  open: boolean;
  applicationId: string;
  currentState: string;
  onClose: () => void;
  onSubmit: (input: {
    applicationId: string;
    newState: ApplicationState;
    reason: string;
  }) => Promise<void>;
}

/**
 * Извлекает человеческое сообщение из ошибки мутации, специально обрабатывая
 * extensions.code === "invalid_transition".
 */
function describeError(err: unknown): string {
  if (!err) return "";
  const apollo = err as ApolloError;
  if (apollo.graphQLErrors?.length) {
    for (const ge of apollo.graphQLErrors) {
      const code = (ge.extensions as { code?: string } | undefined)?.code;
      if (code === "invalid_transition") {
        return "Переход запрещён state machine. Проверьте текущее состояние заявки.";
      }
      // Backend в текущей реализации может возвращать ошибку без extensions;
      // ловим её по сообщению.
      if (/invalid[_ ]transition/i.test(ge.message)) {
        return "Переход запрещён state machine. Проверьте текущее состояние заявки.";
      }
    }
    return apollo.graphQLErrors.map((e) => e.message).join("; ");
  }
  if (apollo.networkError) {
    return `Ошибка сети: ${apollo.networkError.message}`;
  }
  if (err instanceof Error) {
    if (/invalid[_ ]transition/i.test(err.message)) {
      return "Переход запрещён state machine. Проверьте текущее состояние заявки.";
    }
    return err.message;
  }
  return "Не удалось перевести заявку";
}

export function TransitionModal({
  open,
  applicationId,
  currentState,
  onClose,
  onSubmit,
}: Props) {
  const transitions = useMemo(
    () => transitionsFor(currentState),
    [currentState]
  );

  const [target, setTarget] = useState<ApplicationState | null>(null);
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const dialogRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (open) {
      setTarget(transitions[0] ?? null);
      setReason("");
      setError(null);
      setSubmitting(false);
      setTimeout(() => {
        const ta = dialogRef.current?.querySelector("textarea");
        ta?.focus();
      }, 50);
    }
  }, [open, transitions]);

  if (!open) return null;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!target) {
      setError("Выберите целевое состояние.");
      return;
    }
    const trimmed = reason.trim();
    if (trimmed.length < REASON_MIN_CHARS) {
      setError(
        `Причина перевода обязательна и должна содержать не менее ${REASON_MIN_CHARS} символов.`
      );
      return;
    }
    setSubmitting(true);
    try {
      await onSubmit({ applicationId, newState: target, reason: trimmed });
      onClose();
    } catch (err) {
      setError(describeError(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="transition-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !submitting) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="w-full max-w-lg rounded-lg border border-gray-200 bg-white p-6 shadow-xl"
      >
        <h2
          id="transition-dialog-title"
          className="text-base font-semibold text-gray-900"
        >
          Перевести заявку в другое состояние
        </h2>
        <p className="mt-1 text-sm text-gray-600">
          Текущее состояние:{" "}
          <span className="font-medium text-gray-900">
            {stateLabel(currentState)}
          </span>
          . Все переходы фиксируются в audit log.
        </p>

        <form className="mt-4 space-y-4" onSubmit={handleSubmit} noValidate>
          {error ? <ErrorBanner error={error} /> : null}

          {transitions.length === 0 ? (
            <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
              Из текущего состояния нет разрешённых переходов.
            </div>
          ) : (
            <fieldset>
              <legend className="mb-2 block text-sm font-medium text-gray-700">
                Целевое состояние
              </legend>
              <div className="space-y-1">
                {transitions.map((s) => (
                  <label
                    key={s}
                    className="flex cursor-pointer items-center gap-2 rounded-md border border-gray-200 bg-white px-3 py-2 text-sm hover:bg-gray-50"
                  >
                    <input
                      type="radio"
                      name="transition-target"
                      value={s}
                      checked={target === s}
                      onChange={() => setTarget(s)}
                      disabled={submitting}
                      className="text-primary"
                    />
                    <span className="text-gray-800">{stateLabel(s)}</span>
                    <span className="ml-auto font-mono text-xs text-gray-400">
                      {s}
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
          )}

          <div>
            <label
              htmlFor="transition-reason"
              className="mb-1 block text-sm font-medium text-gray-700"
            >
              Причина перевода (≥ {REASON_MIN_CHARS} символов)
            </label>
            <textarea
              id="transition-reason"
              rows={4}
              className="w-full resize-vertical rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-primary"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              disabled={submitting}
              required
              minLength={REASON_MIN_CHARS}
            />
            <p className="mt-1 text-xs text-gray-500">
              {reason.trim().length}/{REASON_MIN_CHARS}+
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
              disabled={submitting || transitions.length === 0 || !target}
              className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-primary-hover disabled:opacity-60"
            >
              {submitting ? <LoadingSpinner label="Переводим…" /> : "Перевести"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
