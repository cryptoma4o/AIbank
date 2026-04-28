"""Pytest config + reusable FakeGateway for agent-document-intake."""
from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


class FakeGateway:
    """Stand-in for GatewayClient.chat — returns canned content."""

    def __init__(
        self,
        content: str | dict[str, Any] | None = None,
        raise_exc: Exception | None = None,
    ) -> None:
        self.content = content
        self.raise_exc = raise_exc
        self.calls: list[dict[str, Any]] = []

    async def chat(
        self,
        role: str,
        messages: list[dict[str, str]],
        tenant_id: str,
        max_tokens: int = 1024,
        temperature: float = 0.0,
    ) -> dict[str, Any]:
        self.calls.append(
            {"role": role, "messages": messages, "tenant_id": tenant_id}
        )
        if self.raise_exc is not None:
            raise self.raise_exc
        if isinstance(self.content, str):
            text = self.content
        elif self.content is None:
            text = "{}"
        else:
            text = json.dumps(self.content, ensure_ascii=False)
        return {
            "choices": [{"message": {"content": text}}],
            "aibank_gateway": {
                "role": role,
                "resolved_model": "mock-fast",
                "backend": "mock",
                "used_fallback": False,
                "tenant_id": tenant_id,
                "latency_ms": 1.0,
            },
        }
