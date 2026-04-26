"""INN (ИНН) generator with proper ФНС checksum algorithm.

INN 10 (ЮЛ): weights = [2,4,10,3,5,9,4,6,8], digit9 = sum(w*d) % 11 % 10
INN 12 (ФЛ/ИП):
  weights1 = [7,2,4,10,3,5,9,4,6,8], digit11 = sum(w*d) % 11 % 10
  weights2 = [3,7,2,4,10,3,5,9,4,6,8], digit12 = sum(w*d) % 11 % 10
"""

from __future__ import annotations

import random

_WEIGHTS_10 = [2, 4, 10, 3, 5, 9, 4, 6, 8]
_WEIGHTS_12_C1 = [7, 2, 4, 10, 3, 5, 9, 4, 6, 8]
_WEIGHTS_12_C2 = [3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8]


def _control_digit(digits: list[int], weights: list[int]) -> int:
    return (sum(d * w for d, w in zip(digits, weights)) % 11) % 10


def generate_inn_10() -> str:
    """Generate a valid 10-digit INN for a legal entity (ЮЛ)."""
    base = [random.randint(0, 9) for _ in range(9)]
    control = _control_digit(base, _WEIGHTS_10)
    return "".join(map(str, base + [control]))


def generate_inn_12() -> str:
    """Generate a valid 12-digit INN for an individual/ИП (ФЛ/ИП)."""
    base = [random.randint(0, 9) for _ in range(10)]
    c1 = _control_digit(base, _WEIGHTS_12_C1)
    c2 = _control_digit(base + [c1], _WEIGHTS_12_C2)
    return "".join(map(str, base + [c1, c2]))


def validate_inn(inn: str) -> bool:
    """Validate an INN checksum. Returns True if valid."""
    if not inn.isdigit():
        return False
    d = [int(c) for c in inn]
    if len(d) == 10:
        return d[9] == _control_digit(d[:9], _WEIGHTS_10)
    if len(d) == 12:
        c1 = _control_digit(d[:10], _WEIGHTS_12_C1)
        c2 = _control_digit(d[:10] + [c1], _WEIGHTS_12_C2)
        return d[10] == c1 and d[11] == c2
    return False
