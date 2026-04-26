"""CLI entry point for the AIbank data generator (generator subpackage)."""

import click
import json

from .company import generate_company
from .inn import generate_inn_10, generate_inn_12, validate_inn
from .ogrn import generate_ogrn, validate_ogrn


@click.group()
def main():
    """AIbank synthetic data generator."""
    pass


@main.command()
@click.option("--count", default=10, help="Number of companies to generate")
@click.option("--output", default="-", help="Output file (default: stdout)")
def companies(count: int, output: str):
    """Generate synthetic Russian company data."""
    data = [generate_company() for _ in range(count)]
    text = json.dumps(data, ensure_ascii=False, indent=2)
    if output == "-":
        click.echo(text)
    else:
        with open(output, "w", encoding="utf-8") as f:
            f.write(text)
        click.echo(f"Written {count} companies to {output}")


@main.command()
@click.option("--type", "inn_type", default="10", type=click.Choice(["10", "12"]))
@click.option("--count", default=5)
def inns(inn_type: str, count: int):
    """Generate valid INN numbers."""
    for _ in range(count):
        inn = generate_inn_10() if inn_type == "10" else generate_inn_12()
        click.echo(inn)


@main.command()
@click.argument("inn")
def validate_inn_cmd(inn: str):
    """Validate an INN checksum."""
    if validate_inn(inn):
        click.echo(f"✓ {inn} is valid")
    else:
        click.echo(f"✗ {inn} is invalid", err=True)
        raise SystemExit(1)


@main.command()
@click.option("--count", default=5)
def ogrns(count: int):
    """Generate valid OGRN numbers."""
    for _ in range(count):
        click.echo(generate_ogrn())
