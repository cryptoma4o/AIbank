"""Embedder interface, deterministic mock and BGE-M3 HTTP client.

Контракт `Embedder` сделан минимальным специально, чтобы заменить MVP-мок
на реальную модель (планируется bge-m3, см. ADR-0008) без изменений ни у
агентов, ни у retriever-слоя:

    class Embedder(Protocol):
        dim: int
        async def embed(self, texts: list[str]) -> list[list[float]]: ...

`MockEmbedder` отдаёт детерминированные 384-мерные векторы, нормированные
на единичную длину — этого достаточно, чтобы тестировать retriever
(косинус = скалярное произведение) без HuggingFace-зависимостей.

`BGEM3Embedder` — тонкий HTTP-клиент к sidecar-сервису
[Text Embeddings Inference (TEI)](https://huggingface.github.io/text-embeddings-inference/),
поднятому рядом с rag-service на GPU-ноде с моделью `BAAI/bge-m3` (1024
измерения). Сервис принимает `POST /embed` с телом `{"inputs": [...]}` и
возвращает `[[float, ...], ...]`. См. `configs/sample-prod.yaml` и ADR-0008.

Намеренно НЕ тащим `sentence-transformers`/`torch` в зависимости —
это десятки мегабайт wheel'ов и тяжёлый старт. BGE-M3 живёт в TEI-сервере,
rag-service общается с ним только через HTTP.
"""
from __future__ import annotations

import asyncio
import hashlib
import logging
import math
import os
from typing import Protocol

import httpx


log = logging.getLogger(__name__)


# Размер дефолтных мок-векторов; настоящий BGE-M3 — 1024.
EMBED_DIM = 384

# Дефолты BGE-M3 / TEI.
DEFAULT_TEI_URL = "http://tei:8080"
DEFAULT_BGE_DIM = 1024
DEFAULT_TEI_TIMEOUT = 30.0
DEFAULT_TEI_BATCH_SIZE = 32
DEFAULT_TEI_MAX_RETRIES = 2


class Embedder(Protocol):
    """Async embedding contract used by the retriever."""

    dim: int

    async def embed(self, texts: list[str]) -> list[list[float]]:
        ...


# ---------------------------------------------------------------------------
# Mock embedder (CI / dev / tests)
# ---------------------------------------------------------------------------


def _hash_to_floats(text: str, dim: int) -> list[float]:
    """Map text → fixed-size deterministic float vector via repeated SHA-256."""
    out: list[float] = []
    counter = 0
    seed = text.encode("utf-8")
    while len(out) < dim:
        h = hashlib.sha256(seed + counter.to_bytes(4, "big")).digest()
        # 32 bytes → 16 floats (signed int16 → range -1..1)
        for i in range(0, len(h), 2):
            if len(out) >= dim:
                break
            val = int.from_bytes(h[i : i + 2], "big", signed=True) / 32768.0
            out.append(val)
        counter += 1
    return out


def _normalize(vec: list[float]) -> list[float]:
    norm = math.sqrt(sum(v * v for v in vec))
    if norm == 0.0:
        return vec
    return [v / norm for v in vec]


class MockEmbedder:
    """Deterministic, dependency-free embedder used in MVP and tests.

    Каждое слово вкладывается отдельно (lower-cased) и усредняется — это
    создаёт нелокальную зависимость от лексики, поэтому совпадение слова
    в запросе и документе уже даёт ненулевое сходство (а не «всё или
    ничего» при попытке хешировать всю строку целиком).
    """

    dim: int = EMBED_DIM

    def __init__(self, dim: int = EMBED_DIM) -> None:
        self.dim = dim

    async def embed(self, texts: list[str]) -> list[list[float]]:
        return [self._embed_one(t) for t in texts]

    def _embed_one(self, text: str) -> list[float]:
        tokens = [t for t in text.lower().split() if t]
        if not tokens:
            return [0.0] * self.dim
        sums = [0.0] * self.dim
        for tok in tokens:
            v = _hash_to_floats(tok, self.dim)
            for i, x in enumerate(v):
                sums[i] += x
        avg = [s / len(tokens) for s in sums]
        return _normalize(avg)


# ---------------------------------------------------------------------------
# BGE-M3 embedder (TEI HTTP sidecar)
# ---------------------------------------------------------------------------


class BGEM3Embedder:
    """HTTP-клиент к Text Embeddings Inference (TEI) с моделью BAAI/bge-m3.

    Контракт TEI:
        POST {base_url}/embed
        Request:  {"inputs": ["text1", "text2", ...]}
        Response: [[float, ...], [float, ...], ...]   # len(response) == len(inputs)

    Возвращаемые векторы L2-нормируются (если вдруг сервер их не отдал
    нормированными) — это важно, чтобы dot-product в retriever-е оставался
    эквивалентом косинуса (как и MockEmbedder делает).

    Реализованы:
    - retry на httpx.HTTPError / 5xx (`max_retries`, экспоненциальный backoff);
    - авто-батчинг входа > `batch_size` (TEI имеет лимит на длину запроса);
    - валидация размерности — если сервер отдал не `dim`, бросаем явную ошибку.
    """

    def __init__(
        self,
        base_url: str = DEFAULT_TEI_URL,
        dim: int = DEFAULT_BGE_DIM,
        timeout: float = DEFAULT_TEI_TIMEOUT,
        batch_size: int = DEFAULT_TEI_BATCH_SIZE,
        max_retries: int = DEFAULT_TEI_MAX_RETRIES,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.dim = dim
        self.timeout = timeout
        self.batch_size = max(1, batch_size)
        self.max_retries = max(0, max_retries)
        self._client = client  # инжектируется в тестах; иначе создаём per-call

    async def embed(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            return []

        # Авто-батчинг: TEI принимает массив, но мы ограничиваем размер
        # одного запроса, чтобы не упереться в payload limit / OOM на GPU.
        out: list[list[float]] = []
        for i in range(0, len(texts), self.batch_size):
            batch = texts[i : i + self.batch_size]
            vectors = await self._embed_batch(batch)
            out.extend(vectors)
        return out

    async def _embed_batch(self, batch: list[str]) -> list[list[float]]:
        url = f"{self.base_url}/embed"
        payload = {"inputs": batch}

        # Retry-цикл: до max_retries+1 попыток (1 базовая + max_retries повторов).
        last_exc: Exception | None = None
        for attempt in range(self.max_retries + 1):
            try:
                data = await self._post(url, payload)
                return self._validate_and_normalize(data, expected_n=len(batch))
            except httpx.HTTPError as exc:
                last_exc = exc
                if attempt >= self.max_retries:
                    log.error(
                        "TEI embed failed after %d attempts: %s",
                        attempt + 1,
                        exc,
                    )
                    raise
                # Экспоненциальный backoff: 0.2с, 0.4с, 0.8с, ...
                backoff = 0.2 * (2**attempt)
                log.warning(
                    "TEI embed attempt %d/%d failed (%s) — retry через %.1fс",
                    attempt + 1,
                    self.max_retries + 1,
                    exc,
                    backoff,
                )
                await asyncio.sleep(backoff)

        # На всякий случай — теоретически недостижимо.
        assert last_exc is not None
        raise last_exc

    async def _post(self, url: str, payload: dict) -> list[list[float]]:
        if self._client is not None:
            response = await self._client.post(url, json=payload, timeout=self.timeout)
        else:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                response = await client.post(url, json=payload)

        # raise_for_status кидает HTTPStatusError (subclass HTTPError) на 4xx/5xx.
        response.raise_for_status()
        data = response.json()
        if not isinstance(data, list):
            raise httpx.HTTPError(
                f"TEI вернул неожиданный формат (ожидался list): {type(data).__name__}"
            )
        return data

    def _validate_and_normalize(
        self, data: list[list[float]], expected_n: int
    ) -> list[list[float]]:
        if len(data) != expected_n:
            raise httpx.HTTPError(
                f"TEI вернул {len(data)} векторов, ожидалось {expected_n}"
            )
        out: list[list[float]] = []
        for i, vec in enumerate(data):
            if not isinstance(vec, list):
                raise httpx.HTTPError(
                    f"TEI: вектор #{i} не list, а {type(vec).__name__}"
                )
            if len(vec) != self.dim:
                raise httpx.HTTPError(
                    f"TEI: вектор #{i} имеет dim={len(vec)}, ожидалось {self.dim}"
                )
            out.append(_normalize([float(x) for x in vec]))
        return out


# ---------------------------------------------------------------------------
# Factory
# ---------------------------------------------------------------------------


def build_embedder(kind: str | None = None) -> Embedder:
    """Фабрика выбора embedder-а.

    Args:
        kind: "mock" | "tei". Если None — берём из env `RAG_EMBEDDER`
            (по умолчанию "mock"). Любое другое значение — `ValueError`.

    Production-конфиг (см. `configs/sample-prod.yaml`):
        RAG_EMBEDDER=tei
        RAG_TEI_URL=http://tei:8080
        RAG_EMBED_DIM=1024
    """
    resolved = (kind or os.environ.get("RAG_EMBEDDER", "mock")).strip().lower()

    if resolved == "mock":
        return MockEmbedder()

    if resolved == "tei":
        base_url = os.environ.get("RAG_TEI_URL", DEFAULT_TEI_URL)
        try:
            dim = int(os.environ.get("RAG_EMBED_DIM", str(DEFAULT_BGE_DIM)))
        except ValueError as exc:
            raise ValueError(
                f"RAG_EMBED_DIM должен быть целым числом, получено: "
                f"{os.environ.get('RAG_EMBED_DIM')!r}"
            ) from exc
        return BGEM3Embedder(base_url=base_url, dim=dim)

    raise ValueError(
        f"Неизвестный RAG_EMBEDDER={resolved!r}. "
        f"Поддерживаемые значения: 'mock' (CI/dev) или 'tei' (production BGE-M3)."
    )
