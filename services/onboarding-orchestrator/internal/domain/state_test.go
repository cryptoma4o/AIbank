package domain

import "testing"

// TestStateMachine_ValidTransitions проверяет ключевые happy-path
// переходы из docs/domain-model.md § 2.2.
func TestStateMachine_ValidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		from, to ApplicationState
	}{
		{StateDraft, StateIdentifying},
		{StateIdentifying, StateCollectingDocuments},
		{StateCollectingDocuments, StateValidating},
		{StateValidating, StateRiskAssessing},
		{StateValidating, StateWaitingForClient},
		{StateWaitingForClient, StateValidating},
		{StateRiskAssessing, StateAutoApproved},
		{StateRiskAssessing, StateManualReview},
		{StateRiskAssessing, StateDeclined},
		{StateAutoApproved, StateApproved},
		{StateManualReview, StateApproved},
		{StateManualReview, StateApprovedWithEDD},
		{StateManualReview, StateDeclined},
		{StateApproved, StateOpeningAccount},
		{StateApprovedWithEDD, StateOpeningAccount},
		{StateOpeningAccount, StateAccountOpened},
		{StateOpeningAccount, StateApproved}, // компенсация SAGA
		{StateRequiresMoreInfo, StateWaitingForClient},
		{StateDraft, StateAbandoned},
	}

	for _, c := range cases {
		c := c
		t.Run(string(c.from)+"→"+string(c.to), func(t *testing.T) {
			t.Parallel()
			if !CanTransition(c.from, c.to) {
				t.Fatalf("expected transition %s→%s to be valid", c.from, c.to)
			}
		})
	}
}

// TestStateMachine_InvalidTransitions фиксирует, что недопустимые
// переходы (включая откаты) запрещены.
func TestStateMachine_InvalidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		from, to ApplicationState
	}{
		// Отлёт назад запрещён.
		{StateIdentifying, StateDraft},
		{StateRiskAssessing, StateValidating},
		{StateApproved, StateRiskAssessing},
		// Прыжок через несколько шагов.
		{StateDraft, StateAccountOpened},
		{StateDraft, StateApproved},
		{StateIdentifying, StateRiskAssessing},
		// Из терминальных — никуда.
		{StateAccountOpened, StateApproved},
		{StateDeclined, StateRiskAssessing},
		{StateAbandoned, StateDraft},
		// Несуществующее состояние.
		{ApplicationState("garbage"), StateDraft},
	}
	for _, c := range cases {
		c := c
		t.Run(string(c.from)+"→"+string(c.to), func(t *testing.T) {
			t.Parallel()
			if CanTransition(c.from, c.to) {
				t.Fatalf("expected transition %s→%s to be invalid", c.from, c.to)
			}
		})
	}
}

func TestStateMachine_IsTerminal(t *testing.T) {
	t.Parallel()
	terminal := []ApplicationState{StateAccountOpened, StateDeclined, StateAbandoned}
	nonTerminal := []ApplicationState{
		StateDraft, StateIdentifying, StateCollectingDocuments,
		StateValidating, StateRiskAssessing, StateApproved, StateOpeningAccount,
	}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("expected %s to be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("expected %s to be non-terminal", s)
		}
	}
}

func TestStateMachine_ValidTransitionsList(t *testing.T) {
	t.Parallel()
	got := ValidTransitions(StateRiskAssessing)
	want := map[ApplicationState]bool{
		StateAutoApproved:     true,
		StateManualReview:     true,
		StateRequiresMoreInfo: true,
		StateDeclined:         true,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d transitions, got %d (%v)", len(want), len(got), got)
	}
	for _, s := range got {
		if !want[s] {
			t.Errorf("unexpected transition %s", s)
		}
	}
}
