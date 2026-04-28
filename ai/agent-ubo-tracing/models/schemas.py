"""Pydantic schemas for UBO-tracing agent."""
from __future__ import annotations

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field


class NodeType(StrEnum):
    PERSON = "person"
    LEGAL_ENTITY = "legal_entity"


class ControlBasis(StrEnum):
    OWNERSHIP = "ownership"
    VOTING = "voting"
    APPOINTMENT = "appointment"


class OwnershipExtract(BaseModel):
    """Single piece of ownership data — typically one ЕГРЮЛ выписка."""

    legal_entity_inn: str
    legal_entity_name: str | None = None
    owners: list[dict[str, Any]] = Field(default_factory=list)
    # owner shape: {id, type: person|legal_entity, name, inn?, share_percent}


class UBORequest(BaseModel):
    tenant_id: str
    application_id: str
    root_inn: str
    ownership_extracts: list[OwnershipExtract] = Field(default_factory=list)


class GraphNode(BaseModel):
    id: str
    type: NodeType
    name: str
    inn: str | None = None


class GraphEdge(BaseModel):
    from_id: str = Field(alias="from")
    to_id: str = Field(alias="to")
    share_percent: float

    model_config = {"populate_by_name": True}


class UBO(BaseModel):
    person_id: str
    name: str
    effective_share_percent: float
    control_basis: ControlBasis = ControlBasis.OWNERSHIP
    paths: list[list[str]] = Field(default_factory=list)


class UBOResult(BaseModel):
    nodes: list[GraphNode] = Field(default_factory=list)
    edges: list[GraphEdge] = Field(default_factory=list)
    ubos: list[UBO] = Field(default_factory=list)
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    unresolved_branches: list[str] = Field(default_factory=list)
    gateway_metadata: dict[str, Any] = Field(default_factory=dict)
