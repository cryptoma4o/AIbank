from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from models.schemas import IndexRequest, SearchRequest, AskRequest, AskResponse, SearchResult
from rag.indexer import index_document
from rag.retriever import search
from rag.answerer import ask

logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

app = FastAPI(
    title="RAG Service",
    version="0.1.0",
    description="Retrieval-Augmented Generation service for AIbank Compliance Assistant.",
)


@app.get("/healthz")
async def healthz() -> JSONResponse:
    return JSONResponse({"status": "ok"})


@app.post("/v1/index")
async def index(req: IndexRequest) -> JSONResponse:
    log.info("Indexing doc_id=%s source=%s", req.doc_id, req.source)
    chunks_indexed = await index_document(req)
    return JSONResponse({"chunks_indexed": chunks_indexed})


@app.post("/v1/search", response_model=list[SearchResult])
async def search_docs(req: SearchRequest) -> list[SearchResult]:
    log.info("Search query=%r top_k=%d", req.query, req.top_k)
    return await search(req)


@app.post("/v1/ask", response_model=AskResponse)
async def ask_question(req: AskRequest) -> AskResponse:
    log.info("Ask question=%r", req.question)
    return await ask(req)


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8105, reload=False)
