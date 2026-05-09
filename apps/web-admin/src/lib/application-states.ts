// application-states.ts — список состояний заявки и их человеческих
// названий.  Зеркалит ApplicationState из bff-admin/graph/schema.graphqls
// и onboarding-orchestrator/internal/domain/state.go.  Идентичен файлу
// в web-onboarding — намеренно дублируем (а не выносим в shared package),
// чтобы skeleton-приложения оставались независимыми.

export const APPLICATION_STATES = [
  "draft",
  "identifying",
  "collecting_documents",
  "validating",
  "waiting_for_client",
  "risk_assessing",
  "auto_approved",
  "manual_review",
  "approved",
  "approved_with_edd",
  "requires_more_info",
  "opening_account",
  "account_opened",
  "declined",
  "abandoned",
] as const;

export type ApplicationState = (typeof APPLICATION_STATES)[number];

const STATE_LABELS: Record<ApplicationState, string> = {
  draft: "Черновик",
  identifying: "Идентификация",
  collecting_documents: "Сбор документов",
  validating: "Проверка документов",
  waiting_for_client: "Ожидание клиента",
  risk_assessing: "Оценка риска",
  auto_approved: "Авто-одобрено",
  manual_review: "Ручная проверка",
  approved: "Одобрено",
  approved_with_edd: "Одобрено с EDD",
  requires_more_info: "Нужны уточнения",
  opening_account: "Открытие счёта",
  account_opened: "Счёт открыт",
  declined: "Отказано",
  abandoned: "Отменено",
};

export function stateLabel(state: string): string {
  if ((APPLICATION_STATES as readonly string[]).includes(state)) {
    return STATE_LABELS[state as ApplicationState];
  }
  return state;
}

export type StateTone = "neutral" | "info" | "success" | "warning" | "danger";

const STATE_TONES: Record<ApplicationState, StateTone> = {
  draft: "neutral",
  identifying: "info",
  collecting_documents: "info",
  validating: "info",
  waiting_for_client: "warning",
  risk_assessing: "info",
  auto_approved: "success",
  manual_review: "warning",
  approved: "success",
  approved_with_edd: "success",
  requires_more_info: "warning",
  opening_account: "info",
  account_opened: "success",
  declined: "danger",
  abandoned: "neutral",
};

export function stateTone(state: string): StateTone {
  if ((APPLICATION_STATES as readonly string[]).includes(state)) {
    return STATE_TONES[state as ApplicationState];
  }
  return "neutral";
}

const TONE_CLASSES: Record<StateTone, string> = {
  neutral: "bg-gray-100 text-gray-700 border-gray-200",
  info: "bg-primary-50 text-primary border-primary-100",
  success: "bg-green-50 text-success border-green-200",
  warning: "bg-amber-50 text-warning border-amber-200",
  danger: "bg-red-50 text-danger border-red-200",
};

export function toneClasses(tone: StateTone): string {
  return TONE_CLASSES[tone];
}

/** Состояния, в которых офицер может принять ручное решение. */
export const DECISION_STATES: ApplicationState[] = [
  "manual_review",
  "requires_more_info",
];

export function isDecisionState(state: string): boolean {
  return (DECISION_STATES as readonly string[]).includes(state);
}

/**
 * TRANSITIONS_BY_STATE — карта допустимых переходов между состояниями.
 *
 * Источник истины — onboarding-orchestrator/internal/domain/state.go,
 * map validTransitions.  Здесь дублируется только для рендеринга UI:
 * показать список доступных кнопок.  При попытке выполнить недопустимый
 * переход backend всё равно вернёт `invalid_transition` — фронт это
 * обработает как ошибку.
 *
 * Терминальные состояния (account_opened, declined, abandoned) — пустой
 * список.
 */
export const TRANSITIONS_BY_STATE: Record<ApplicationState, ApplicationState[]> = {
  draft: ["identifying", "abandoned"],
  identifying: ["collecting_documents", "declined", "abandoned"],
  collecting_documents: ["validating", "abandoned"],
  validating: ["waiting_for_client", "risk_assessing", "declined"],
  waiting_for_client: ["validating", "abandoned"],
  risk_assessing: [
    "auto_approved",
    "manual_review",
    "requires_more_info",
    "declined",
  ],
  auto_approved: ["approved"],
  manual_review: ["approved", "approved_with_edd", "declined"],
  requires_more_info: ["waiting_for_client", "declined"],
  approved: ["opening_account"],
  approved_with_edd: ["opening_account"],
  opening_account: ["account_opened", "approved"],
  account_opened: [],
  declined: [],
  abandoned: [],
};

/** Терминальные состояния — переходы запрещены. */
export const TERMINAL_STATES: ApplicationState[] = [
  "account_opened",
  "declined",
  "abandoned",
];

export function isTerminalState(state: string): boolean {
  return (TERMINAL_STATES as readonly string[]).includes(state);
}

/**
 * Возвращает список допустимых переходов для текущего состояния.
 * Если состояние не известно — пустой массив.
 */
export function transitionsFor(state: string): ApplicationState[] {
  if ((APPLICATION_STATES as readonly string[]).includes(state)) {
    return TRANSITIONS_BY_STATE[state as ApplicationState];
  }
  return [];
}
