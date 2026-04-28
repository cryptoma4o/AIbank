"""Скрипт-генератор синтетического корпуса для docs-parsing.

Используется один раз при создании корпуса. Не выполняется в CI:
кейсы хранятся в Git как зафиксированные .json файлы (см. README.md).

Запуск:
    cd ai/eval-harness/datasets/docs-parsing
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
    "Захаров", "Борисов", "Яковлев", "Григорьев", "Романов",
]
FIRST_NAMES_M = ["Александр", "Дмитрий", "Максим", "Сергей", "Андрей",
                 "Алексей", "Артём", "Илья", "Кирилл", "Михаил",
                 "Никита", "Матвей", "Роман", "Егор", "Арсений"]
FIRST_NAMES_F = ["Анна", "Мария", "Елена", "Ольга", "Наталья",
                 "Ирина", "Татьяна", "Светлана", "Юлия", "Екатерина"]
MIDDLE_NAMES_M = ["Александрович", "Дмитриевич", "Сергеевич", "Андреевич",
                  "Алексеевич", "Иванович", "Михайлович", "Николаевич",
                  "Викторович", "Юрьевич"]
MIDDLE_NAMES_F = ["Александровна", "Дмитриевна", "Сергеевна", "Андреевна",
                  "Алексеевна", "Ивановна", "Михайловна", "Николаевна",
                  "Викторовна", "Юрьевна"]

ISSUERS = [
    "ОТДЕЛ УФМС РОССИИ ПО Г. МОСКВЕ",
    "ОТДЕЛ УФМС РОССИИ ПО Г. САНКТ-ПЕТЕРБУРГУ",
    "ОВД ПО Г. КАЗАНЬ",
    "ГУ МВД РОССИИ ПО МОСКОВСКОЙ ОБЛ.",
    "ОТДЕЛ ПО Г. НОВОСИБИРСКУ",
    "ОТДЕЛЕНИЕМ УФМС РОССИИ ПО ЕКАТЕРИНБУРГУ",
    "ГУ МВД РОССИИ ПО НИЖЕГОРОДСКОЙ ОБЛ.",
]

CITIES = [
    ("Москва", "г. Москва, ул. Тверская, д. {}, кв. {}"),
    ("Санкт-Петербург", "г. Санкт-Петербург, Невский пр-кт, д. {}, кв. {}"),
    ("Казань", "г. Казань, ул. Баумана, д. {}, кв. {}"),
    ("Новосибирск", "г. Новосибирск, ул. Ленина, д. {}, кв. {}"),
    ("Екатеринбург", "г. Екатеринбург, ул. Малышева, д. {}, кв. {}"),
    ("Нижний Новгород", "г. Нижний Новгород, ул. Большая Покровская, д. {}, кв. {}"),
    ("Ростов-на-Дону", "г. Ростов-на-Дону, пр. Соколова, д. {}, кв. {}"),
]

COMPANY_NAMES = [
    "Ромашка", "Берёзка", "Алый Парус", "Новый Век", "Прогресс",
    "Гранит", "Вектор", "Сириус", "Альтаир", "Каскад",
    "Меридиан", "Импульс", "Атлант", "Восток", "Заря",
    "Радуга", "Магистраль", "Фортуна", "Стрела", "Север",
    "Юг", "Запад", "Континент", "Лидер", "Глобус",
]

OKVEDS = [
    ("62.01", "Разработка компьютерного программного обеспечения"),
    ("47.11", "Торговля розничная преимущественно пищевыми продуктами"),
    ("46.90", "Торговля оптовая неспециализированная"),
    ("68.20", "Аренда и управление собственным недвижимым имуществом"),
    ("70.22", "Консультирование по вопросам коммерческой деятельности"),
    ("82.99", "Деятельность по предоставлению прочих вспомогательных услуг"),
    ("43.21", "Производство электромонтажных работ"),
    ("49.41", "Деятельность автомобильного грузового транспорта"),
    ("45.20", "Техническое обслуживание и ремонт автотранспортных средств"),
    ("96.01", "Стирка и химическая чистка текстильных и меховых изделий"),
]


# ── INN / OGRN с корректными контрольными разрядами ────────────────────────────


def _inn_legal_check(digits: list[int]) -> int:
    weights = [2, 4, 10, 3, 5, 9, 4, 6, 8]
    s = sum(d * w for d, w in zip(digits, weights))
    return s % 11 % 10


def gen_inn_legal(rnd: random.Random) -> str:
    head = [rnd.randint(1, 9)] + [rnd.randint(0, 9) for _ in range(8)]
    return "".join(str(d) for d in head + [_inn_legal_check(head)])


def gen_ogrn(rnd: random.Random) -> str:
    # ОГРН: 1 символ (тип) + 2 (год) + 2 (регион) + 7 (порядковый) + 1 (контроль)
    body = [1] + [rnd.randint(0, 9) for _ in range(11)]
    body_str = "".join(str(d) for d in body)
    check = int(body_str) % 11 % 10
    return body_str + str(check)


def gen_passport(rnd: random.Random) -> tuple[str, str, str]:
    series = f"{rnd.randint(40, 49):02d} {rnd.randint(0, 99):02d}"
    number = f"{rnd.randint(100000, 999999)}"
    code = f"{rnd.randint(100, 999)}-{rnd.randint(100, 999)}"
    return series, number, code


def gen_address(rnd: random.Random) -> tuple[str, str]:
    city, tmpl = rnd.choice(CITIES)
    house = rnd.randint(1, 199)
    apt = rnd.randint(1, 250)
    return city, tmpl.format(house, apt)


def gen_date(rnd: random.Random, year_min: int, year_max: int) -> str:
    y = rnd.randint(year_min, year_max)
    m = rnd.randint(1, 12)
    days = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m - 1]
    d = rnd.randint(1, days)
    return f"{d:02d}.{m:02d}.{y}"


# ── Генерация кейсов ───────────────────────────────────────────────────────────


def make_passport_case(rnd: random.Random, idx: int) -> dict:
    is_male = rnd.random() < 0.55
    last = rnd.choice(LAST_NAMES) + ("" if is_male else "а")
    first = rnd.choice(FIRST_NAMES_M if is_male else FIRST_NAMES_F)
    middle = rnd.choice(MIDDLE_NAMES_M if is_male else MIDDLE_NAMES_F)
    series, number, code = gen_passport(rnd)
    issuer = rnd.choice(ISSUERS)
    issued_at = gen_date(rnd, 2008, 2024)
    birth_date = gen_date(rnd, 1960, 2002)
    _, address = gen_address(rnd)

    inn = gen_inn_legal(rnd)  # как для ИП

    # 10% кейсов — "грязные", чтобы проверить устойчивость парсинга:
    # пропадает поле, документ повёрнут и т.п.
    is_dirty = (idx % 10 == 9)
    expected = {
        "document_type": "passport",
        "fields": {
            "last_name": last,
            "first_name": first,
            "middle_name": middle,
            "series": series,
            "number": number,
            "issued_by": issuer,
            "issued_at": issued_at,
            "issued_code": code,
            "birth_date": birth_date,
            "registration_address": address,
        },
    }
    if is_dirty:
        # тяжёлый кейс: ожидаем, что агент сообщит о нечитаемом поле,
        # а не выдумает его. Поле помечено маркером, runner обработает по-особому.
        expected["fields"]["middle_name"] = "<unreadable>"
        expected["quality_warnings"] = ["low_resolution_or_skewed"]

    return {
        "id": f"docs-parsing-passport-{idx + 1:03d}",
        "agent": "document-intake",
        "input": {
            "document_type": "passport",
            "document_url": f"fixtures://passports/{idx + 1:03d}.png",
            "page_count": 1,
            "tenant_id": "tnt_demo",
        },
        "expected": expected,
        "tags": ["docs-parsing", "passport", "synthetic"] + (["adversarial"] if is_dirty else []),
        "description": (
            f"Паспорт РФ, {'мужской' if is_male else 'женский'}, "
            f"{address.split(',')[0]}"
            + (" — сложный кейс (низкое качество)" if is_dirty else "")
        ),
    }


def make_charter_case(rnd: random.Random, idx: int) -> dict:
    name = rnd.choice(COMPANY_NAMES) + (
        f"-{rnd.choice(['Сервис', 'Капитал', 'Групп', 'Инвест', 'Холдинг', 'Трейд', 'Логистик', ''])}"
        if rnd.random() < 0.6
        else ""
    )
    name = name.rstrip("-")

    # Уставной капитал: типовые значения
    capital_kop = rnd.choice([10_00000, 50_00000, 100_00000, 500_00000, 1_000_00000])
    inn = gen_inn_legal(rnd)
    ogrn = gen_ogrn(rnd)
    kpp = f"{inn[:4]}{rnd.randint(10, 99):02d}001"

    main_okved = rnd.choice(OKVEDS)
    additional = rnd.sample([o for o in OKVEDS if o != main_okved], k=rnd.randint(0, 3))

    director_last = rnd.choice(LAST_NAMES)
    director_first = rnd.choice(FIRST_NAMES_M)
    director_middle = rnd.choice(MIDDLE_NAMES_M)
    director_full = f"{director_last} {director_first} {director_middle}"

    city, address = gen_address(rnd)
    reg_date = gen_date(rnd, 2010, 2025)

    is_dirty = (idx % 10 == 9)
    expected = {
        "document_type": "charter",
        "fields": {
            "short_name": f"ООО «{name}»",
            "full_name": f"Общество с ограниченной ответственностью «{name}»",
            "inn": inn,
            "ogrn": ogrn,
            "kpp": kpp,
            "registration_date": reg_date,
            "legal_address": address,
            "director_full_name": director_full,
            "capital_kopecks": capital_kop,
            "okved_main": main_okved[0],
            "okved_additional": [o[0] for o in additional],
        },
    }
    if is_dirty:
        # тяжёлый кейс: устав со старой версией с уже неактуальным директором.
        # Агент должен извлечь и пометить как требующее сверки с ЕГРЮЛ.
        expected["quality_warnings"] = ["multiple_versions_detected", "outdated_director"]

    return {
        "id": f"docs-parsing-charter-{idx + 1:03d}",
        "agent": "document-intake",
        "input": {
            "document_type": "charter",
            "document_url": f"fixtures://charters/{idx + 1:03d}.pdf",
            "page_count": rnd.randint(8, 22),
            "tenant_id": "tnt_demo",
        },
        "expected": expected,
        "tags": ["docs-parsing", "charter", "ooo", "synthetic"] + (["adversarial"] if is_dirty else []),
        "description": f"Устав ООО «{name}», {city}" + (" — сложный кейс (старая редакция)" if is_dirty else ""),
    }


def main():
    rnd = random.Random(20260426)
    here = Path(__file__).parent

    passports = [make_passport_case(rnd, i) for i in range(25)]
    charters = [make_charter_case(rnd, i) for i in range(25)]

    (here / "passport-cases.json").write_text(
        json.dumps(passports, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    (here / "charter-cases.json").write_text(
        json.dumps(charters, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"Wrote {len(passports)} passport + {len(charters)} charter cases")


if __name__ == "__main__":
    main()
