"""Unit tests for UBO tracer with fake gateway."""
from __future__ import annotations

import json
from typing import Any

import pytest

from agent import tracer as tracer_mod
from agent.tracer import trace_ubo
from models.schemas import OwnershipExtract, UBORequest


class FakeGateway:
    def __init__(self, payload: dict[str, Any] | None = None) -> None:
        self.payload = payload
        self.calls: list[dict[str, Any]] = []

    async def chat(self, role: str, messages: list[dict[str, str]],
                   tenant_id: str, max_tokens: int = 1024,
                   temperature: float = 0.1) -> dict[str, Any]:
        self.calls.append({"role": role, "tenant_id": tenant_id})
        content = json.dumps(self.payload or {}, ensure_ascii=False)
        return {
            "choices": [{"message": {"content": content}}],
            "aibank_gateway": {
                "role": role, "resolved_model": "mock-fast", "backend": "mock",
                "used_fallback": False, "tenant_id": tenant_id, "latency_ms": 0.5,
            },
        }


@pytest.mark.asyncio
async def test_simple_chain_one_owner_100pct() -> None:
    req = UBORequest(
        tenant_id="bank-alpha",
        application_id="app-1",
        root_inn="7700000001",
        ownership_extracts=[
            OwnershipExtract(
                legal_entity_inn="7700000001",
                legal_entity_name="ООО Ромашка",
                owners=[
                    {"id": "p1", "type": "person", "name": "Иванов И.И.",
                     "share_percent": 100.0},
                ],
            ),
        ],
    )
    fake = FakeGateway(payload={
        "nodes": [
            {"id": "7700000001", "type": "legal_entity", "name": "ООО Ромашка"},
            {"id": "p1", "type": "person", "name": "Иванов И.И."},
        ],
        "edges": [{"from": "p1", "to": "7700000001", "share_percent": 100.0}],
        "ubos": [{
            "person_id": "p1", "name": "Иванов И.И.",
            "effective_share_percent": 100.0,
            "control_basis": "ownership", "paths": [["p1", "7700000001"]],
        }],
        "confidence": 0.95,
        "unresolved_branches": [],
    })
    result = await trace_ubo(req, client=fake)
    assert len(result.ubos) == 1
    assert result.ubos[0].name == "Иванов И.И."
    assert result.ubos[0].effective_share_percent == pytest.approx(100.0)
    assert result.confidence == pytest.approx(0.95)
    assert fake.calls[0]["role"] == "ubo-tracing"


@pytest.mark.asyncio
async def test_branching_50_50_two_ubos() -> None:
    req = UBORequest(
        tenant_id="bank-alpha",
        application_id="app-2",
        root_inn="7700000002",
        ownership_extracts=[
            OwnershipExtract(
                legal_entity_inn="7700000002",
                owners=[
                    {"id": "p1", "type": "person", "name": "A", "share_percent": 50.0},
                    {"id": "p2", "type": "person", "name": "B", "share_percent": 50.0},
                ],
            ),
        ],
    )
    fake = FakeGateway(payload={
        "nodes": [
            {"id": "7700000002", "type": "legal_entity", "name": "LE"},
            {"id": "p1", "type": "person", "name": "A"},
            {"id": "p2", "type": "person", "name": "B"},
        ],
        "edges": [
            {"from": "p1", "to": "7700000002", "share_percent": 50.0},
            {"from": "p2", "to": "7700000002", "share_percent": 50.0},
        ],
        "ubos": [
            {"person_id": "p1", "name": "A", "effective_share_percent": 50.0,
             "control_basis": "ownership", "paths": [["p1", "7700000002"]]},
            {"person_id": "p2", "name": "B", "effective_share_percent": 50.0,
             "control_basis": "ownership", "paths": [["p2", "7700000002"]]},
        ],
        "confidence": 0.9,
        "unresolved_branches": [],
    })
    result = await trace_ubo(req, client=fake)
    assert {u.person_id for u in result.ubos} == {"p1", "p2"}
    assert all(u.effective_share_percent == pytest.approx(50.0) for u in result.ubos)


@pytest.mark.asyncio
async def test_nested_legal_entity_chain() -> None:
    """LE owns LE owns person — verify person treated as UBO via heuristic too."""
    req = UBORequest(
        tenant_id="bank-alpha",
        application_id="app-3",
        root_inn="7700000010",
        ownership_extracts=[
            OwnershipExtract(
                legal_entity_inn="7700000010",
                owners=[{
                    "id": "7700000020", "type": "legal_entity",
                    "name": "Холдинг",
                    "share_percent": 100.0,
                }],
            ),
            OwnershipExtract(
                legal_entity_inn="7700000020",
                owners=[{
                    "id": "p9", "type": "person", "name": "Сидоров С.С.",
                    "share_percent": 100.0,
                }],
            ),
        ],
    )
    # LLM returns empty payload — heuristic should kick in.
    fake = FakeGateway(payload={"nodes": [], "edges": [], "ubos": [],
                                "confidence": 0.0, "unresolved_branches": []})
    result = await trace_ubo(req, client=fake)
    assert result.ubos, "heuristic should produce a UBO"
    assert result.ubos[0].person_id == "p9"
    assert result.ubos[0].effective_share_percent == pytest.approx(100.0)


@pytest.mark.asyncio
async def test_unresolved_foreign_branch() -> None:
    req = UBORequest(
        tenant_id="bank-alpha",
        application_id="app-4",
        root_inn="7700000030",
        ownership_extracts=[
            OwnershipExtract(
                legal_entity_inn="7700000030",
                owners=[{
                    "id": "foreign-cy-1", "type": "legal_entity",
                    "name": "Cyprus Holdings Ltd",
                    "share_percent": 100.0,
                }],
            ),
        ],
    )
    fake = FakeGateway(payload={
        "nodes": [
            {"id": "7700000030", "type": "legal_entity", "name": "Корень"},
            {"id": "foreign-cy-1", "type": "legal_entity", "name": "Cyprus Holdings Ltd"},
        ],
        "edges": [{"from": "foreign-cy-1", "to": "7700000030", "share_percent": 100.0}],
        "ubos": [],
        "confidence": 0.4,
        "unresolved_branches": ["иностранный холдинг Cyprus Holdings Ltd"],
    })
    result = await trace_ubo(req, client=fake)
    assert result.ubos == []
    assert any("Cyprus" in b or "иностранный" in b for b in result.unresolved_branches)


@pytest.mark.asyncio
async def test_below_threshold_filtered() -> None:
    """UBOs below 25% must be filtered out even if LLM lists them."""
    req = UBORequest(
        tenant_id="bank-alpha",
        application_id="app-5",
        root_inn="7700000040",
        ownership_extracts=[],
    )
    fake = FakeGateway(payload={
        "nodes": [{"id": "p1", "type": "person", "name": "A"}],
        "edges": [],
        "ubos": [{
            "person_id": "p1", "name": "A", "effective_share_percent": 10.0,
            "control_basis": "ownership", "paths": [],
        }],
        "confidence": 0.7, "unresolved_branches": [],
    })
    result = await trace_ubo(req, client=fake)
    assert result.ubos == []


@pytest.mark.asyncio
async def test_role_is_ubo_tracing() -> None:
    assert tracer_mod.ROLE == "ubo-tracing"
