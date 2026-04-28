"""GET /v1/usage — отчёт о потреблении токенов per (tenant, model)."""
from __future__ import annotations

from fastapi import APIRouter, Query

from core.usage import tracker

api = APIRouter()


@api.get("/v1/usage")
def get_usage(tenant_id: str | None = Query(default=None)) -> dict:
    return {
        "object": "list",
        "data": tracker.snapshot(tenant_id=tenant_id),
        "aggregate_by_tenant": tracker.aggregate_by_tenant(),
    }
