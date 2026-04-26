from __future__ import annotations
from enum import StrEnum
from pydantic import BaseModel

class ChangeType(StrEnum):
    DIRECTOR_CHANGED = "director_changed"
    ADDRESS_CHANGED = "address_changed"
    OKVED_CHANGED = "okved_changed"
    ROSFINMON_HIT = "rosfinmon_hit"
    STATUS_CHANGED = "status_changed"
    NO_CHANGE = "no_change"

class ReconcileRequest(BaseModel):
    application_id: str
    tenant_id: str
    inn: str
    previous_egrul_data: dict   # snapshot stored when application was submitted

class ReconcileChange(BaseModel):
    change_type: ChangeType
    field: str
    old_value: str
    new_value: str

class ReconcileResult(BaseModel):
    application_id: str
    inn: str
    changes: list[ReconcileChange]
    requires_review: bool       # True if any significant change detected
    rosfinmon_blocked: bool
