"use client";

// WizardProgress — индикатор прохождения 10-этапной формы онбординга.
//
// Сейчас фронт реализует этапы 1-4 (см. docs/onboarding-form-spec.md).
// Этапы 5-10 показаны как "запланировано" и заблокированы.  Сами шаги
// рендерятся как горизонтальная лента с тремя состояниями: completed,
// current, locked.  Активный шаг помечен ARIA-атрибутом aria-current.

import Link from "next/link";

export interface WizardStep {
  id: number;
  title: string;
  shortTitle: string;
  href?: string | null;
}

export const WIZARD_STEPS: WizardStep[] = [
  { id: 1, title: "Этап 1 · Предварительная проверка", shortTitle: "Скоринг" },
  { id: 2, title: "Этап 2 · Параметры заявки", shortTitle: "Параметры" },
  { id: 3, title: "Этап 3 · Сведения о деятельности (AML)", shortTitle: "Деятельность" },
  { id: 4, title: "Этап 4 · ЕИО и представители", shortTitle: "Представители" },
];

interface Props {
  currentStep: number;
  applicationId?: string | null;
}

export function WizardProgress({ currentStep, applicationId }: Props) {
  return (
    <nav aria-label="Прохождение онбординга" data-testid="wizard-progress">
      <ol className="flex flex-wrap items-center gap-2 text-xs">
        {WIZARD_STEPS.map((step, idx) => {
          const completed = step.id < currentStep;
          const current = step.id === currentStep;
          const canNavigate = applicationId && (completed || current) && step.id >= 3;
          const href = canNavigate
            ? `/applications/${applicationId}/wizard/${step.id}`
            : null;

          const dotClass = current
            ? "border-primary bg-primary text-white"
            : completed
              ? "border-primary bg-white text-primary"
              : "border-gray-300 bg-white text-gray-400";

          const labelClass = current
            ? "font-semibold text-gray-900"
            : completed
              ? "text-gray-700"
              : "text-gray-400";

          const dot = (
            <span
              className={`inline-flex h-6 w-6 items-center justify-center rounded-full border text-[11px] font-semibold ${dotClass}`}
              aria-hidden="true"
            >
              {step.id}
            </span>
          );

          return (
            <li key={step.id} className="flex items-center gap-2">
              {href ? (
                <Link
                  href={href}
                  className="flex items-center gap-2 hover:text-primary"
                  aria-current={current ? "step" : undefined}
                >
                  {dot}
                  <span className={labelClass}>{step.shortTitle}</span>
                </Link>
              ) : (
                <span
                  className="flex items-center gap-2"
                  aria-current={current ? "step" : undefined}
                >
                  {dot}
                  <span className={labelClass}>{step.shortTitle}</span>
                </span>
              )}
              {idx < WIZARD_STEPS.length - 1 ? (
                <span className="text-gray-300" aria-hidden="true">
                  →
                </span>
              ) : null}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
