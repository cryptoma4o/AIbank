"""UBO tracing logic — builds ownership graph and identifies UBOs (>=25%)."""
from __future__ import annotations

import json
import logging
from typing import Any

from agent.gateway_client import (
    GatewayClient,
    GatewayError,
    extract_content,
    extract_metadata,
)
from agent.prompts import SYSTEM_PROMPT, USER_PROMPT_TEMPLATE
from models.schemas import (
    ControlBasis,
    GraphEdge,
    GraphNode,
    NodeType,
    OwnershipExtract,
    UBO,
    UBORequest,
    UBOResult,
)

log = logging.getLogger(__name__)

ROLE = "ubo-tracing"
UBO_THRESHOLD = 25.0  # 115-ФЗ


def _parse_llm_payload(content: str) -> dict[str, Any]:
    text = content.strip()
    if text.startswith("```"):
        text = text.strip("`")
        if text.lower().startswith("json"):
            text = text[4:]
        text = text.strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            return json.loads(text[start : end + 1])
        raise


def _coerce_node_type(raw: Any) -> NodeType:
    s = str(raw or "").lower().replace("-", "_")
    if s in ("person", "individual", "natural"):
        return NodeType.PERSON
    return NodeType.LEGAL_ENTITY


def _coerce_control(raw: Any) -> ControlBasis:
    s = str(raw or "ownership").lower()
    if s in ("voting", "votes"):
        return ControlBasis.VOTING
    if s in ("appointment", "managerial"):
        return ControlBasis.APPOINTMENT
    return ControlBasis.OWNERSHIP


def _coerce_nodes(raw: Any) -> list[GraphNode]:
    out: list[GraphNode] = []
    if not isinstance(raw, list):
        return out
    for item in raw:
        if not isinstance(item, dict):
            continue
        node_id = str(item.get("id") or item.get("inn") or item.get("name") or "")
        name = str(item.get("name") or node_id)
        if not node_id:
            continue
        out.append(
            GraphNode(
                id=node_id,
                type=_coerce_node_type(item.get("type")),
                name=name,
                inn=item.get("inn") or None,
            )
        )
    return out


def _coerce_edges(raw: Any) -> list[GraphEdge]:
    out: list[GraphEdge] = []
    if not isinstance(raw, list):
        return out
    for item in raw:
        if not isinstance(item, dict):
            continue
        try:
            out.append(
                GraphEdge(
                    **{
                        "from": str(item.get("from", "")),
                        "to": str(item.get("to", "")),
                        "share_percent": float(item.get("share_percent", 0) or 0),
                    }
                )
            )
        except (ValueError, TypeError) as exc:
            log.warning("skip malformed edge %s: %s", item, exc)
    return out


def _coerce_ubos(raw: Any) -> list[UBO]:
    out: list[UBO] = []
    if not isinstance(raw, list):
        return out
    for item in raw:
        if not isinstance(item, dict):
            continue
        try:
            paths_raw = item.get("paths") or []
            paths = [
                [str(x) for x in p] for p in paths_raw if isinstance(p, list)
            ]
            out.append(
                UBO(
                    person_id=str(item.get("person_id") or item.get("id") or ""),
                    name=str(item.get("name", "")),
                    effective_share_percent=float(
                        item.get("effective_share_percent", 0) or 0
                    ),
                    control_basis=_coerce_control(item.get("control_basis")),
                    paths=paths,
                )
            )
        except (ValueError, TypeError) as exc:
            log.warning("skip malformed UBO %s: %s", item, exc)
    return out


def _heuristic_trace(
    root_inn: str, extracts: list[OwnershipExtract]
) -> tuple[list[GraphNode], list[GraphEdge], list[UBO], list[str]]:
    """Deterministic graph traversal as fallback / sanity baseline.

    Computes effective shares by multiplying share_percent along paths from
    every person up to the root_inn. Branches that terminate without reaching
    a person (e.g. foreign holding stub) go into unresolved_branches.
    """
    nodes: dict[str, GraphNode] = {}
    edges: list[GraphEdge] = []
    # adj: parent_id -> list[(child_id, share_percent)]
    adj: dict[str, list[tuple[str, float]]] = {}
    has_owners: set[str] = set()

    def _ensure_node(node_id: str, name: str, type_: NodeType,
                     inn: str | None = None) -> None:
        if node_id and node_id not in nodes:
            nodes[node_id] = GraphNode(id=node_id, type=type_, name=name or node_id, inn=inn)

    for extract in extracts:
        le_id = extract.legal_entity_inn
        _ensure_node(
            le_id,
            extract.legal_entity_name or le_id,
            NodeType.LEGAL_ENTITY,
            extract.legal_entity_inn,
        )
        for owner in extract.owners:
            owner_id = str(
                owner.get("id")
                or owner.get("inn")
                or owner.get("name")
                or ""
            )
            if not owner_id:
                continue
            owner_type = _coerce_node_type(owner.get("type"))
            _ensure_node(
                owner_id,
                str(owner.get("name") or owner_id),
                owner_type,
                owner.get("inn"),
            )
            share = float(owner.get("share_percent", 0) or 0)
            edges.append(GraphEdge(**{"from": owner_id, "to": le_id, "share_percent": share}))
            adj.setdefault(le_id, []).append((owner_id, share))
            has_owners.add(le_id)

    # DFS from root upwards through ownership chain.
    ubos_acc: dict[str, UBO] = {}
    unresolved: list[str] = []

    def _walk(le_id: str, accumulated: float, path: list[str]) -> None:
        if le_id not in adj:
            # Branch ends at a legal entity without further owners that isn't a person.
            if nodes.get(le_id) and nodes[le_id].type == NodeType.LEGAL_ENTITY:
                label = f"unresolved chain via {nodes[le_id].name} ({le_id})"
                if label not in unresolved:
                    unresolved.append(label)
            return
        for owner_id, share in adj[le_id]:
            new_share = accumulated * (share / 100.0)
            new_path = path + [owner_id]
            owner_node = nodes.get(owner_id)
            if owner_node and owner_node.type == NodeType.PERSON:
                effective_pct = round(new_share * 100.0, 4)
                existing = ubos_acc.get(owner_id)
                if existing is None:
                    ubos_acc[owner_id] = UBO(
                        person_id=owner_id,
                        name=owner_node.name,
                        effective_share_percent=effective_pct,
                        control_basis=ControlBasis.OWNERSHIP,
                        paths=[list(reversed(new_path + [le_id]))],
                    )
                else:
                    existing.effective_share_percent = round(
                        existing.effective_share_percent + effective_pct, 4
                    )
                    existing.paths.append(list(reversed(new_path + [le_id])))
            else:
                _walk(owner_id, new_share, new_path)

    if root_inn in adj:
        _walk(root_inn, 1.0, [])
    elif root_inn in nodes:
        unresolved.append(f"root {root_inn} has no ownership data")

    ubos = [u for u in ubos_acc.values() if u.effective_share_percent >= UBO_THRESHOLD]
    return list(nodes.values()), edges, ubos, unresolved


async def trace_ubo(
    req: UBORequest, client: GatewayClient | None = None
) -> UBOResult:
    extracts_json = json.dumps(
        [e.model_dump() for e in req.ownership_extracts],
        ensure_ascii=False,
        indent=2,
    )
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {
            "role": "user",
            "content": USER_PROMPT_TEMPLATE.format(
                root_inn=req.root_inn, extracts_json=extracts_json
            ),
        },
    ]

    gw = client or GatewayClient()
    metadata: dict[str, Any] = {"application_id": req.application_id}
    nodes: list[GraphNode] = []
    edges: list[GraphEdge] = []
    ubos: list[UBO] = []
    unresolved: list[str] = []
    confidence = 0.0
    llm_ok = False

    try:
        response = await gw.chat(
            role=ROLE,
            messages=messages,
            tenant_id=req.tenant_id,
            max_tokens=1500,
        )
        metadata.update(extract_metadata(response))
        payload = _parse_llm_payload(extract_content(response))
        nodes = _coerce_nodes(payload.get("nodes"))
        edges = _coerce_edges(payload.get("edges"))
        ubos = _coerce_ubos(payload.get("ubos"))
        unresolved = [str(x) for x in (payload.get("unresolved_branches") or [])]
        try:
            confidence = float(payload.get("confidence", 0) or 0)
        except (TypeError, ValueError):
            confidence = 0.0
        confidence = max(0.0, min(1.0, confidence))
        llm_ok = True
    except (GatewayError, json.JSONDecodeError, ValueError) as exc:
        log.warning("LLM UBO trace failed app=%s: %s", req.application_id, exc)
        metadata["llm_error"] = str(exc)

    # If LLM didn't produce a usable graph, fall back to deterministic traversal.
    if not nodes or not ubos:
        h_nodes, h_edges, h_ubos, h_unresolved = _heuristic_trace(
            req.root_inn, req.ownership_extracts
        )
        if not nodes:
            nodes = h_nodes
        if not edges:
            edges = h_edges
        if not ubos:
            ubos = h_ubos
        for u in h_unresolved:
            if u not in unresolved:
                unresolved.append(u)
        if not llm_ok:
            confidence = 0.5  # heuristic-only baseline

    # Filter UBOs by 25% threshold (defensive — LLM may include below-threshold).
    ubos = [u for u in ubos if u.effective_share_percent >= UBO_THRESHOLD]

    return UBOResult(
        nodes=nodes,
        edges=edges,
        ubos=ubos,
        confidence=confidence,
        unresolved_branches=unresolved,
        gateway_metadata=metadata,
    )
