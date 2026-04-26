# Data Generator

Synthetic test data generator for AIbank evaluation datasets. All output is fully synthetic —
no real personal data is used. Fixed seed (42) ensures reproducible output.

## Purpose

Generates realistic-looking Russian banking documents and identifiers for:
- Populating eval-harness datasets
- Load testing document parsing agents
- Development and QA without touching production data

## Generators

| Module | What it generates |
|--------|-------------------|
| `inn.py` | INN (ИНН) — 10-digit (legal entity) or 12-digit (individual) with correct FNS checksum |
| `ogrn.py` | OGRN (ОГРН) — 13-digit for organisations, 15-digit for IP, with mod-11 checksum |
| `passport.py` | Synthetic Russian passport PDF (series, number, name, DOB, issuing department) |

## Usage

```bash
# Generate a legal entity INN
python -m tools.data_generator generate inn --type legal

# Generate an individual INN
python -m tools.data_generator generate inn --type individual

# Generate an OGRN for an organisation
python -m tools.data_generator generate ogrn --type org

# Generate an OGRN for an individual entrepreneur (IP)
python -m tools.data_generator generate ogrn --type ip

# Generate a passport PDF
python -m tools.data_generator generate passport --output /tmp/passport.pdf

# Generate multiple passports
python -m tools.data_generator generate passport --output /tmp/ --count 10
```

## Reproducibility

All generators accept `--seed` (default: 42). Same seed = same output.

## Installation

```bash
pip install -e tools/data-generator
```
