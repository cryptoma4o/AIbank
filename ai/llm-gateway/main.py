from __future__ import annotations

import logging

import uvicorn
from fastapi import FastAPI

from core.config import settings
from core.observability import init_otel
from routers.completions import api as completions_router
from routers.models import api as models_router
from routers.usage import api as usage_router

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

app = FastAPI(title="AIbank LLM Gateway", version="0.2.0")
app.include_router(completions_router)
app.include_router(models_router)
app.include_router(usage_router)

# OTEL is a no-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset (default for tests).
init_otel(app, service_name="llm-gateway", version="0.2.0")


@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "llm-gateway", "force_mock": settings.force_mock}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=settings.port, reload=False)
