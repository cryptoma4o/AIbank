from __future__ import annotations
from fastapi import APIRouter
from core.router import router as model_router

api = APIRouter()


@api.get("/v1/models")
async def list_models() -> dict:
    models = [
        {"id": name, "object": "model", "owned_by": "aibank"}
        for name in model_router.list_models()
    ]
    return {"object": "list", "data": models}
