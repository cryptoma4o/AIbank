"""In-memory учёт токенов и стоимости per (tenant, model).

Не персистентен. В Фазе 1+ заменится на публикацию событий в Kafka для
billing-service (см. ADR-0010 — будущий). Сейчас обеспечивает базовый
observability и /v1/usage endpoint для дебага.
"""
from __future__ import annotations

import threading
import time
from collections import defaultdict
from dataclasses import asdict, dataclass, field


@dataclass
class UsageRow:
    tenant_id: str
    model: str
    role: str | None
    requests: int = 0
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_kopecks: int = 0
    last_updated_unix: float = field(default_factory=time.time)


class UsageTracker:
    """Thread-safe аккумулятор использования.

    Вызывается из request-хендлера после успешного ответа модели.
    """

    def __init__(self) -> None:
        self._rows: dict[tuple[str, str], UsageRow] = {}
        self._lock = threading.Lock()

    def record(
        self,
        tenant_id: str,
        model: str,
        role: str | None,
        prompt_tokens: int,
        completion_tokens: int,
        cost_per_1k_input_kop: int,
        cost_per_1k_output_kop: int,
    ) -> None:
        cost = (
            (prompt_tokens * cost_per_1k_input_kop) // 1000
            + (completion_tokens * cost_per_1k_output_kop) // 1000
        )
        key = (tenant_id, model)
        with self._lock:
            row = self._rows.get(key)
            if row is None:
                row = UsageRow(tenant_id=tenant_id, model=model, role=role)
                self._rows[key] = row
            row.requests += 1
            row.prompt_tokens += prompt_tokens
            row.completion_tokens += completion_tokens
            row.total_kopecks += cost
            row.last_updated_unix = time.time()

    def snapshot(self, tenant_id: str | None = None) -> list[dict]:
        with self._lock:
            rows = list(self._rows.values())
        if tenant_id:
            rows = [r for r in rows if r.tenant_id == tenant_id]
        rows.sort(key=lambda r: (r.tenant_id, r.model))
        return [asdict(r) for r in rows]

    def reset(self) -> None:
        """Только для тестов."""
        with self._lock:
            self._rows.clear()

    def aggregate_by_tenant(self) -> dict[str, dict]:
        """Возвращает агрегаты по тенанту: суммарные токены и стоимость в копейках."""
        agg: dict[str, dict] = defaultdict(
            lambda: {"requests": 0, "prompt_tokens": 0, "completion_tokens": 0, "total_kopecks": 0}
        )
        with self._lock:
            for r in self._rows.values():
                bucket = agg[r.tenant_id]
                bucket["requests"] += r.requests
                bucket["prompt_tokens"] += r.prompt_tokens
                bucket["completion_tokens"] += r.completion_tokens
                bucket["total_kopecks"] += r.total_kopecks
        return dict(agg)


# Module-level singleton — единственный аккумулятор на процесс.
tracker = UsageTracker()
