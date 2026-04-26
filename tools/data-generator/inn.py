"""INN (ИНН) generator with correct FNS checksum algorithm.

INN format:
  - 10 digits for legal entities (юридические лица):
      digits 1-9: base number, digit 10: control digit
      control = (sum(d[i] * w[i] for i in 0..8) % 11) % 10
      weights = [2, 4, 10, 3, 5, 9, 4, 6, 8]

  - 12 digits for individuals (физические лица):
      digits 1-10: base number, digits 11-12: two control digits
      control1 weights = [7, 2, 4, 10, 3, 5, 9, 4, 6, 8]
      control2 weights = [3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8]
      c1 = (sum(d[i] * w1[i] for i in 0..9) % 11) % 10
      c2 = (sum(d[i] * w2[i] for i in 0..10) % 11) % 10  (includes c1 at index 10)

Reference: FNS Order No. ММВ-7-6/435@ dated 2012-11-29.
"""

from __future__ import annotations

import random

_WEIGHTS_10 = [2, 4, 10, 3, 5, 9, 4, 6, 8]
_WEIGHTS_12_C1 = [7, 2, 4, 10, 3, 5, 9, 4, 6, 8]
_WEIGHTS_12_C2 = [3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8]

# Valid region codes (1–99, excluding gaps). Simplified to common ones.
_REGION_CODES = list(range(1, 100))


def _control_digit(digits: list[int], weights: list[int]) -> int:
    total = sum(d * w for d, w in zip(digits, weights))
    return (total % 11) % 10


def generate_inn_legal(rng: random.Random | None = None) -> str:
    """Generate a valid 10-digit INN for a legal entity."""
    rng = rng or random.Random(42)

    # Region code: first two digits (01–99)
    region = rng.choice(_REGION_CODES)
    region_digits = [region // 10, region % 10]

    # Intra-region sequence: 7 random digits
    seq = [rng.randint(0, 9) for _ in range(7)]

    base = region_digits + seq  # 9 digits
    control = _control_digit(base, _WEIGHTS_10)
    digits = base + [control]

    return "".join(map(str, digits))


def generate_inn_individual(rng: random.Random | None = None) -> str:
    """Generate a valid 12-digit INN for an individual."""
    rng = rng or random.Random(42)

    region = rng.choice(_REGION_CODES)
    region_digits = [region // 10, region % 10]

    # 8 random sequence digits → base = 10 digits total
    seq = [rng.randint(0, 9) for _ in range(8)]
    base = region_digits + seq  # 10 digits

    c1 = _control_digit(base, _WEIGHTS_12_C1)
    c2 = _control_digit(base + [c1], _WEIGHTS_12_C2)

    digits = base + [c1, c2]
    return "".join(map(str, digits))


def validate_inn(inn: str) -> bool:
    """Return True if the INN string has a valid FNS checksum."""
    if not inn.isdigit():
        return False

    d = [int(c) for c in inn]

    if len(d) == 10:
        expected = _control_digit(d[:9], _WEIGHTS_10)
        return d[9] == expected

    if len(d) == 12:
        c1 = _control_digit(d[:10], _WEIGHTS_12_C1)
        c2 = _control_digit(d[:10] + [c1], _WEIGHTS_12_C2)
        return d[10] == c1 and d[11] == c2

    return False


if __name__ == "__main__":
    rng = random.Random(42)
    legal = generate_inn_legal(rng)
    individual = generate_inn_individual(rng)
    print(f"Legal INN (10-digit):     {legal}  valid={validate_inn(legal)}")
    print(f"Individual INN (12-digit): {individual}  valid={validate_inn(individual)}")
