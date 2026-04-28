"""Скрипт-генератор синтетического корпуса для reconciliation.

Сверка извлечённых из документов данных (`extracted_data`) с данными ЕГРЮЛ
(`egrul_data`). Используется агентом `agent-reconciliation`.

Запуск:
    cd ai/eval-harness/datasets/reconciliation
    python3 _generate.py
"""
from __future__ import annotations

import json
import random
from pathlib import Path

random.seed(20260426)

# ── Сэмплирующие словари ───────────────────────────────────────────────────────

LAST_NAMES = [
    "Иванов", "Петров", "Сидоров", "Козлов", "Смирнов",
    "Кузнецов", "Васильев", "Михайлов", "Новиков", "Фёдоров",
    "Морозов", "Волков", "Алексеев", "Лебедев", "Семёнов",
    "Егоров", "Павлов", "Никитин", "Соколов", "Соловьёв",
]
FIRST_NAMES_M = ["Александр", "Дмитрий", "Максим", "Сергей", "Андрей",
                 "Алексей", "Артём", "Илья", "Кирилл", "Михаил"]
MIDDLE_NAMES_M = ["Александрович", "Дмитриевич", "Сергеевич", "Андреевич",
                  "Алексеевич", "Иванович", "Михайлович", "Николаевич"]

COMPANY_NAMES = [
    "Ромашка", "Берёзка", "Алый Парус", "Новый Век", "Прогресс",
    "Гранит", "Вектор", "Сириус", "Альтаир", "Каскад",
    "Меридиан", "Импульс", "Атлант", "Восток", "Заря",
]

CITIES = [
    ("Москва", "г. Москва, ул. Тверская, д. {}, кв. {}"),
    ("Санкт-Петербург", "г. Санкт-Петербург, Невский пр-кт, д. {}, кв. {}"),
    ("Казань", "г. Казань, ул. Баумана, д. {}, кв. {}"),
    ("Новосибирск", "г. Новосибирск, ул. Ленина, д. {}, кв. {}"),
    ("Екатеринбург", "г. Екатеринбург, ул. Малышева, д. {}, кв. {}"),
]

OKVEDS = ["62.01", "47.11", "46.90", "68.20", "70.22", "82.99", "43.21", "49.41"]


# ── INN / OGRN ──────────────────────────────────────────────────────────────────


def _inn_legal_check(digits: list[int]) -> int:
    weights = [2, 4, 10, 3, 5, 9, 4, 6, 8]
    s = sum(d * w for d, w in zip(digits, weights))
    return s % 11 % 10


def gen_inn_legal(rnd: random.Random) -> str:
    head = [rnd.randint(1, 9)] + [rnd.randint(0, 9) for _ in range(8)]
    return "".join(str(d) for d in head + [_inn_legal_check(head)])


def gen_ogrn(rnd: random.Random) -> str:
    body = [1] + [rnd.randint(0, 9) for _ in range(11)]
    body_str = "".join(str(d) for d in body)
    check = int(body_str) % 11 % 10
    return body_str + str(check)


def gen_address(rnd: random.Random) -> str:
    _, tmpl = rnd.choice(CITIES)
    return tmpl.format(rnd.randint(1, 199), rnd.randint(1, 250))


def gen_director(rnd: random.Random) -> str:
    return f"{rnd.choice(LAST_NAMES)} {rnd.choice(FIRST_NAMES_M)} {rnd.choice(MIDDLE_NAMES_M)}"


def base_company(rnd: random.Random) -> dict:
    name = rnd.choice(COMPANY_NAMES)
    inn = gen_inn_legal(rnd)
    return {
        "short_name": f"ООО «{name}»",
        "inn": inn,
        "ogrn": gen_ogrn(rnd),
        "kpp": f"{inn[:4]}{rnd.randint(10, 99):02d}001",
        "legal_address": gen_address(rnd),
        "director_full_name": gen_director(rnd),
        "okved_main": rnd.choice(OKVEDS),
    }


# ── Генерация кейсов по подсетам ───────────────────────────────────────────────


def make_no_discrepancy_case(rnd: random.Random, idx: int) -> dict:
    company = base_company(rnd)
    extracted = dict(company)
    egrul = dict(company)
    return {
        "id": f"reconciliation-clean-{idx + 1:03d}",
        "agent": "reconciliation",
        "input": {
            "extracted_data": extracted,
            "egrul_data": egrul,
            "tenant_id": "tnt_demo",
        },
        "expected": {
            "matches": True,
            "discrepancies": [],
            "requires_review": False,
        },
        "tags": ["reconciliation", "no-discrepancies", "synthetic"],
        "description": f"Полное совпадение данных по {company['short_name']}",
    }


def _twist_address(addr: str, rnd: random.Random) -> str:
    # маленькая опечатка: "ул." -> "улица" или другой номер дома
    if "д. " in addr:
        parts = addr.split("д. ")
        tail = parts[1]
        num, rest = tail.split(",", 1)
        new_num = str(int(num) + rnd.choice([-1, 1, 2]))
        return f"{parts[0]}д. {new_num},{rest}"
    return addr + " (стр. 1)"


def make_minor_discrepancy_case(rnd: random.Random, idx: int) -> dict:
    company = base_company(rnd)
    extracted = dict(company)
    egrul = dict(company)

    discrepancies = []
    flavor = idx % 3
    if flavor == 0:
        # Расхождение в адресе (low)
        egrul["legal_address"] = _twist_address(company["legal_address"], rnd)
        discrepancies.append({
            "field": "legal_address",
            "extracted": extracted["legal_address"],
            "egrul": egrul["legal_address"],
            "severity": "low",
        })
    elif flavor == 1:
        # Расхождение в основном ОКВЭД (medium)
        new_okved = rnd.choice([o for o in OKVEDS if o != company["okved_main"]])
        egrul["okved_main"] = new_okved
        discrepancies.append({
            "field": "okved_main",
            "extracted": extracted["okved_main"],
            "egrul": new_okved,
            "severity": "medium",
        })
    else:
        # Расхождение в КПП (low)
        new_kpp = company["kpp"][:-1] + str((int(company["kpp"][-1]) + 1) % 10)
        egrul["kpp"] = new_kpp
        discrepancies.append({
            "field": "kpp",
            "extracted": extracted["kpp"],
            "egrul": new_kpp,
            "severity": "low",
        })

    return {
        "id": f"reconciliation-minor-{idx + 1:03d}",
        "agent": "reconciliation",
        "input": {
            "extracted_data": extracted,
            "egrul_data": egrul,
            "tenant_id": "tnt_demo",
        },
        "expected": {
            "matches": False,
            "discrepancies": discrepancies,
            "requires_review": False,
        },
        "tags": ["reconciliation", "minor-discrepancies", "synthetic"],
        "description": (
            f"Незначительное расхождение в '{discrepancies[0]['field']}' "
            f"для {company['short_name']}"
        ),
    }


def _twist_inn(inn: str) -> str:
    # одна цифра иначе — заведомо ломаем контрольный разряд (имитация опечатки)
    chars = list(inn)
    pos = 3
    chars[pos] = str((int(chars[pos]) + 1) % 10)
    return "".join(chars)


def make_major_discrepancy_case(rnd: random.Random, idx: int) -> dict:
    company = base_company(rnd)
    extracted = dict(company)
    egrul = dict(company)
    discrepancies = []

    flavor = idx % 3
    if flavor == 0:
        # Разный директор
        new_director = gen_director(rnd)
        while new_director == company["director_full_name"]:
            new_director = gen_director(rnd)
        egrul["director_full_name"] = new_director
        discrepancies.append({
            "field": "director_full_name",
            "extracted": extracted["director_full_name"],
            "egrul": new_director,
            "severity": "high",
        })
    elif flavor == 1:
        # ИНН с опечаткой
        bad_inn = _twist_inn(company["inn"])
        extracted["inn"] = bad_inn
        discrepancies.append({
            "field": "inn",
            "extracted": bad_inn,
            "egrul": company["inn"],
            "severity": "high",
        })
    else:
        # Отсутствует ОГРН в извлечённых данных
        extracted["ogrn"] = None
        discrepancies.append({
            "field": "ogrn",
            "extracted": None,
            "egrul": company["ogrn"],
            "severity": "high",
        })

    is_adversarial = (idx % 10 == 9)

    return {
        "id": f"reconciliation-major-{idx + 1:03d}",
        "agent": "reconciliation",
        "input": {
            "extracted_data": extracted,
            "egrul_data": egrul,
            "tenant_id": "tnt_demo",
        },
        "expected": {
            "matches": False,
            "discrepancies": discrepancies,
            "requires_review": True,
        },
        "tags": ["reconciliation", "major-discrepancies", "synthetic"]
                + (["adversarial"] if is_adversarial else []),
        "description": (
            f"Серьёзное расхождение в '{discrepancies[0]['field']}' "
            f"для {company['short_name']} — требуется ручная проверка"
        ),
    }


def main():
    rnd = random.Random(20260426)
    here = Path(__file__).parent

    cases = []
    cases += [make_no_discrepancy_case(rnd, i) for i in range(10)]
    cases += [make_minor_discrepancy_case(rnd, i) for i in range(10)]
    cases += [make_major_discrepancy_case(rnd, i) for i in range(10)]

    (here / "reconciliation-cases.json").write_text(
        json.dumps(cases, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"Wrote {len(cases)} reconciliation cases")


if __name__ == "__main__":
    main()
