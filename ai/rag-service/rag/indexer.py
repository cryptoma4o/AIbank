from __future__ import annotations
import logging
from models.schemas import IndexRequest

log = logging.getLogger(__name__)


async def index_document(req: IndexRequest) -> int:
    """
    Pre-MVP stub. Production: chunk content, embed each chunk, upsert into Qdrant.
    Returns number of chunks indexed.
    """
    chunks = [req.content[i:i + req.chunk_size] for i in range(0, len(req.content), req.chunk_size)]
    log.info("index_document: doc_id=%s chunks=%d (stub, not stored)", req.doc_id, len(chunks))
    return len(chunks)
