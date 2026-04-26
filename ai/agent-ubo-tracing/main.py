from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from models.schemas import TraceRequest, TraceResult
from agent.tracer import trace_ubo

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
app = FastAPI(title="AIbank UBO Tracing Agent", version="0.1.0")

@app.post("/v1/trace", response_model=TraceResult)
async def trace(req: TraceRequest) -> TraceResult:
    return await trace_ubo(req)

@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "agent-ubo-tracing"}

if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8102, reload=False)
