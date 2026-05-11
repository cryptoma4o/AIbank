"""10 готовых сценариев онбординга для AIbank E2E.

Каждый Scenario описывает заявителя (applicant), юрлицо и ожидаемый исход.
Используется в:
  - tests/e2e/api/test_applicant_flow.py — pytest параметризация
  - scripts/seed-staging-companies.py — ручной seed staging-сервера

Это единый источник правды. Новый сценарий — добавить сюда, и он сразу
покрывается тестами и доступен для seed.
"""

from __future__ import annotations

from dataclasses import dataclass, field

# Минимальный валидный PDF (~190 байт) для multipart-загрузки документов.
# Размер маленький — быстро летит по сети, минимизирует диск на staging.
MIN_PDF: bytes = (
    b"%PDF-1.4\n"
    b"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
    b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
    b"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\n"
    b"xref\n"
    b"0 4\n"
    b"0000000000 65535 f \n"
    b"0000000009 00000 n \n"
    b"0000000053 00000 n \n"
    b"0000000099 00000 n \n"
    b"trailer<</Size 4/Root 1 0 R>>\n"
    b"startxref\n"
    b"156\n"
    b"%%EOF\n"
)


@dataclass
class Scenario:
    sid: int
    name: str
    le_type: str  # IP | LLC | JSC | NPF
    okved: str
    le_inn: str  # ИНН юрлица (10 цифр) или физлица для ИП (12 цифр)
    le_ogrn: str
    applicant_inn: str  # 12 цифр (физлицо-заявитель)
    applicant_phone: str
    applicant_full_name: str
    documents: list[str]  # типы: passport|charter|protocol|extract|agreement
    risk_thresholds: dict[str, float]
    final_decision: dict[str, str] | None  # {decision, reviewer, reason} или None
    skip_documents: bool = False  # для draft: applicant+application и всё
    invalid_applicant_inn: bool = False  # граничный кейс валидации
    note: str = ""


SCENARIOS: list[Scenario] = [
    Scenario(
        sid=1,
        name="ООО «Ромашка Трейд»",
        le_type="LLC",
        okved="47.11",
        le_inn="7701000001",
        le_ogrn="1027700000001",
        applicant_inn="770100000011",
        applicant_phone="+79991000001",
        applicant_full_name="Иванов Иван Иванович",
        documents=["passport", "charter", "protocol", "extract"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved", "reviewer": "seed-bot", "reason": "Low risk, complete docs"},
        note="LLC happy path → approved",
    ),
    Scenario(
        sid=2,
        name="ООО «ТехноСофт»",
        le_type="LLC",
        okved="62.01",
        le_inn="7702000002",
        le_ogrn="1027700000002",
        applicant_inn="770200000022",
        applicant_phone="+79991000002",
        applicant_full_name="Соколова Анна Петровна",
        documents=["passport", "charter", "extract"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved", "reviewer": "seed-bot", "reason": "IT, low risk"},
        note="LLC IT-компания → approved",
    ),
    Scenario(
        sid=3,
        name="ИП Петров А.С.",
        le_type="IP",
        okved="96.02",
        le_inn="770300000033",
        le_ogrn="304770000000033",
        applicant_inn="770300000033",
        applicant_phone="+79991000003",
        applicant_full_name="Петров Алексей Сергеевич",
        documents=["passport", "extract"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved", "reviewer": "seed-bot", "reason": "Sole prop, services"},
        note="IP services → approved",
    ),
    Scenario(
        sid=4,
        name="ИП Сидорова Е.В.",
        le_type="IP",
        okved="47.91",
        le_inn="770400000044",
        le_ogrn="304770000000044",
        applicant_inn="770400000044",
        applicant_phone="+79991000004",
        applicant_full_name="Сидорова Елена Викторовна",
        documents=["passport"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,  # без human-decision → остаётся в running/manual_review
        note="IP partial docs → manual_review",
    ),
    Scenario(
        sid=5,
        name="АО «МедСтрой»",
        le_type="JSC",
        okved="41.20",
        le_inn="7705000005",
        le_ogrn="1027700000005",
        applicant_inn="770500000055",
        applicant_phone="+79991000005",
        applicant_full_name="Морозов Дмитрий Олегович",
        documents=["passport", "charter", "protocol"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved_with_edd", "reviewer": "seed-bot", "reason": "Construction, mid-size, EDD required"},
        note="JSC construction → approved_with_edd",
    ),
    Scenario(
        sid=6,
        name="АО «Энергоинвест»",
        le_type="JSC",
        okved="35.11",
        le_inn="7706000006",
        le_ogrn="1027700000006",
        applicant_inn="770600000066",
        applicant_phone="+79991000006",
        applicant_full_name="Волкова Татьяна Михайловна",
        documents=["passport", "charter", "protocol", "extract", "agreement"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,  # большой АО → manual_review
        note="JSC large public → manual_review",
    ),
    Scenario(
        sid=7,
        name="ООО «Глобал Трейд Корп»",
        le_type="LLC",
        okved="46.90",
        le_inn="7707000007",
        le_ogrn="1027700000007",
        applicant_inn="770700000077",
        applicant_phone="+79991000007",
        applicant_full_name="Кузнецов Роман Андреевич",
        documents=["passport", "charter"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "declined", "reviewer": "seed-bot", "reason": "115-FZ sanction match"},
        note="LLC high risk → declined (115-ФЗ match)",
    ),
    Scenario(
        sid=8,
        name="ООО «Кипр Холдинг»",
        le_type="LLC",
        okved="64.99",
        le_inn="7708000008",
        le_ogrn="1027700000008",
        applicant_inn="770800000088",
        applicant_phone="+79991000008",
        applicant_full_name="Семёнов Павел Викторович",
        documents=["passport", "charter"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "declined", "reviewer": "seed-bot", "reason": "Offshore UBO chain incomplete"},
        note="LLC offshore UBO → declined",
    ),
    Scenario(
        sid=9,
        name="ООО «Старт Айдиа»",
        le_type="LLC",
        okved="70.22",
        le_inn="7709000009",
        le_ogrn="1027700000009",
        applicant_inn="770900000099",
        applicant_phone="+79991000009",
        applicant_full_name="Орлова Мария Сергеевна",
        documents=[],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,
        skip_documents=True,
        note="LLC draft (no docs) → state=draft",
    ),
    Scenario(
        sid=10,
        name="ООО «Ошибка ИНН»",
        le_type="LLC",
        okved="47.11",
        le_inn="7710000010",
        le_ogrn="1027700000010",
        applicant_inn="ABC0000010",  # не цифры — упадёт на validate()
        applicant_phone="+79991000010",
        applicant_full_name="Тестов Эдж Кейсович",
        documents=[],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,
        skip_documents=True,
        invalid_applicant_inn=True,
        note="Boundary: invalid INN → 400 on POST /v1/applicants",
    ),
]


def by_sid(sid: int) -> Scenario:
    """Удобный getter — Scenario по sid."""
    for sc in SCENARIOS:
        if sc.sid == sid:
            return sc
    raise KeyError(f"scenario sid={sid} not found")
