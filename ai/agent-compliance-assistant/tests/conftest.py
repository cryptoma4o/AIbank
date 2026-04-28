"""Pytest config + reusable fakes for agent-compliance-assistant."""
from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


class FakeGateway:
    """Stand-in for GatewayClient — returns a canned chat-completion response."""

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
        temperature: float = 0.1,
    ) -> dict[str, Any]:
        self.calls.append(
            {"role": role, "messages": messages, "tenant_id": tenant_id}
        )
        if self.raise_exc is not None:
            raise self.raise_exc
        if isinstance(self.content, str):
            text = self.content
        elif self.content is None:
            text = "Ответ недоступен."
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


class FakeRAGClient:
    """Stand-in for RAGClient — returns canned hits."""

    def __init__(
        self,
        hits: list[dict[str, Any]] | None = None,
        raise_exc: Exception | None = None,
    ) -> None:
        self.hits = hits or []
        self.raise_exc = raise_exc
        self.calls: list[dict[str, Any]] = []

    async def search(
        self,
        tenant_id: str,
        query: str,
        top_k: int = 5,
        source_type: str | None = None,
    ) -> list[dict[str, Any]]:
        self.calls.append(
            {
                "tenant_id": tenant_id,
                "query": query,
                "top_k": top_k,
                "source_type": source_type,
            }
        )
        if self.raise_exc is not None:
            raise self.raise_exc
        return list(self.hits)


SAMPLE_HIT_115FZ_ART7 = {
    "document_id": "115-fz-art7",
    "score": 0.92,
    "snippet": (
        "Статья 7 115-ФЗ обязывает кредитные организации идентифицировать клиента "
        "и его представителя, а также бенефициарных владельцев."
    ),
    "source": "115-ФЗ ст.7",
    "source_type": "law",
    "metadata": {},
}

SAMPLE_HIT_375P = {
    "document_id": "375-p",
    "score": 0.78,
    "snippet": (
        "Положение 375-П Банка России определяет факторы повышенного риска "
        "проведения операций отмывания доходов."
    ),
    "source": "375-П",
    "source_type": "regulation",
    "metadata": {},
}
