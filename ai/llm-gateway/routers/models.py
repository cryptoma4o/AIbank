from __future__ import annotations

from fastapi import APIRouter

from core.router import get_router

api = APIRouter()


@api.get("/v1/models")
def list_models() -> dict:
    rt = get_router()
    models = [
        {"id": name, "object": "model", "owned_by": "aibank"}
        for name in rt.list_models()
    ]
    roles = [
        {"id": f"role:{name}", "object": "role", "owned_by": "aibank"}
        for name in rt.list_roles()
    ]
    return {"object": "list", "data": models + roles}
