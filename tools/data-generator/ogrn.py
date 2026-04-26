"""OGRN (ОГРН) generator with correct checksum.

OGRN format (13 digits, for organisations):
  - Digit 1: sign (3 = current year 20xx, 1 = 19xx – legacy, use 1 or 3)
  - Digits 2-3: year (last 2 digits)
  - Digits 4-5: region code (01–99)
  - Digits 6-12: sequence number in FNS register (7 digits)
  - Digit 13: control digit
    control = (N mod 11), if result == 10 then control = 0
    where N = first 12 digits as integer

OGRNIP format (15 digits, for individual entrepreneurs):
  - Same structure but 15 digits total, divisor is 13 (not 11)
  - control = (N mod 13) % 10, where N = first 14 digits

Reference: Federal Law No. 129-FZ "On State Registration of Legal Entities and IE".
"""

from __future__ import annotations

import random

_REGION_CODES = list(range(1, 100))


def _ogrn_control(base_digits: list[int], divisor: int) -> int:
    n = int("".join(map(str, base_digits)))
    remainder = n % divisor
    return remainder if remainder != 10 else 0


def generate_ogrn(rng: random.Random | None = None) -> str:
    """Generate a valid 13-digit OGRN for a legal entity."""
    rng = rng or random.Random(42)

    sign = rng.choice([1, 5])  # 1 = primary registration, 5 = re-registration
    year = rng.randint(2, 23)  # 2002–2023
    region = rng.choice(_REGION_CODES)
    sequence = rng.randint(1, 9999999)

    base = (
        [sign]
        + [year // 10, year % 10]
        + [region // 10, region % 10]
        + list(map(int, f"{sequence:07d}"))
    )  # 12 digits

    control = _ogrn_control(base, divisor=11)
    digits = base + [control]
    result = "".join(map(str, digits))
    assert len(result) == 13, f"OGRN length error: {result}"
    return result


def generate_ogrnip(rng: random.Random | None = None) -> str:
    """Generate a valid 15-digit OGRNIP for an individual entrepreneur (IP)."""
    rng = rng or random.Random(42)

    sign = 3  # OGRNIP always starts with 3
    year = rng.randint(4, 23)  # 2004–2023 (IEs registered since 2004 reform)
    region = rng.choice(_REGION_CODES)
    sequence = rng.randint(1, 999999999)

    base = (
        [sign]
        + [year // 10, year % 10]
        + [region // 10, region % 10]
        + list(map(int, f"{sequence:09d}"))
    )  # 14 digits

    control = _ogrn_control(base, divisor=13)
    digits = base + [control]
    result = "".join(map(str, digits))
    assert len(result) == 15, f"OGRNIP length error: {result}"
    return result


def validate_ogrn(ogrn: str) -> bool:
    """Return True if the OGRN/OGRNIP string has a valid checksum."""
    if not ogrn.isdigit():
        return False
    d = [int(c) for c in ogrn]

    if len(d) == 13:
        expected = _ogrn_control(d[:12], divisor=11)
        return d[12] == expected

    if len(d) == 15:
        expected = _ogrn_control(d[:14], divisor=13)
        return d[14] == expected

    return False


if __name__ == "__main__":
    rng = random.Random(42)
    ogrn = generate_ogrn(rng)
    ogrnip = generate_ogrnip(rng)
    print(f"OGRN (13-digit):   {ogrn}  valid={validate_ogrn(ogrn)}")
    print(f"OGRNIP (15-digit): {ogrnip}  valid={validate_ogrn(ogrnip)}")
