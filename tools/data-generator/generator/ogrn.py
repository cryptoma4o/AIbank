"""OGRN / ОГРНИП generator with proper checksum.

OGRN (13 digits): last digit = (number_without_last % 11) % 10
ОГРНИП (15 digits): last digit = (number_without_last % 13) % 10
"""

from __future__ import annotations

import random


def generate_ogrn() -> str:
    """Generate a valid 13-digit OGRN for a legal entity."""
    # Digits 1-12: sign(1 or 5) + year(2) + region(2) + sequence(7)
    sign = random.choice([1, 5])
    year = random.randint(2, 23)
    region = random.randint(1, 99)
    sequence = random.randint(1, 9_999_999)
    base_str = f"{sign}{year:02d}{region:02d}{sequence:07d}"
    assert len(base_str) == 12
    n = int(base_str)
    control = (n % 11) % 10
    return base_str + str(control)


def generate_ogrnip() -> str:
    """Generate a valid 15-digit ОГРНИП for an individual entrepreneur."""
    sign = 3  # ОГРНИП always starts with 3
    year = random.randint(4, 23)
    region = random.randint(1, 99)
    sequence = random.randint(1, 999_999_999)
    base_str = f"{sign}{year:02d}{region:02d}{sequence:09d}"
    assert len(base_str) == 14
    n = int(base_str)
    control = (n % 13) % 10
    return base_str + str(control)


def validate_ogrn(ogrn: str) -> bool:
    """Validate an OGRN or ОГРНИП checksum."""
    if not ogrn.isdigit():
        return False
    if len(ogrn) == 13:
        return int(ogrn[-1]) == (int(ogrn[:12]) % 11) % 10
    if len(ogrn) == 15:
        return int(ogrn[-1]) == (int(ogrn[:14]) % 13) % 10
    return False
