"""Synthetic Russian company data generator using Faker."""

from __future__ import annotations

import random
import datetime

from faker import Faker

from .inn import generate_inn_10
from .ogrn import generate_ogrn

fake = Faker("ru_RU")

LEGAL_FORMS = ["ООО", "АО", "ПАО", "ЗАО"]
OKVED_CODES = [
    "62.01", "62.02", "63.11", "64.19", "66.19",
    "46.90", "47.91", "69.10", "70.22", "74.90",
]


def generate_company() -> dict:
    """Generate a synthetic Russian company record."""
    name = fake.company()
    legal_form = random.choice(LEGAL_FORMS)
    return {
        "legal_form": legal_form,
        "full_name": f'{legal_form} "{name}"',
        "short_name": f'{legal_form} "{name[:20]}"',
        "inn": generate_inn_10(),
        "ogrn": generate_ogrn(),
        "kpp": (
            f"{random.randint(100, 999)}"
            f"{random.randint(100, 999)}"
            f"{random.randint(100, 999):03d}"
        ),
        "legal_address": fake.address(),
        "okved_primary": random.choice(OKVED_CODES),
        "registration_date": fake.date_between(
            start_date="-10y", end_date="-1y"
        ).isoformat(),
        "status": "active",
        "ceo_last_name": fake.last_name(),
        "ceo_first_name": fake.first_name(),
        "ceo_middle_name": fake.middle_name(),
    }
