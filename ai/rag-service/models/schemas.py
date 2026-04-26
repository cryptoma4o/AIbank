from __future__ import annotations
from pydantic import BaseModel


class IndexRequest(BaseModel):
    doc_id: str
    title: str
    content: str
    source: str     # e.g. "115-fz", "375-p", "499-p"
    chunk_size: int = 500


class SearchRequest(BaseModel):
    query: str
    top_k: int = 5
    source_filter: str | None = None   # filter by regulation source


class SearchResult(BaseModel):
    doc_id: str
    title: str
    chunk: str
    score: float
    source: str


class AskRequest(BaseModel):
    question: str
    top_k: int = 3
    source_filter: str | None = None


class AskResponse(BaseModel):
    answer: str
    sources: list[SearchResult]
    model_used: str
