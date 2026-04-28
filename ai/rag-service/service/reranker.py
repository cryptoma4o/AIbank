"""Reranker — абстракция над second-pass scoring для retriever.

Сейчас в Pre-MVP retriever блендит dense-recall с Jaccard-overlap. После
получения GPU и развёртывания BGE-reranker-v2-m3 (или подобного cross-encoder)
переключение делается через ENV ``RAG_RERANKER=bge`` без изменений в
retriever.

Реализации:

* ``NoOpReranker`` — возвращает dense-score как есть (быстро, для smoke и в
  случае, если cross-encoder сломан).
* ``LexicalReranker`` — Jaccard-overlap (Pre-MVP default; то, что было
  inline в retriever до этого PR).
* ``BGEReranker`` — HTTP-клиент к TEI / vLLM с моделью ``BAAI/bge-reranker-v2-m3``
  по OpenAI-compatible или ``/rerank``-endpoint'у.

Контракт:
    rerank(query, candidates) → list[float] (по одному score на candidate,
    в том же порядке, что входные candidates)

Производственные replace'ы (после получения GPU):

    RAG_RERANKER=bge
    RAG_RERANKER_URL=http://bge-reranker.cluster.local:8080
    RAG_RERANKER_MODEL=BAAI/bge-reranker-v2-m3

Связанные документы:
    - ADR-0008 (vector DB / sparse / reranker stack)
    - docs/pilot-readiness.md § 1.3 (ML/AI блокеры)
"""
from __future__ import annotations

import logging
import os
import re
from abc import ABC, abstractmethod
from typing import Iterable

import httpx

log = logging.getLogger(__name__)


_TOKEN_RE = re.compile(r"[\wа-яёА-ЯЁ]+", re.UNICODE)


def _tokenize(text: str) -> set[str]:
    return {m.group(0).lower() for m in _TOKEN_RE.finditer(text)}


class Reranker(ABC):
    """Reranker возвращает per-candidate score'ы (0..1, чем больше — релевантнее)."""

    name: str = "reranker"

    @abstractmethod
    async def rerank(self, query: str, candidates: Iterable[str]) -> list[float]:
        ...


class NoOpReranker(Reranker):
    """Возвращает 0.0 для всех — retriever полагается только на dense-score.

    Используется когда cross-encoder временно недоступен / при smoke-tests.
    """

    name = "noop"

    async def rerank(self, query: str, candidates: Iterable[str]) -> list[float]:
        return [0.0 for _ in candidates]


class LexicalReranker(Reranker):
    """Jaccard-style overlap (Pre-MVP default, был inline в retriever).

    Не требует никакой инфраструктуры — работает без сети и моделей.
    Подходит для CI и dev-локалки. После подключения BGE-reranker заменяется
    через RAG_RERANKER=bge.
    """

    name = "lexical"

    async def rerank(self, query: str, candidates: Iterable[str]) -> list[float]:
        q = _tokenize(query)
        if not q:
            return [0.0 for _ in candidates]
        return [self._score(q, c) for c in candidates]

    @staticmethod
    def _score(q_tokens: set[str], text: str) -> float:
        t = _tokenize(text)
        if not t:
            return 0.0
        return len(q_tokens & t) / len(q_tokens)


class BGEReranker(Reranker):
    """HTTP-клиент к TEI или vLLM с моделью ``BAAI/bge-reranker-v2-m3``.

    Предполагается endpoint, совместимый с TEI ``/rerank`` или
    sentence-transformers HTTP wrapper. Параметры:

    * ``url``  — базовый URL (например ``http://bge-reranker:8080``).
    * ``model`` — имя модели (передаётся в payload, если backend требует).
    * ``timeout_s`` — общий timeout для запроса (по умолчанию 30s).

    На сетевые ошибки fallback'а нет — retriever ловит исключение и
    обрабатывает (типичный путь: degrade на dense-only score).
    """

    name = "bge"

    def __init__(
        self,
        url: str,
        model: str = "BAAI/bge-reranker-v2-m3",
        timeout_s: float = 30.0,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        if not url:
            raise ValueError("BGEReranker: url is required")
        self.url = url.rstrip("/")
        self.model = model
        self.timeout_s = timeout_s
        self._client = client  # тестам можно передать stub

    async def rerank(self, query: str, candidates: Iterable[str]) -> list[float]:
        cands = list(candidates)
        if not cands:
            return []

        # Используем TEI ``/rerank`` shape: {"query": "...", "texts": [...]}.
        # Большинство bge-reranker-deploy'ев поддерживают этот формат.
        payload = {"query": query, "texts": cands, "model": self.model}

        client = self._client or httpx.AsyncClient(timeout=self.timeout_s)
        try:
            resp = await client.post(f"{self.url}/rerank", json=payload)
            resp.raise_for_status()
            data = resp.json()
        finally:
            if self._client is None:
                await client.aclose()

        # Поддерживаемые shape'ы ответа:
        #   1) [{"index": 0, "score": 0.93}, ...]    — TEI
        #   2) {"scores": [0.93, ...]}                — кастомные wrapper'ы
        scores: list[float] = [0.0] * len(cands)
        if isinstance(data, list):
            for item in data:
                idx = int(item.get("index", -1))
                if 0 <= idx < len(cands):
                    scores[idx] = float(item.get("score", 0.0))
        elif isinstance(data, dict) and "scores" in data:
            raw = data["scores"]
            if isinstance(raw, list) and len(raw) == len(cands):
                scores = [float(x) for x in raw]
        else:
            log.warning("BGEReranker: неизвестная shape ответа: %r", type(data))

        # Нормализуем в [0, 1] если backend отдаёт логиты (heuristic: >1 или <0).
        if scores and (max(scores) > 1.0 or min(scores) < 0.0):
            scores = _sigmoid_normalize(scores)
        return scores


def _sigmoid_normalize(xs: list[float]) -> list[float]:
    """Простой sigmoid для приведения логитов к [0, 1]."""
    import math

    return [1.0 / (1.0 + math.exp(-x)) for x in xs]


def build_reranker() -> Reranker:
    """Factory из ENV.

    * ``RAG_RERANKER=lexical`` (default Pre-MVP) → LexicalReranker
    * ``RAG_RERANKER=bge``                       → BGEReranker, требует RAG_RERANKER_URL
    * ``RAG_RERANKER=noop``                      → NoOpReranker (только dense-score)
    """
    kind = (os.environ.get("RAG_RERANKER") or "lexical").lower()
    if kind == "noop":
        return NoOpReranker()
    if kind == "bge":
        url = os.environ.get("RAG_RERANKER_URL")
        if not url:
            log.warning(
                "RAG_RERANKER=bge но RAG_RERANKER_URL не задан — fallback на lexical"
            )
            return LexicalReranker()
        model = os.environ.get("RAG_RERANKER_MODEL", "BAAI/bge-reranker-v2-m3")
        timeout_s = float(os.environ.get("RAG_RERANKER_TIMEOUT_S", "30"))
        log.info("BGE-reranker enabled: url=%s model=%s", url, model)
        return BGEReranker(url=url, model=model, timeout_s=timeout_s)
    return LexicalReranker()
