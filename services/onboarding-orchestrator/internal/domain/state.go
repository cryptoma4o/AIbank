package domain

// ApplicationState — финитный автомат заявки.
//
// Источник истины: docs/domain-model.md § 2.2 (Application State Machine).
// 11 состояний; терминальные — account_opened, declined, abandoned.
// Откат недопустим (переход назад = создание нового Application).
type ApplicationState string

const (
	StateDraft               ApplicationState = "draft"
	StateIdentifying         ApplicationState = "identifying"
	StateCollectingDocuments ApplicationState = "collecting_documents"
	StateValidating          ApplicationState = "validating"
	StateWaitingForClient    ApplicationState = "waiting_for_client"
	StateRiskAssessing       ApplicationState = "risk_assessing"
	StateAutoApproved        ApplicationState = "auto_approved"
	StateManualReview        ApplicationState = "manual_review"
	StateApproved            ApplicationState = "approved"
	StateApprovedWithEDD     ApplicationState = "approved_with_edd"
	StateRequiresMoreInfo    ApplicationState = "requires_more_info"
	StateOpeningAccount      ApplicationState = "opening_account"
	StateAccountOpened       ApplicationState = "account_opened"
	StateDeclined            ApplicationState = "declined"
	StateAbandoned           ApplicationState = "abandoned"
)

// IsValid возвращает true, если значение принадлежит словарю состояний.
func (s ApplicationState) IsValid() bool {
	_, ok := validTransitions[s]
	if ok {
		return true
	}
	// Терминальные состояния присутствуют как ключ с пустым списком переходов.
	switch s {
	case StateAccountOpened, StateDeclined, StateAbandoned:
		return true
	}
	return false
}

// IsTerminal возвращает true для финальных состояний (account_opened,
// declined, abandoned). Из терминала переходы запрещены.
func (s ApplicationState) IsTerminal() bool {
	switch s {
	case StateAccountOpened, StateDeclined, StateAbandoned:
		return true
	}
	return false
}

// validTransitions описывает разрешённые переходы между состояниями.
//
// Любой переход, не указанный здесь — ErrInvalidTransition. Терминальные
// состояния отсутствуют как ключ (или присутствуют с пустым slice — см. ниже).
var validTransitions = map[ApplicationState][]ApplicationState{
	StateDraft: {
		StateIdentifying,
		StateAbandoned,
	},
	StateIdentifying: {
		StateCollectingDocuments,
		StateDeclined,
		StateAbandoned,
	},
	StateCollectingDocuments: {
		StateValidating,
		StateAbandoned,
	},
	StateValidating: {
		StateWaitingForClient,
		StateRiskAssessing,
		StateDeclined,
	},
	StateWaitingForClient: {
		StateValidating,
		StateAbandoned,
	},
	StateRiskAssessing: {
		StateAutoApproved,
		StateManualReview,
		StateRequiresMoreInfo,
		StateDeclined,
	},
	StateAutoApproved: {
		StateApproved,
	},
	StateManualReview: {
		StateApproved,
		StateApprovedWithEDD,
		StateDeclined,
	},
	StateRequiresMoreInfo: {
		StateWaitingForClient,
		StateDeclined,
	},
	StateApproved: {
		StateOpeningAccount,
	},
	StateApprovedWithEDD: {
		StateOpeningAccount,
	},
	StateOpeningAccount: {
		StateAccountOpened,
		StateApproved, // компенсация: ABS не открыл — откатываем в approved
	},
	// Терминальные — нет исходящих, но регистрируем как known states.
	StateAccountOpened: {},
	StateDeclined:      {},
	StateAbandoned:     {},
}

// ValidTransitions возвращает копию списка разрешённых переходов из
// заданного состояния. Возвращаемый slice безопасен для модификации
// вызывающим кодом.
func ValidTransitions(from ApplicationState) []ApplicationState {
	src, ok := validTransitions[from]
	if !ok {
		return nil
	}
	out := make([]ApplicationState, len(src))
	copy(out, src)
	return out
}

// CanTransition возвращает true, если переход from→to разрешён state machine.
func CanTransition(from, to ApplicationState) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
