from __future__ import annotations
import httpx
from .models import AuditEvent, EventType


class AuditClient:
    def __init__(self, base_url: str, tenant_id: str) -> None:
        self._base_url = base_url.rstrip("/")
        self._tenant_id = tenant_id
        self._http = httpx.AsyncClient(timeout=5.0)

    async def log(
        self,
        event_type: EventType,
        actor_id: str,
        actor_role: str,
        resource_type: str,
        resource_id: str,
        payload: dict | None = None,
    ) -> None:
        event = AuditEvent(
            tenant_id=self._tenant_id,
            event_type=event_type,
            actor_id=actor_id,
            actor_role=actor_role,
            resource_type=resource_type,
            resource_id=resource_id,
            payload=payload or {},
        )
        resp = await self._http.post(
            f"{self._base_url}/v1/events",
            content=event.model_dump_json(),
            headers={"Content-Type": "application/json"},
        )
        resp.raise_for_status()

    async def aclose(self) -> None:
        await self._http.aclose()

    async def __aenter__(self) -> "AuditClient":
        return self

    async def __aexit__(self, *_: object) -> None:
        await self.aclose()
