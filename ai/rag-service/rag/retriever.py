from __future__ import annotations
import logging
from models.schemas import SearchRequest, SearchResult

log = logging.getLogger(__name__)

QDRANT_URL = "http://qdrant:6333"
COLLECTION = "aibank_compliance"


async def search(req: SearchRequest) -> list[SearchResult]:
    """
    Pre-MVP stub: returns empty results when Qdrant is unavailable.
    Production: compute embedding via llm-gateway /v1/embeddings, search Qdrant.
    """
    try:
        from qdrant_client import AsyncQdrantClient
        client = AsyncQdrantClient(url=QDRANT_URL)
        # Real implementation: embed query, search collection
        # Stub: collection may not exist yet
        collections = await client.get_collections()
        names = [c.name for c in collections.collections]
        if COLLECTION not in names:
            return []
        # Would do: results = await client.search(collection_name=COLLECTION, ...)
        return []
    except Exception as exc:
        log.warning("Qdrant unavailable: %s", exc)
        return []
