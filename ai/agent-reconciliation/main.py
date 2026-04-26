from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from models.schemas import ReconcileRequest, ReconcileResult
from agent.reconciler import reconcile

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
app = FastAPI(title="AIbank Reconciliation Agent", version="0.1.0")

@app.post("/v1/reconcile", response_model=ReconcileResult)
async def reconcile_endpoint(req: ReconcileRequest) -> ReconcileResult:
    return await reconcile(req)

@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "agent-reconciliation"}

if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8103, reload=False)
