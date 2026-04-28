"""Unit tests for extractor: LLM happy-path, fallbacks, OCR-only path."""
from __future__ import annotations

import base64

import pytest

from agent import extractor as extractor_mod
from agent.extractor import extract_fields
from agent.gateway_client import GatewayError
from models.schemas import DocumentType

from tests.conftest import FakeGateway


def _b64(s: str) -> str:
    return base64.b64encode(s.encode("utf-8")).decode("ascii")


@pytest.mark.asyncio
async def test_role_constant_matches_gateway_yaml() -> None:
    assert extractor_mod.ROLE == "document-vision"


@pytest.mark.asyncio
async def test_llm_structured_json_yields_fields() -> None:
    fake = FakeGateway(
        content={
            "surname": "Иванов",
            "name": "Иван",
            "patronymic": "Иванович",
            "birth_date": "1985-04-12",
            "series": "4509",
            "number": "123456",
        }
    )
    text_payload = "ПАСПОРТ Иванов Иван Иванович 12.04.1985 4509 123456"
    result = await extract_fields(
        document_id="doc-1",
        document_type=DocumentType.PASSPORT,
        content_base64=_b64(text_payload),
        content_url=None,
        tenant_id="bank-alpha",
        client=fake,
    )

    assert result.document_id == "doc-1"
    assert result.model_used == "mock-fast"
    fields = {f.name: f for f in result.fields}
    assert fields["surname"].value == "Иванов"
    assert fields["birth_date"].value == "1985-04-12"
    # All LLM fields land at high confidence.
    assert all(f.confidence >= 0.8 for f in result.fields)
    # Gateway was called with the right role + tenant.
    assert fake.calls and fake.calls[0]["role"] == "document-vision"
    assert fake.calls[0]["tenant_id"] == "bank-alpha"
    assert result.gateway_metadata["resolved_model"] == "mock-fast"


@pytest.mark.asyncio
async def test_malformed_llm_response_falls_back_to_rules() -> None:
    fake = FakeGateway(content="не-JSON ответ ниоткуда")
    egrul_text = "ОГРН 1027700132195 ИНН 7707083893 КПП 770701001"
    result = await extract_fields(
        document_id="doc-2",
        document_type=DocumentType.EGRUL,
        content_base64=_b64(egrul_text),
        content_url=None,
        tenant_id="bank-alpha",
        client=fake,
    )

    # Rule-based regex picks INN and OGRN out of the OCR text.
    field_map = {f.name: f for f in result.fields}
    assert field_map["inn"].value == "7707083893"
    assert field_map["ogrn"].value == "1027700132195"
    # Falls back → low confidence.
    assert all(f.confidence < 0.6 for f in result.fields)
    assert result.model_used == "rule-based"
    assert "llm_error" in result.gateway_metadata


@pytest.mark.asyncio
async def test_llm_returns_fenced_json_block_is_parsed() -> None:
    fake = FakeGateway(content="```json\n{\"inn\": \"7707083893\"}\n```")
    result = await extract_fields(
        document_id="doc-3",
        document_type=DocumentType.INN_CERTIFICATE,
        content_base64=_b64("ИНН 7707083893"),
        content_url=None,
        tenant_id="bank-alpha",
        client=fake,
    )
    field_map = {f.name: f for f in result.fields}
    assert field_map["inn"].value == "7707083893"
    assert field_map["inn"].confidence >= 0.8


@pytest.mark.asyncio
async def test_gateway_failure_returns_rule_based_result() -> None:
    fake = FakeGateway(raise_exc=GatewayError("connection refused"))
    text = "ИНН 7707083893 ОГРН 1027700132195"
    result = await extract_fields(
        document_id="doc-4",
        document_type=DocumentType.EGRUL,
        content_base64=_b64(text),
        content_url=None,
        tenant_id="bank-alpha",
        client=fake,
    )
    assert {f.name for f in result.fields} >= {"inn", "ogrn"}
    assert result.model_used == "rule-based"
    assert "llm_error" in result.gateway_metadata


@pytest.mark.asyncio
async def test_ocr_only_path_with_no_tenant_skips_llm() -> None:
    """When tenant_id missing we must not hit the gateway, only rules run."""
    fake = FakeGateway(content={"inn": "must-not-appear"})
    text = "ИНН 7707083893 БИК 044525225"
    result = await extract_fields(
        document_id="doc-5",
        document_type=DocumentType.BANK_STATEMENT,
        content_base64=_b64(text),
        content_url=None,
        tenant_id="",          # explicit empty
        client=fake,
    )
    assert fake.calls == []      # gateway NOT called
    field_map = {f.name: f for f in result.fields}
    assert field_map["inn"].value == "7707083893"
    assert field_map["bik"].value == "044525225"
    assert result.model_used == "rule-based"
    assert result.gateway_metadata.get("llm_skipped") == "missing_tenant"


@pytest.mark.asyncio
async def test_empty_llm_payload_with_no_rule_match_returns_no_fields() -> None:
    fake = FakeGateway(content={})
    result = await extract_fields(
        document_id="doc-6",
        document_type=DocumentType.CHARTER,
        content_base64=_b64("уставные документы без распознаваемых полей"),
        content_url=None,
        tenant_id="bank-alpha",
        client=fake,
    )
    # LLM said {} → no fields; rule-based for charter without ИНН/ОГРН → also nothing.
    assert result.fields == []
    assert result.processing_ms >= 0


@pytest.mark.asyncio
async def test_llm_filters_underscored_keys() -> None:
    fake = FakeGateway(content={"_meta": "skip", "inn": "7707083893"})
    result = await extract_fields(
        document_id="doc-7",
        document_type=DocumentType.INN_CERTIFICATE,
        content_base64=_b64("..."),
        content_url=None,
        tenant_id="bank-alpha",
        client=fake,
    )
    names = {f.name for f in result.fields}
    assert "inn" in names
    assert "_meta" not in names
