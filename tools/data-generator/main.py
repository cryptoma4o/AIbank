"""CLI entry point for the AIbank data generator.

Usage:
    python -m tools.data_generator generate inn --type legal
    python -m tools.data_generator generate inn --type individual
    python -m tools.data_generator generate ogrn --type org
    python -m tools.data_generator generate ogrn --type ip
    python -m tools.data_generator generate passport --output /tmp/passport.pdf
    python -m tools.data_generator generate passport --output /tmp/ --count 5
"""

from __future__ import annotations

import json
import random
import sys
from pathlib import Path

import click

# Add this file's directory to sys.path so sibling modules are importable
# both when run directly (python main.py) and as a module (-m tools.data_generator.main).
_THIS_DIR = Path(__file__).resolve().parent
if str(_THIS_DIR) not in sys.path:
    sys.path.insert(0, str(_THIS_DIR))

from inn import generate_inn_individual, generate_inn_legal, validate_inn  # noqa: E402
from ogrn import generate_ogrn, generate_ogrnip, validate_ogrn  # noqa: E402
from passport import generate_passport_pdf  # noqa: E402


@click.group()
def cli() -> None:
    """AIbank synthetic data generator. All output is fully synthetic."""


@cli.group()
def generate() -> None:
    """Generate synthetic Russian banking identifiers and documents."""


@generate.command("inn")
@click.option(
    "--type",
    "inn_type",
    type=click.Choice(["legal", "individual"], case_sensitive=False),
    default="legal",
    show_default=True,
    help="INN type: 'legal' (10-digit) or 'individual' (12-digit)",
)
@click.option("--count", default=1, show_default=True, help="Number of INNs to generate")
@click.option("--seed", default=42, show_default=True, help="Random seed for reproducibility")
@click.option("--json", "as_json", is_flag=True, default=False, help="Output as JSON array")
def cmd_inn(inn_type: str, count: int, seed: int, as_json: bool) -> None:
    """Generate INN (ИНН) with correct FNS checksum."""
    rng = random.Random(seed)
    results = []
    for _ in range(count):
        if inn_type == "legal":
            value = generate_inn_legal(rng)
        else:
            value = generate_inn_individual(rng)
        results.append({"inn": value, "type": inn_type, "valid": validate_inn(value)})

    if as_json:
        click.echo(json.dumps(results, ensure_ascii=False, indent=2))
    else:
        for r in results:
            click.echo(f"{r['inn']}  ({r['type']}, valid={r['valid']})")


@generate.command("ogrn")
@click.option(
    "--type",
    "ogrn_type",
    type=click.Choice(["org", "ip"], case_sensitive=False),
    default="org",
    show_default=True,
    help="OGRN type: 'org' (13-digit) or 'ip' (15-digit OGRNIP)",
)
@click.option("--count", default=1, show_default=True, help="Number of OGRNs to generate")
@click.option("--seed", default=42, show_default=True, help="Random seed for reproducibility")
@click.option("--json", "as_json", is_flag=True, default=False, help="Output as JSON array")
def cmd_ogrn(ogrn_type: str, count: int, seed: int, as_json: bool) -> None:
    """Generate OGRN (ОГРН) / OGRNIP with correct checksum."""
    rng = random.Random(seed)
    results = []
    for _ in range(count):
        if ogrn_type == "org":
            value = generate_ogrn(rng)
        else:
            value = generate_ogrnip(rng)
        results.append({"ogrn": value, "type": ogrn_type, "valid": validate_ogrn(value)})

    if as_json:
        click.echo(json.dumps(results, ensure_ascii=False, indent=2))
    else:
        for r in results:
            click.echo(f"{r['ogrn']}  ({r['type']}, valid={r['valid']})")


@generate.command("passport")
@click.option(
    "--output",
    required=True,
    help="Output path: a .pdf file, or a directory (files named passport_NNN.pdf)",
)
@click.option("--count", default=1, show_default=True, help="Number of passports to generate")
@click.option("--seed", default=42, show_default=True, help="Random seed for reproducibility")
@click.option("--json", "as_json", is_flag=True, default=False, help="Print metadata as JSON")
def cmd_passport(output: str, count: int, seed: int, as_json: bool) -> None:
    """Generate synthetic Russian passport PDF(s)."""
    rng = random.Random(seed)
    output_path = Path(output)
    records = []

    for i in range(count):
        if output_path.suffix.lower() == ".pdf" and count == 1:
            pdf_path = output_path
        else:
            output_path.mkdir(parents=True, exist_ok=True)
            pdf_path = output_path / f"passport_{i + 1:03d}.pdf"

        data = generate_passport_pdf(pdf_path, rng=rng)
        record = {
            "file": str(pdf_path),
            "series": data.series,
            "number": data.number,
            "last_name": data.last_name,
            "first_name": data.first_name,
            "middle_name": data.middle_name,
            "dob": data.dob,
            "gender": data.gender,
            "issue_date": data.issue_date,
            "issued_by": data.issued_by,
            "division_code": data.division_code,
        }
        records.append(record)

        if not as_json:
            click.echo(
                f"[{i + 1}/{count}] {pdf_path}  "
                f"{data.last_name} {data.first_name}, {data.dob}"
            )

    if as_json:
        click.echo(json.dumps(records, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    cli()
