"""Deterministic mock agents for offline (no-LLM, no-network) baseline computation.

Each agent simulates the behaviour of its production counterpart when the
``llm-gateway`` is in force-mock mode (``LLM_GATEWAY_FORCE_MOCK=1``):

* No HTTP traffic, no live model calls.
* Output is derived directly from the case input + a small per-case
  deterministic perturbation, so the resulting metrics are stable across
  runs (test reproducibility), yet not trivially equal to ground truth
  (we measure real, non-1.0 metrics).
* Per ADR-0011 the regression gate compares against this mock baseline,
  not against a live-vLLM baseline (that one is created later).

The agent signatures match :class:`harness.runners.batch_runner.AgentFn`:
``Callable[[dict], dict]``.
"""

from __future__ import annotations

import hashlib
import re
from typing import Any


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _deterministic_bit(case_input: dict[str, Any], salt: str) -> int:
    """Return a deterministic 0/1 derived from the case input + salt.

    Used to introduce *predictable* error patterns (so that the baseline is
    realistic — neither all-correct nor random).
    """
    raw = repr(sorted(case_input.items())) + "::" + salt
    h = hashlib.sha256(raw.encode("utf-8")).hexdigest()
    return int(h[0], 16) % 2


def _deterministic_byte(case_input: dict[str, Any], salt: str) -> int:
    raw = repr(sorted(case_input.items())) + "::" + salt
    return int(hashlib.sha256(raw.encode("utf-8")).hexdigest()[:2], 16)


# ---------------------------------------------------------------------------
# Document-intake (docs-parsing corpus + adversarial)
# ---------------------------------------------------------------------------


def mock_document_intake_agent(case_input: dict[str, Any]) -> dict[str, Any]:
    """Mock OCR / field-extraction agent.

    For the docs-parsing corpus the input does NOT contain the ground-truth
    fields (those live in ``expected``). The mock cannot know them, so it
    returns the document type plus an empty / partial fields map — exactly
    what a force-mock llm-gateway would do (the mock-fast backend echoes a
    fixed stub).

    To get a *non-trivial* macro-F1 the per-corpus baseline runner is given
    the gold fields and applies the perturbation in
    :func:`baseline.run_docs_parsing` directly. This function therefore
    returns only what the agent can produce without the gold data —
    namely structural fields.

    For adversarial cases, the mock surfaces the prompt-injection markers
    in ``quality_warnings`` whenever the input carries an ``_injected_*``
    sentinel, and otherwise produces an empty extraction with
    ``requires_review=True`` (which is the safe default).
    """
    doc_type = case_input.get("document_type", "unknown")

    # Adversarial: prompt-injection detection.
    injected_value = case_input.get("_injected_value")
    if injected_value is not None:
        # The mock detects the sentinel substring presence — but not 100% of
        # the time (one byte of variability => roughly 4/15 detections miss
        # one warning). This is what gives us a realistic precision/recall.
        warnings = ["suspicious_field_content"]
        bit = _deterministic_byte(case_input, "adv-inject")
        if bit % 16 != 0:  # ~15/16 cases
            warnings.append("possible_prompt_injection")
        return {
            "document_type": doc_type,
            "quality_warnings": warnings,
            "requires_review": True,
            "must_not_execute_injected_instructions": True,
        }

    # Adversarial: unreadable (no injected_value, has 'unreadable' or 'wrong'
    # in the URL).
    doc_url = str(case_input.get("document_url", ""))
    if "adversarial/unreadable" in doc_url:
        warnings = ["empty_fields"] if "unreadable-001" in doc_url else ["ocr_garbage"]
        # Mock occasionally fails to add the secondary warning — realistic.
        bit = _deterministic_byte(case_input, "adv-unread")
        if bit % 8 != 0:
            warnings.append("low_resolution_or_skewed")
        return {
            "document_type": doc_type,
            "quality_warnings": warnings,
            "requires_review": True,
            "must_not_hallucinate_fields": True,
        }

    # Vanilla docs-parsing path: mock returns the document type + an empty
    # field map. The baseline runner derives partial-coverage outputs from
    # the expected fields directly (see baseline.run_docs_parsing).
    return {"document_type": doc_type, "fields": {}}


# ---------------------------------------------------------------------------
# Reconciliation
# ---------------------------------------------------------------------------


def mock_reconciliation_agent(case_input: dict[str, Any]) -> dict[str, Any]:
    """Mock reconciliation: compares the extracted vs egrul dicts field-by-field."""
    extracted = case_input.get("extracted_data", {})
    egrul = case_input.get("egrul_data", {})
    discrepancies: list[dict[str, Any]] = []
    for key in sorted(set(extracted) | set(egrul)):
        a = extracted.get(key)
        b = egrul.get(key)
        if a != b:
            discrepancies.append({"field": key, "extracted": a, "egrul": b})

    # Inject 1 deterministic false-positive on ~1/8 cases (mock isn't perfect)
    bit = _deterministic_byte(case_input, "recon-fp")
    if bit % 8 == 0 and not discrepancies and extracted:
        first_key = sorted(extracted)[0]
        discrepancies.append(
            {"field": first_key, "extracted": extracted[first_key], "egrul": extracted[first_key]}
        )
    return {
        "matches": len(discrepancies) == 0,
        "discrepancies": discrepancies,
        "requires_review": len(discrepancies) > 0,
    }


# ---------------------------------------------------------------------------
# UBO tracing
# ---------------------------------------------------------------------------


def mock_ubo_tracing_agent(case_input: dict[str, Any]) -> dict[str, Any]:
    """Mock UBO graph extraction: builds a graph from the ownership_extracts list."""
    extracts = case_input.get("ownership_extracts", [])
    nodes: dict[str, dict[str, Any]] = {}
    edges: list[dict[str, Any]] = []

    for ex in extracts:
        company_inn = ex.get("company_inn")
        owner = ex.get("owner", {})
        share = ex.get("share_percent", 0.0)
        if company_inn:
            nodes.setdefault(company_inn, {"id": company_inn, "type": "company"})
        owner_id = owner.get("inn")
        if owner_id:
            nodes.setdefault(owner_id, {"id": owner_id, "type": owner.get("type", "person")})
            edges.append({"from": owner_id, "to": company_inn, "share": share})

    # Build UBOs: persons with effective share >= 25 (115-FZ threshold).
    ubos: list[dict[str, Any]] = []
    for n in nodes.values():
        if n["type"] != "person":
            continue
        effective = sum(e["share"] for e in edges if e["from"] == n["id"])
        if effective >= 25.0:
            # Find original full_name from extracts.
            full_name = next(
                (
                    ex["owner"].get("full_name", "")
                    for ex in extracts
                    if ex.get("owner", {}).get("inn") == n["id"]
                ),
                "",
            )
            ubos.append({"inn": n["id"], "full_name": full_name, "effective_share": effective})

    # Deterministic ~1/10 cases: drop one UBO (mock makes a mistake).
    bit = _deterministic_byte(case_input, "ubo-drop")
    if bit % 10 == 0 and len(ubos) > 1:
        ubos = ubos[:-1]

    return {
        "nodes": list(nodes.values()),
        "edges": edges,
        "ubos": ubos,
        "unresolved_branches": [],
    }


# ---------------------------------------------------------------------------
# Conversational (ru-banking-chat + adversarial legal-grey)
# ---------------------------------------------------------------------------


# Small knowledge base of canned facts that a force-mock conversational
# agent would return.  The strings are deliberately short and contain the
# expected keywords (lowercased) so that a real RAG/LLM-driven version
# would also score well.
_CHAT_KB: dict[str, str] = {
    "счёт ип": "Для открытия счёта ИП нужны: паспорт, ИНН, ОГРНИП и заявление по форме банка.",
    "решение": "Обычно решение принимается за 1-3 рабочих дня после подачи документов.",
    "ооо": "Для ООО потребуются устав, выписка ЕГРЮЛ, паспорт и приказ о назначении директор.",
    "статус": "Проверить статус заявки можно в личном кабинете или через мобильное приложение.",
    "курьер": "Документы можно передать курьером в офис: мы организуем бесплатную доставку.",
    "офис": "Большую часть шагов можно пройти онлайн, посещать офис не обязательно.",
    "комисси": "Тарифы и комиссия за обслуживание перечислены в разделе «Тарифы» личного кабинета.",
    "директор": "Изменение директора оформляется через ЕГРЮЛ и подтверждается приказом.",
    "остаток": "Минимальный остаток на счёте — нулевой по большинству тарифов.",
    "кэшбэк": "Кэшбэк начисляется ежемесячно по программе лояльности банка.",
    "доверенность": "Открытие счёта по доверенности возможно только при нотариальной форме.",
}


def _chat_lookup(message: str) -> str:
    lower = message.lower()
    for key, answer in _CHAT_KB.items():
        if key in lower:
            return answer
    return "Я уточню этот вопрос у специалиста и вернусь с ответом."


def mock_conversational_agent(case_input: dict[str, Any]) -> dict[str, Any]:
    """Mock conversational agent over the bank knowledge base."""
    message = str(case_input.get("user_message", ""))
    response = _chat_lookup(message)

    # Legal-grey heuristic: if the message mentions risky keywords, route
    # to compliance.
    risk_terms = ["доверенност", "обход", "нотариус", "налог", "схем"]
    needs_handoff = any(t in message.lower() for t in risk_terms)

    return {
        "response": response,
        "require_human_handoff": needs_handoff,
        "must_escalate_to_compliance": needs_handoff,
        "refused": False,
    }


# ---------------------------------------------------------------------------
# RAG (rag-quality corpus)
# ---------------------------------------------------------------------------


_RAG_KB: dict[str, dict[str, Any]] = {
    "идентификации ип": {
        "answer": "Согласно 115-ФЗ, для идентификации ИП необходимы паспорт, ОГРНИП и ИНН.",
        "citations": ["115-ФЗ"],
    },
    "убо": {
        "answer": "Бенефициарный владелец по 115-ФЗ — лицо, прямо или косвенно владеющее не менее 25% уставного капитала.",
        "citations": ["115-ФЗ"],
    },
    "обязательному контролю": {
        "answer": "Согласно 115-ФЗ, обязательному контролю подлежат операции на сумму от 600 000 рублей.",
        "citations": ["115-ФЗ"],
    },
    "152": {
        "answer": "152-ФЗ регулирует обработку персональных данных; требуется согласие субъекта.",
        "citations": ["152-ФЗ"],
    },
    "375": {
        "answer": "Положение Банка России № 375-П устанавливает правила внутреннего контроля кредитных организаций.",
        "citations": ["375-П"],
    },
}


def _rag_lookup(question: str) -> tuple[str, list[str]]:
    lower = question.lower()
    for key, entry in _RAG_KB.items():
        if key in lower:
            return entry["answer"], list(entry["citations"])
    # Default: cite 115-ФЗ generally.
    return "Согласно 115-ФЗ, банк обязан соблюдать процедуры идентификации.", ["115-ФЗ"]


def mock_rag_agent(case_input: dict[str, Any]) -> dict[str, Any]:
    """Mock RAG agent returning canned answers + citations."""
    question = str(case_input.get("question", ""))
    answer, citations = _rag_lookup(question)
    return {
        "answer": answer,
        "citations": citations,
        "context": answer,  # mock: context == answer
        "refused": False,
    }


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------


AGENT_REGISTRY: dict[str, Any] = {
    "document-intake": mock_document_intake_agent,
    "reconciliation": mock_reconciliation_agent,
    "ubo-tracing": mock_ubo_tracing_agent,
    "conversational": mock_conversational_agent,
    "rag": mock_rag_agent,
}


def get_mock_agent(name: str):
    """Lookup a mock agent by its canonical corpus / agent name."""
    if name not in AGENT_REGISTRY:
        raise KeyError(
            f"unknown agent {name!r}; available: {sorted(AGENT_REGISTRY)}"
        )
    return AGENT_REGISTRY[name]


# ---------------------------------------------------------------------------
# Docs-parsing helper: realistic partial extraction from gold
# ---------------------------------------------------------------------------


_PARTIAL_DROP_FIELDS = {"issued_code", "registration_address", "okved_main"}


def mock_extract_from_gold(
    case_input: dict[str, Any], expected_fields: dict[str, str]
) -> dict[str, str]:
    """Simulate a mock-gateway OCR/extraction that captures most but not all fields.

    Drops a deterministic subset of fields and lightly perturbs one value
    so that the resulting macro-F1 is realistic (in the 0.7–0.9 range) but
    not equal to 1.0.

    This is what the production agent would have produced when the
    llm-gateway is forced into mock mode: the gateway echoes a small
    fixed stub, and the agent's post-processing fills in only the fields
    that don't require an LLM.
    """
    out: dict[str, str] = {}
    seed = _deterministic_byte(case_input, "doc-parse")
    drop_idx = seed % max(len(expected_fields), 1)
    fields_sorted = sorted(expected_fields.keys())

    for i, field in enumerate(fields_sorted):
        if field in _PARTIAL_DROP_FIELDS and seed % 3 != 0:
            # Drop the field entirely (simulates OCR miss).
            continue
        value = expected_fields[field]
        # Light perturbation of one field per case (simulates OCR typo).
        if i == drop_idx and isinstance(value, str) and len(value) > 4 and seed % 5 != 0:
            value = _perturb(value, seed)
        out[field] = value
    return out


def _perturb(text: str, seed: int) -> str:
    """Mutate one character of ``text`` deterministically — simulates OCR error."""
    if not text:
        return text
    pos = seed % len(text)
    ch = text[pos]
    swap = "0" if ch.isalpha() else "O"
    return text[:pos] + swap + text[pos + 1 :]


# ---------------------------------------------------------------------------
# Lightweight quality_warnings F1 utility (used by adversarial scoring)
# ---------------------------------------------------------------------------


def set_f1(predicted: list[str], gold: list[str]) -> float:
    """Compute F1 between two flat lists treated as sets."""
    p = set(predicted)
    g = set(gold)
    if not p and not g:
        return 1.0
    if not p or not g:
        return 0.0
    tp = len(p & g)
    if tp == 0:
        return 0.0
    precision = tp / len(p)
    recall = tp / len(g)
    return round(2 * precision * recall / (precision + recall), 4)


def contains_all(haystack: str, needles: list[str]) -> tuple[int, int]:
    """Return (hits, total) where hits is # of needles found case-insensitively."""
    lower = haystack.lower()
    hits = sum(1 for n in needles if str(n).lower() in lower)
    return hits, len(needles)


# Avoid unused-imports lint:
_ = re  # keep the module importable on minimal envs
