"""End-to-end onboarding scenarios for the eval-harness.

Each scenario walks an applicant through the orchestrator state machine
(see services/onboarding-orchestrator/internal/domain/state.go) using
respx-stubbed HTTP backends — no real services required.
"""
from .happy_path import HAPPY_PATH_SCENARIO

__all__ = ["HAPPY_PATH_SCENARIO"]
