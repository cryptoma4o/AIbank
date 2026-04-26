from __future__ import annotations
import logging
import httpx
from models.schemas import ReconcileRequest, ReconcileResult, ReconcileChange, ChangeType

log = logging.getLogger(__name__)

EGRUL_URL = "http://ext-egrul:8090"
ROSFINMON_URL = "http://ext-rosfinmon:8091"

async def reconcile(req: ReconcileRequest) -> ReconcileResult:
    changes: list[ReconcileChange] = []
    rosfinmon_blocked = False

    async with httpx.AsyncClient(timeout=15.0) as client:
        # Fetch current ЕГРЮЛ data
        egrul_resp = await client.get(f"{EGRUL_URL}/v1/egrul/{req.inn}")
        current = egrul_resp.json() if egrul_resp.status_code == 200 else {}

        # Check Росфинмониторинг
        rosfinmon_resp = await client.get(f"{ROSFINMON_URL}/v1/rosfinmon/check/{req.inn}")
        if rosfinmon_resp.status_code == 200:
            rosfinmon_data = rosfinmon_resp.json()
            rosfinmon_blocked = rosfinmon_data.get("blocked", False)
            if rosfinmon_blocked:
                changes.append(ReconcileChange(
                    change_type=ChangeType.ROSFINMON_HIT,
                    field="rosfinmon",
                    old_value="clear",
                    new_value=rosfinmon_data.get("reason", "blocked"),
                ))

    prev = req.previous_egrul_data

    # Compare key fields
    field_map = [
        ("ceo", ChangeType.DIRECTOR_CHANGED),
        ("address", ChangeType.ADDRESS_CHANGED),
        ("okved", ChangeType.OKVED_CHANGED),
        ("status", ChangeType.STATUS_CHANGED),
    ]
    for field, change_type in field_map:
        old_val = str(prev.get(field, ""))
        new_val = str(current.get(field, ""))
        if old_val and new_val and old_val != new_val:
            changes.append(ReconcileChange(
                change_type=change_type,
                field=field,
                old_value=old_val,
                new_value=new_val,
            ))

    significant = {ChangeType.ROSFINMON_HIT, ChangeType.DIRECTOR_CHANGED, ChangeType.STATUS_CHANGED}
    requires_review = any(c.change_type in significant for c in changes)

    return ReconcileResult(
        application_id=req.application_id,
        inn=req.inn,
        changes=changes,
        requires_review=requires_review,
        rosfinmon_blocked=rosfinmon_blocked,
    )
