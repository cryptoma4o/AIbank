from __future__ import annotations
import json
import logging
import httpx
from models.schemas import TraceRequest, TraceResult, UBONodeOut
from agent.prompts import SYSTEM_PROMPT, EXTRACT_FOUNDERS_PROMPT

log = logging.getLogger(__name__)

EGRUL_URL = "http://ext-egrul:8090"
LLM_GATEWAY_URL = "http://llm-gateway:8100"
DEFAULT_MODEL = "gemma-4"

async def trace_ubo(req: TraceRequest) -> TraceResult:
    async with httpx.AsyncClient(timeout=30.0) as client:
        egrul_resp = await client.get(f"{EGRUL_URL}/v1/egrul/{req.inn}")
        egrul_data = egrul_resp.json() if egrul_resp.status_code == 200 else {}

    egrul_text = json.dumps(egrul_data, ensure_ascii=False)
    nodes: list[UBONodeOut] = []
    model_used = DEFAULT_MODEL

    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": EXTRACT_FOUNDERS_PROMPT.format(egrul_text=egrul_text)},
    ]

    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            resp = await client.post(
                f"{LLM_GATEWAY_URL}/v1/chat/completions",
                json={"model": DEFAULT_MODEL, "messages": messages, "temperature": 0.1, "max_tokens": 1024},
            )
            resp.raise_for_status()
            content = resp.json()["choices"][0]["message"]["content"]
            founders_raw = json.loads(content)
            nodes = [UBONodeOut(**f) for f in founders_raw if isinstance(f, dict)]
    except Exception as exc:
        log.warning("LLM tracing failed for INN %s: %s — stub response", req.inn, exc)
        model_used = "stub"
        nodes = [UBONodeOut(name="[Stub Founder]", node_type="person", inn="", direct_stake=100.0)]

    ubos = [n for n in nodes if n.direct_stake >= 25.0]

    return TraceResult(
        application_id=req.application_id,
        inn=req.inn,
        nodes=nodes,
        ubos=ubos,
        depth_reached=1,
        model_used=model_used,
    )
