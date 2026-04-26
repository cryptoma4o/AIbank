from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from routers.completions import api as completions_router
from routers.models import api as models_router

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

app = FastAPI(title="AIbank LLM Gateway", version="0.1.0")
app.include_router(completions_router)
app.include_router(models_router)


@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "llm-gateway"}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8100, reload=False)
