"""Tests for embedder factory + BGE-M3 HTTP client.

MockEmbedder уже косвенно покрыт в test_retriever.py — здесь только
санити-чек, что фабрика отдаёт его и что он работает изолированно.

BGEM3Embedder тестируется с моком httpx через `respx`:
  - happy path
  - L2-normalization
  - retry на 5xx
  - окончательный фейл после исчерпания retries
  - авто-батчинг (batch_size < len(texts))
"""
from __future__ import annotations

import math
from typing import Any

import httpx
import pytest
import respx

from service.embedder import (
    BGEM3Embedder,
    DEFAULT_BGE_DIM,
    EMBED_DIM,
    MockEmbedder,
    build_embedder,
)


# ---------------------------------------------------------------------------
# MockEmbedder — sanity check (полное покрытие в test_retriever.py)
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_mock_embedder_returns_normalized_dim_vectors() -> None:
    emb = MockEmbedder()
    [v] = await emb.embed(["идентификация клиента"])
    assert len(v) == EMBED_DIM
    norm_sq = sum(x * x for x in v)
    assert math.isclose(norm_sq, 1.0, rel_tol=1e-6, abs_tol=1e-6)


# ---------------------------------------------------------------------------
# BGEM3Embedder — happy path & validation
# ---------------------------------------------------------------------------


def _make_unnormalized_vectors(n: int, dim: int) -> list[list[float]]:
    """Сгенерировать ненормированные векторы для проверки L2-нормализации."""
    out: list[list[float]] = []
    for i in range(n):
        vec = [float((i + 1) * (j + 1)) for j in range(dim)]
        out.append(vec)
    return out


@pytest.mark.asyncio
@respx.mock
async def test_bgem3_happy_path_returns_dim_vectors() -> None:
    dim = 8
    raw = _make_unnormalized_vectors(2, dim)
    route = respx.post("http://tei-test:8080/embed").mock(
        return_value=httpx.Response(200, json=raw)
    )

    emb = BGEM3Embedder(base_url="http://tei-test:8080", dim=dim, max_retries=2)
    vectors = await emb.embed(["a", "b"])

    assert route.call_count == 1
    assert len(vectors) == 2
    assert all(len(v) == dim for v in vectors)


@pytest.mark.asyncio
@respx.mock
async def test_bgem3_l2_normalization_applied() -> None:
    dim = 4
    raw = [[3.0, 0.0, 4.0, 0.0]]  # норма = 5
    respx.post("http://tei-test:8080/embed").mock(
        return_value=httpx.Response(200, json=raw)
    )

    emb = BGEM3Embedder(base_url="http://tei-test:8080", dim=dim)
    [vec] = await emb.embed(["x"])

    norm_sq = sum(x * x for x in vec)
    assert math.isclose(norm_sq, 1.0, rel_tol=1e-6, abs_tol=1e-6)
    # И направление сохранено: [0.6, 0, 0.8, 0].
    assert math.isclose(vec[0], 0.6, abs_tol=1e-6)
    assert math.isclose(vec[2], 0.8, abs_tol=1e-6)


@pytest.mark.asyncio
@respx.mock
async def test_bgem3_dim_mismatch_raises() -> None:
    raw = [[1.0, 2.0, 3.0]]  # dim=3, ждём 4
    respx.post("http://tei-test:8080/embed").mock(
        return_value=httpx.Response(200, json=raw)
    )

    emb = BGEM3Embedder(base_url="http://tei-test:8080", dim=4, max_retries=0)
    with pytest.raises(httpx.HTTPError):
        await emb.embed(["x"])


# ---------------------------------------------------------------------------
# BGEM3Embedder — retries
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
@respx.mock
async def test_bgem3_retries_on_5xx_and_succeeds() -> None:
    dim = 4
    raw = [[1.0, 0.0, 0.0, 0.0]]
    # respx side_effect-сценарий: первый ответ 503, второй 200.
    route = respx.post("http://tei-test:8080/embed").mock(
        side_effect=[
            httpx.Response(503, text="model loading"),
            httpx.Response(200, json=raw),
        ]
    )

    emb = BGEM3Embedder(
        base_url="http://tei-test:8080", dim=dim, max_retries=2
    )
    vectors = await emb.embed(["x"])

    assert route.call_count == 2
    assert len(vectors) == 1
    assert len(vectors[0]) == dim


@pytest.mark.asyncio
@respx.mock
async def test_bgem3_fails_after_exhausting_retries() -> None:
    # max_retries=2 → всего 3 попытки. Все 5xx.
    route = respx.post("http://tei-test:8080/embed").mock(
        side_effect=[
            httpx.Response(503, text="boom"),
            httpx.Response(503, text="boom"),
            httpx.Response(503, text="boom"),
        ]
    )

    emb = BGEM3Embedder(base_url="http://tei-test:8080", dim=4, max_retries=2)
    with pytest.raises(httpx.HTTPError):
        await emb.embed(["x"])

    assert route.call_count == 3


# ---------------------------------------------------------------------------
# BGEM3Embedder — auto-batching
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
@respx.mock
async def test_bgem3_auto_batches_long_inputs() -> None:
    dim = 4
    batch_size = 100

    call_payloads: list[list[str]] = []

    def _responder(request: httpx.Request) -> httpx.Response:
        body: dict[str, Any] = httpx.Request("POST", "http://x", content=request.content).read() and {}
        # Парсим вручную, чтобы не зависеть от внутренностей httpx.
        import json as _json

        parsed = _json.loads(request.content)
        inputs = parsed["inputs"]
        call_payloads.append(list(inputs))
        # Возвращаем по одному простому вектору на каждый input.
        vectors = [[1.0, 0.0, 0.0, 0.0] for _ in inputs]
        return httpx.Response(200, json=vectors)

    respx.post("http://tei-test:8080/embed").mock(side_effect=_responder)

    emb = BGEM3Embedder(
        base_url="http://tei-test:8080",
        dim=dim,
        batch_size=batch_size,
        max_retries=0,
    )
    texts = [f"doc-{i}" for i in range(250)]
    vectors = await emb.embed(texts)

    # 250 текстов, batch_size=100 → 100 + 100 + 50 = 3 батч-запроса.
    assert len(call_payloads) == 3
    assert [len(p) for p in call_payloads] == [100, 100, 50]
    assert len(vectors) == 250


# ---------------------------------------------------------------------------
# Factory build_embedder()
# ---------------------------------------------------------------------------


def test_factory_kind_mock_returns_mock_embedder(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("RAG_EMBEDDER", raising=False)
    emb = build_embedder("mock")
    assert isinstance(emb, MockEmbedder)
    assert emb.dim == EMBED_DIM


def test_factory_kind_tei_returns_bgem3_embedder(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("RAG_TEI_URL", "http://my-tei:9000")
    monkeypatch.setenv("RAG_EMBED_DIM", "1024")
    emb = build_embedder("tei")
    assert isinstance(emb, BGEM3Embedder)
    assert emb.base_url == "http://my-tei:9000"
    assert emb.dim == 1024


def test_factory_unknown_kind_raises(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("RAG_EMBEDDER", raising=False)
    with pytest.raises(ValueError, match="Неизвестный RAG_EMBEDDER"):
        build_embedder("openai")


def test_factory_uses_env_var_when_kind_omitted(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("RAG_EMBEDDER", "tei")
    monkeypatch.setenv("RAG_TEI_URL", "http://from-env:8080")
    monkeypatch.setenv("RAG_EMBED_DIM", str(DEFAULT_BGE_DIM))
    emb = build_embedder()  # kind не указан → читаем env
    assert isinstance(emb, BGEM3Embedder)
    assert emb.base_url == "http://from-env:8080"


def test_factory_default_is_mock_when_no_env(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("RAG_EMBEDDER", raising=False)
    emb = build_embedder()
    assert isinstance(emb, MockEmbedder)
