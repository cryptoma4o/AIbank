from __future__ import annotations
from pydantic import BaseModel

class TraceRequest(BaseModel):
    application_id: str
    tenant_id: str
    inn: str
    ogrn: str
    max_depth: int = 3   # maximum ownership chain depth

class UBONodeOut(BaseModel):
    name: str
    node_type: str          # "person" | "company"
    inn: str = ""
    direct_stake: float     # 0-100 %

class TraceResult(BaseModel):
    application_id: str
    inn: str
    nodes: list[UBONodeOut]
    ubos: list[UBONodeOut]  # nodes with effective stake >= 25%
    depth_reached: int
    model_used: str
