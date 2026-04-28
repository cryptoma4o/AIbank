"""Скрипт-генератор синтетического корпуса для ubo-graphs.

Прослеживание цепочек владения (UBO — Ultimate Beneficial Owner) для агента
`agent-ubo-tracing`. Каждый кейс содержит «выписки» о владельцах
(`ownership_extracts`) и ожидаемый результат: список UBO, граф (узлы/рёбра)
и нерешённые ветки (например, иностранный филиал).

Запуск:
    cd ai/eval-harness/datasets/ubo-graphs
    python3 _generate.py
"""
from __future__ import annotations

import json
import random
from pathlib import Path

random.seed(20260426)

LAST_NAMES = [
    "Иванов", "Петров", "Сидоров", "Козлов", "Смирнов",
    "Кузнецов", "Васильев", "Михайлов", "Новиков", "Фёдоров",
]
FIRST_NAMES_M = ["Александр", "Дмитрий", "Максим", "Сергей", "Андрей", "Алексей"]
MIDDLE_NAMES_M = ["Александрович", "Дмитриевич", "Сергеевич", "Андреевич",
                  "Алексеевич", "Иванович", "Михайлович", "Николаевич"]

COMPANY_NAMES = [
    "Ромашка", "Берёзка", "Прогресс", "Гранит", "Вектор",
    "Сириус", "Альтаир", "Каскад", "Меридиан", "Импульс",
]


def _inn_legal_check(digits: list[int]) -> int:
    weights = [2, 4, 10, 3, 5, 9, 4, 6, 8]
    return sum(d * w for d, w in zip(digits, weights)) % 11 % 10


def gen_inn_legal(rnd: random.Random) -> str:
    head = [rnd.randint(1, 9)] + [rnd.randint(0, 9) for _ in range(8)]
    return "".join(str(d) for d in head + [_inn_legal_check(head)])


def _inn_person_check(digits: list[int], weights: list[int]) -> int:
    return sum(d * w for d, w in zip(digits, weights)) % 11 % 10


def gen_inn_person(rnd: random.Random) -> str:
    head = [rnd.randint(1, 9)] + [rnd.randint(0, 9) for _ in range(9)]
    n11 = _inn_person_check(head, [7, 2, 4, 10, 3, 5, 9, 4, 6, 8])
    n12 = _inn_person_check(head + [n11], [3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8])
    return "".join(str(d) for d in head + [n11, n12])


def gen_person(rnd: random.Random) -> dict:
    full_name = f"{rnd.choice(LAST_NAMES)} {rnd.choice(FIRST_NAMES_M)} {rnd.choice(MIDDLE_NAMES_M)}"
    return {
        "type": "person",
        "full_name": full_name,
        "inn": gen_inn_person(rnd),
    }


def gen_company(rnd: random.Random) -> dict:
    name = rnd.choice(COMPANY_NAMES)
    return {
        "type": "company",
        "short_name": f"ООО «{name}»",
        "inn": gen_inn_legal(rnd),
    }


# ── Подсеты ────────────────────────────────────────────────────────────────────


def make_trivial_case(rnd: random.Random, idx: int) -> dict:
    """1 учредитель-физлицо со 100% долей → он же UBO."""
    root = gen_company(rnd)
    person = gen_person(rnd)
    extracts = [{
        "company_inn": root["inn"],
        "owner": person,
        "share_percent": 100.0,
    }]
    return {
        "id": f"ubo-graphs-trivial-{idx + 1:03d}",
        "agent": "ubo-tracing",
        "input": {
            "root_inn": root["inn"],
            "root_short_name": root["short_name"],
            "ownership_extracts": extracts,
            "tenant_id": "tnt_demo",
        },
        "expected": {
            "nodes": [
                {"id": root["inn"], "type": "company"},
                {"id": person["inn"], "type": "person"},
            ],
            "edges": [{"from": person["inn"], "to": root["inn"], "share": 100.0}],
            "ubos": [
                {"inn": person["inn"], "full_name": person["full_name"], "effective_share": 100.0}
            ],
            "unresolved_branches": [],
        },
        "tags": ["ubo-graphs", "trivial", "synthetic"],
        "description": f"Тривиальный UBO: 1 физлицо 100% владеет {root['short_name']}",
    }


def make_split_case(rnd: random.Random, idx: int) -> dict:
    """2-3 учредителя по 33-50% — все ≥25% становятся UBO."""
    root = gen_company(rnd)
    n_owners = rnd.choice([2, 2, 3])
    if n_owners == 2:
        shares = [50.0, 50.0]
    else:
        shares = [34.0, 33.0, 33.0]
    persons = [gen_person(rnd) for _ in range(n_owners)]
    extracts = [
        {"company_inn": root["inn"], "owner": p, "share_percent": s}
        for p, s in zip(persons, shares)
    ]
    nodes = [{"id": root["inn"], "type": "company"}] + [
        {"id": p["inn"], "type": "person"} for p in persons
    ]
    edges = [
        {"from": p["inn"], "to": root["inn"], "share": s}
        for p, s in zip(persons, shares)
    ]
    ubos = [
        {"inn": p["inn"], "full_name": p["full_name"], "effective_share": s}
        for p, s in zip(persons, shares) if s >= 25.0
    ]
    return {
        "id": f"ubo-graphs-split-{idx + 1:03d}",
        "agent": "ubo-tracing",
        "input": {
            "root_inn": root["inn"],
            "root_short_name": root["short_name"],
            "ownership_extracts": extracts,
            "tenant_id": "tnt_demo",
        },
        "expected": {
            "nodes": nodes,
            "edges": edges,
            "ubos": ubos,
            "unresolved_branches": [],
        },
        "tags": ["ubo-graphs", "split", "synthetic"],
        "description": f"Разделённое владение {root['short_name']} ({n_owners} физлица)",
    }


def make_nested_case(rnd: random.Random, idx: int) -> dict:
    """ООО владеет ООО, которым владеет физлицо: эффективная доля = произведение."""
    root = gen_company(rnd)
    middle = gen_company(rnd)
    person = gen_person(rnd)
    s1 = rnd.choice([100.0, 75.0, 60.0])  # доля middle в root
    s2 = 100.0  # доля person в middle
    extracts = [
        {"company_inn": root["inn"], "owner": middle, "share_percent": s1},
        {"company_inn": middle["inn"], "owner": person, "share_percent": s2},
    ]
    eff = round(s1 * s2 / 100.0, 2)
    nodes = [
        {"id": root["inn"], "type": "company"},
        {"id": middle["inn"], "type": "company"},
        {"id": person["inn"], "type": "person"},
    ]
    edges = [
        {"from": middle["inn"], "to": root["inn"], "share": s1},
        {"from": person["inn"], "to": middle["inn"], "share": s2},
    ]
    ubos = [
        {"inn": person["inn"], "full_name": person["full_name"], "effective_share": eff}
    ] if eff >= 25.0 else []
    return {
        "id": f"ubo-graphs-nested-{idx + 1:03d}",
        "agent": "ubo-tracing",
        "input": {
            "root_inn": root["inn"],
            "root_short_name": root["short_name"],
            "ownership_extracts": extracts,
            "tenant_id": "tnt_demo",
        },
        "expected": {
            "nodes": nodes,
            "edges": edges,
            "ubos": ubos,
            "unresolved_branches": [],
        },
        "tags": ["ubo-graphs", "nested", "synthetic"],
        "description": (
            f"Цепочка: {person['full_name']} → {middle['short_name']} "
            f"({s2}%) → {root['short_name']} ({s1}%)"
        ),
    }


def make_edge_case(rnd: random.Random, idx: int) -> dict:
    """
    edge cases:
      0 → иностранный филиал (нерезолвится)
      1 → государственное участие (не считается UBO)
      2 → технический владелец <25% (не UBO)
      3 → смесь: государство + физлицо ниже 25% + иностранный
      4 → анонимный траст без бенефициара
    """
    root = gen_company(rnd)
    flavor = idx % 5

    if flavor == 0:
        foreign = {
            "type": "foreign_company",
            "short_name": "Cyprus Holdings Ltd.",
            "jurisdiction": "CY",
        }
        extracts = [{"company_inn": root["inn"], "owner": foreign, "share_percent": 100.0}]
        expected = {
            "nodes": [
                {"id": root["inn"], "type": "company"},
                {"id": "foreign:CY:Cyprus Holdings Ltd.", "type": "foreign_company"},
            ],
            "edges": [
                {"from": "foreign:CY:Cyprus Holdings Ltd.", "to": root["inn"], "share": 100.0}
            ],
            "ubos": [],
            "unresolved_branches": [
                {"reason": "foreign_jurisdiction", "node": "foreign:CY:Cyprus Holdings Ltd."}
            ],
        }
        desc = f"Иностранный учредитель (CY) у {root['short_name']} — не резолвится"

    elif flavor == 1:
        state = {"type": "state", "name": "Российская Федерация"}
        extracts = [{"company_inn": root["inn"], "owner": state, "share_percent": 100.0}]
        expected = {
            "nodes": [
                {"id": root["inn"], "type": "company"},
                {"id": "state:RU", "type": "state"},
            ],
            "edges": [{"from": "state:RU", "to": root["inn"], "share": 100.0}],
            "ubos": [],
            "unresolved_branches": [],
        }
        desc = f"100% государственное участие в {root['short_name']} — UBO нет"

    elif flavor == 2:
        # 80% — крупный, 20% — мелкий, ниже порога
        big = gen_person(rnd)
        small = gen_person(rnd)
        extracts = [
            {"company_inn": root["inn"], "owner": big, "share_percent": 80.0},
            {"company_inn": root["inn"], "owner": small, "share_percent": 20.0},
        ]
        expected = {
            "nodes": [
                {"id": root["inn"], "type": "company"},
                {"id": big["inn"], "type": "person"},
                {"id": small["inn"], "type": "person"},
            ],
            "edges": [
                {"from": big["inn"], "to": root["inn"], "share": 80.0},
                {"from": small["inn"], "to": root["inn"], "share": 20.0},
            ],
            "ubos": [
                {"inn": big["inn"], "full_name": big["full_name"], "effective_share": 80.0}
            ],
            "unresolved_branches": [],
        }
        desc = f"Технический владелец 20% у {root['short_name']} — не UBO"

    elif flavor == 3:
        state = {"type": "state", "name": "Российская Федерация"}
        small = gen_person(rnd)
        foreign = {"type": "foreign_company", "short_name": "BVI Trust Inc.", "jurisdiction": "VG"}
        extracts = [
            {"company_inn": root["inn"], "owner": state, "share_percent": 51.0},
            {"company_inn": root["inn"], "owner": small, "share_percent": 19.0},
            {"company_inn": root["inn"], "owner": foreign, "share_percent": 30.0},
        ]
        expected = {
            "nodes": [
                {"id": root["inn"], "type": "company"},
                {"id": "state:RU", "type": "state"},
                {"id": small["inn"], "type": "person"},
                {"id": "foreign:VG:BVI Trust Inc.", "type": "foreign_company"},
            ],
            "edges": [
                {"from": "state:RU", "to": root["inn"], "share": 51.0},
                {"from": small["inn"], "to": root["inn"], "share": 19.0},
                {"from": "foreign:VG:BVI Trust Inc.", "to": root["inn"], "share": 30.0},
            ],
            "ubos": [],
            "unresolved_branches": [
                {"reason": "foreign_jurisdiction", "node": "foreign:VG:BVI Trust Inc."}
            ],
        }
        desc = f"Смесь: государство 51% + 30% офшор + 19% физлицо в {root['short_name']}"

    else:
        trust = {"type": "trust", "name": "Частный траст без указания бенефициаров"}
        extracts = [{"company_inn": root["inn"], "owner": trust, "share_percent": 100.0}]
        expected = {
            "nodes": [
                {"id": root["inn"], "type": "company"},
                {"id": "trust:anonymous", "type": "trust"},
            ],
            "edges": [{"from": "trust:anonymous", "to": root["inn"], "share": 100.0}],
            "ubos": [],
            "unresolved_branches": [
                {"reason": "anonymous_trust", "node": "trust:anonymous"}
            ],
        }
        desc = f"Анонимный траст 100% у {root['short_name']} — бенефициар не известен"

    is_adversarial = (idx % 5 == 4) or (idx % 5 == 0)

    return {
        "id": f"ubo-graphs-edge-{idx + 1:03d}",
        "agent": "ubo-tracing",
        "input": {
            "root_inn": root["inn"],
            "root_short_name": root["short_name"],
            "ownership_extracts": extracts,
            "tenant_id": "tnt_demo",
        },
        "expected": expected,
        "tags": ["ubo-graphs", "edge", "synthetic"]
                + (["adversarial"] if is_adversarial else []),
        "description": desc,
    }


def main():
    rnd = random.Random(20260426)
    here = Path(__file__).parent

    cases = []
    cases += [make_trivial_case(rnd, i) for i in range(5)]
    cases += [make_split_case(rnd, i) for i in range(5)]
    cases += [make_nested_case(rnd, i) for i in range(5)]
    cases += [make_edge_case(rnd, i) for i in range(5)]

    (here / "ubo-cases.json").write_text(
        json.dumps(cases, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"Wrote {len(cases)} ubo-graphs cases")


if __name__ == "__main__":
    main()
