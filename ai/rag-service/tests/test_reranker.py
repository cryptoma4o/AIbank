"""Тесты pluggable Reranker — LexicalReranker, NoOpReranker, BGEReranker, build_reranker."""
from __future__ import annotations

import json

import httpx
import pytest

from service.reranker import (
    BGEReranker,
    LexicalReranker,
    NoOpReranker,
    build_reranker,
)


@pytest.mark.asyncio
async def test_lexical_reranker_jaccard_overlap():
    r = LexicalReranker()
    scores = await r.rerank(
        "115-ФЗ контроль операций",
        [
            "Контроль операций по 115-ФЗ — обязанность кредитной организации",  # high
            "Информация о новостях рынка облигаций",  # low
            "",
        ],
    )
    assert len(scores) == 3
    assert scores[0] > scores[1], "релевантный текст должен иметь больший score"
    assert scores[2] == 0.0, "пустой текст → 0"


@pytest.mark.asyncio
async def test_lexical_reranker_empty_query():
    r = LexicalReranker()
    scores = await r.rerank("", ["any text", "another"])
    assert scores == [0.0, 0.0]


@pytest.mark.asyncio
async def test_noop_reranker_returns_zeros():
    r = NoOpReranker()
    scores = await r.rerank("query", ["a", "b", "c"])
    assert scores == [0.0, 0.0, 0.0]
    assert r.name == "noop"


@pytest.mark.asyncio
async def test_bge_reranker_tei_response_shape():
    """TEI shape: список dict'ов с index+score."""

    def handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["query"] == "тест"
        assert body["texts"] == ["один", "два"]
        return httpx.Response(
            200,
            json=[
                {"index": 0, "score": 0.85},
                {"index": 1, "score": 0.42},
            ],
        )

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    r = BGEReranker(url="http://bge:8080", client=client)
    try:
        scores = await r.rerank("тест", ["один", "два"])
    finally:
        await client.aclose()
    assert scores == [0.85, 0.42]


@pytest.mark.asyncio
async def test_bge_reranker_scores_dict_shape():
    """Альтернативная shape: {scores: [...]} — кастомный wrapper."""

    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"scores": [0.7, 0.3, 0.5]})

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    r = BGEReranker(url="http://bge:8080", client=client)
    try:
        scores = await r.rerank("q", ["a", "b", "c"])
    finally:
        await client.aclose()
    assert scores == [0.7, 0.3, 0.5]


@pytest.mark.asyncio
async def test_bge_reranker_logits_normalized():
    """Если backend отдаёт логиты (>1 или <0), применяется sigmoid."""

    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"scores": [3.0, -2.0, 0.0]})

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    r = BGEReranker(url="http://bge:8080", client=client)
    try:
        scores = await r.rerank("q", ["a", "b", "c"])
    finally:
        await client.aclose()
    # sigmoid(3) ≈ 0.95, sigmoid(-2) ≈ 0.12, sigmoid(0) = 0.5
    assert all(0.0 <= s <= 1.0 for s in scores)
    assert scores[0] > 0.9
    assert scores[1] < 0.2
    assert abs(scores[2] - 0.5) < 0.01


@pytest.mark.asyncio
async def test_bge_reranker_empty_candidates():
    r = BGEReranker(url="http://bge:8080")
    scores = await r.rerank("q", [])
    assert scores == []


def test_bge_reranker_requires_url():
    with pytest.raises(ValueError):
        BGEReranker(url="")


def test_build_reranker_default_lexical(monkeypatch):
    monkeypatch.delenv("RAG_RERANKER", raising=False)
    r = build_reranker()
    assert isinstance(r, LexicalReranker)


def test_build_reranker_noop(monkeypatch):
    monkeypatch.setenv("RAG_RERANKER", "noop")
    r = build_reranker()
    assert isinstance(r, NoOpReranker)


def test_build_reranker_bge_with_url(monkeypatch):
    monkeypatch.setenv("RAG_RERANKER", "bge")
    monkeypatch.setenv("RAG_RERANKER_URL", "http://bge.test:8080")
    monkeypatch.setenv("RAG_RERANKER_MODEL", "bge-test-v1")
    r = build_reranker()
    assert isinstance(r, BGEReranker)
    assert r.url == "http://bge.test:8080"
    assert r.model == "bge-test-v1"


def test_build_reranker_bge_without_url_falls_back(monkeypatch):
    """RAG_RERANKER=bge но без URL → fallback на lexical (вместо crash)."""
    monkeypatch.setenv("RAG_RERANKER", "bge")
    monkeypatch.delenv("RAG_RERANKER_URL", raising=False)
    r = build_reranker()
    assert isinstance(r, LexicalReranker)
