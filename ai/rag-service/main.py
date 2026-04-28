"""rag-service FastAPI app — semantic search over banking regulatory corpus.

Endpoints:
- POST /v1/search  — public search used by agent-compliance-assistant
- POST /v1/index   — admin/ingestion (per-tenant corpus loader)
- GET  /healthz    — liveness probe

Per-tenant изоляция реализована именами коллекций (`rag_<tenant>`).
"""
from __future__ import annotations

import hashlib
import logging
import os

import uvicorn
from fastapi import FastAPI, Header, HTTPException, Request

from models.schemas import (
    IndexRequest,
    IndexResponse,
    SearchRequest,
    SearchResponse,
)
from service import observability as obs
from service.embedder import (
    BGEM3Embedder,
    Embedder,
    MockEmbedder,
    build_embedder,
)
from service.qdrant_client import VectorStore, build_default_store
from service.reranker import Reranker, build_reranker
from service.retriever import Retriever

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
log = logging.getLogger(__name__)

app = FastAPI(
    title="AIbank RAG Service",
    version="0.1.0",
    description="Semantic + lexical retrieval over Russian banking regulations corpus.",
)
obs.init_otel(app)


def _embedder_kind(embedder: Embedder) -> str:
    """Человекочитаемое имя embedder-а для /healthz и логов."""
    if isinstance(embedder, BGEM3Embedder):
        return "tei"
    if isinstance(embedder, MockEmbedder):
        return "mock"
    return type(embedder).__name__.lower()


def _build_components() -> tuple[Retriever, VectorStore, Embedder, Reranker]:
    """Create retriever and dependencies. Test code monkeypatches `app.state.retriever`.

    Embedder через ``build_embedder()`` (env ``RAG_EMBEDDER``, "mock" default).
    Production — ``tei`` с BAAI/bge-m3 (ADR-0008).

    Reranker через ``build_reranker()`` (env ``RAG_RERANKER``, "lexical" default).
    После получения GPU — ``bge`` с BAAI/bge-reranker-v2-m3.
    """
    embedder: Embedder = build_embedder()
    store: VectorStore = build_default_store()
    reranker: Reranker = build_reranker()
    retriever = Retriever(store=store, embedder=embedder, reranker=reranker)
    return retriever, store, embedder, reranker


@app.on_event("startup")
async def _startup() -> None:
    retriever, store, embedder, reranker = _build_components()
    app.state.retriever = retriever
    app.state.store = store
    app.state.embedder = embedder
    app.state.reranker = reranker
    log.info(
        "rag-service готов (backend=%s, embedder=%s, embed_dim=%d, reranker=%s)",
        store.backend_name,
        _embedder_kind(embedder),
        embedder.dim,
        reranker.name,
    )


def _get_retriever(request: Request) -> Retriever:
    retriever = getattr(request.app.state, "retriever", None)
    if retriever is None:
        retriever, store, embedder, reranker = _build_components()
        request.app.state.retriever = retriever
        request.app.state.store = store
        request.app.state.embedder = embedder
        request.app.state.reranker = reranker
    return retriever


@app.get("/healthz")
async def healthz(request: Request) -> dict:
    # Trigger lazy init so tests using TestClient without a `with` block
    # still see the backend name.
    _get_retriever(request)
    store = request.app.state.store
    embedder = request.app.state.embedder
    reranker = getattr(request.app.state, "reranker", None)
    return {
        "status": "ok",
        "service": "rag-service",
        "backend": store.backend_name,
        "embedder": _embedder_kind(embedder),
        "dim": embedder.dim,
        "reranker": reranker.name if reranker else "unknown",
    }


@app.post("/v1/search", response_model=SearchResponse)
async def search(
    req: SearchRequest,
    request: Request,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> SearchResponse:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    if not req.query.strip():
        raise HTTPException(status_code=400, detail="query must not be empty")
    retriever = _get_retriever(request)
    hits = await retriever.search(
        tenant_id=req.tenant_id,
        query=req.query,
        top_k=max(1, min(req.top_k, 20)),
        filters=req.filters,
    )
    # Hash query — it may carry sensitive context.
    entity_id = hashlib.sha256(req.query.strip().encode("utf-8")).hexdigest()[:16]
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=entity_id,
        actor_id=x_actor_id or "system",
        payload={
            "top_k": req.top_k,
            "hit_count": len(hits),
            "backend": request.app.state.store.backend_name,
        },
    )
    return SearchResponse(
        tenant_id=req.tenant_id,
        query=req.query,
        hits=hits,
        backend=request.app.state.store.backend_name,
    )


@app.post("/v1/index", response_model=IndexResponse)
async def index(req: IndexRequest, request: Request) -> IndexResponse:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    retriever = _get_retriever(request)
    collection, indexed = await retriever.index(req.tenant_id, req.documents)
    return IndexResponse(
        tenant_id=req.tenant_id,
        indexed=indexed,
        collection=collection,
    )


if __name__ == "__main__":  # pragma: no cover
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.environ.get("PORT", "8105")),
        reload=False,
    )
