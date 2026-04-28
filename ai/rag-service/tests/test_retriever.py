"""Retriever unit tests with the in-memory MemoryStore (no Qdrant needed)."""
from __future__ import annotations

import pytest

from models.schemas import IndexDocument, SearchFilters
from service.embedder import MockEmbedder
from service.qdrant_client import MemoryStore, collection_name
from service.retriever import Retriever


SAMPLE_DOCS_115FZ = [
    IndexDocument(
        id="115-fz-art7",
        text=(
            "Статья 7 115-ФЗ обязывает кредитные организации идентифицировать "
            "клиента и его представителя, а также бенефициарных владельцев."
        ),
        source="115-ФЗ ст.7",
        source_type="law",
    ),
    IndexDocument(
        id="115-fz-art6",
        text=(
            "Статья 6 115-ФЗ устанавливает перечень операций, подлежащих "
            "обязательному контролю при сумме свыше 600 тысяч рублей."
        ),
        source="115-ФЗ ст.6",
        source_type="law",
    ),
    IndexDocument(
        id="375-p-1",
        text=(
            "Положение 375-П Банка России определяет факторы повышенного риска "
            "проведения операций отмывания доходов и финансирования терроризма."
        ),
        source="375-П",
        source_type="regulation",
    ),
]


@pytest.fixture
def retriever() -> Retriever:
    return Retriever(store=MemoryStore(), embedder=MockEmbedder())


@pytest.mark.asyncio
async def test_index_returns_collection_and_count(retriever: Retriever) -> None:
    coll, n = await retriever.index("bank-alpha", SAMPLE_DOCS_115FZ)
    assert coll == collection_name("bank-alpha") == "rag_bank-alpha"
    assert n == 3


@pytest.mark.asyncio
async def test_search_recovers_relevant_doc(retriever: Retriever) -> None:
    await retriever.index("bank-alpha", SAMPLE_DOCS_115FZ)

    hits = await retriever.search(
        tenant_id="bank-alpha",
        query="идентификация бенефициарных владельцев клиента",
        top_k=3,
    )
    assert hits, "expected at least one hit"
    assert hits[0].document_id == "115-fz-art7"
    assert hits[0].score > 0
    assert "идентифицировать" in hits[0].snippet or "идентификация" in hits[0].snippet


@pytest.mark.asyncio
async def test_search_obligatory_control_threshold(retriever: Retriever) -> None:
    await retriever.index("bank-alpha", SAMPLE_DOCS_115FZ)

    hits = await retriever.search(
        tenant_id="bank-alpha",
        query="обязательный контроль операций сумма 600 тысяч",
        top_k=3,
    )
    assert hits[0].document_id == "115-fz-art6"


@pytest.mark.asyncio
async def test_search_filters_by_source_type(retriever: Retriever) -> None:
    await retriever.index("bank-alpha", SAMPLE_DOCS_115FZ)

    hits = await retriever.search(
        tenant_id="bank-alpha",
        query="факторы риска",
        top_k=5,
        filters=SearchFilters(source_type="regulation"),
    )
    assert hits, "expected the 375-П regulation to match"
    assert all(h.source_type == "regulation" for h in hits)
    assert hits[0].document_id == "375-p-1"


@pytest.mark.asyncio
async def test_tenant_isolation(retriever: Retriever) -> None:
    await retriever.index("bank-alpha", SAMPLE_DOCS_115FZ)
    await retriever.index(
        "bank-beta",
        [
            IndexDocument(
                id="internal-1",
                text="Внутренний документ банка-бета о лимитах операций.",
                source="internal-policy",
                source_type="internal",
            )
        ],
    )

    alpha_hits = await retriever.search(
        tenant_id="bank-alpha", query="внутренний документ лимиты", top_k=5
    )
    # Alpha doesn't have the beta internal doc — matching should miss it.
    assert all(h.document_id != "internal-1" for h in alpha_hits)

    beta_hits = await retriever.search(
        tenant_id="bank-beta", query="лимиты операций", top_k=5
    )
    assert any(h.document_id == "internal-1" for h in beta_hits)
    # Beta must NOT see 115-ФЗ docs from alpha tenant.
    assert all(not h.document_id.startswith("115-fz") for h in beta_hits)


@pytest.mark.asyncio
async def test_search_empty_corpus_returns_no_hits(retriever: Retriever) -> None:
    hits = await retriever.search(tenant_id="bank-empty", query="115-ФЗ", top_k=3)
    assert hits == []


@pytest.mark.asyncio
async def test_index_then_reindex_same_id_replaces(retriever: Retriever) -> None:
    await retriever.index(
        "bank-alpha",
        [IndexDocument(id="d1", text="первая версия документа", source="x")],
    )
    await retriever.index(
        "bank-alpha",
        [IndexDocument(id="d1", text="вторая обновлённая версия документа", source="x")],
    )
    hits = await retriever.search(
        tenant_id="bank-alpha", query="обновлённая версия", top_k=3
    )
    assert hits and hits[0].document_id == "d1"
    assert "обновлённая" in hits[0].snippet
