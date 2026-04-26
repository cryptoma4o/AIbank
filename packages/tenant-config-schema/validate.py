#!/usr/bin/env python3
import sys, json, yaml, jsonschema
from pathlib import Path

def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <config.yaml>")
        sys.exit(1)

    schema_path = Path(__file__).parent / "schema.json"
    schema = json.loads(schema_path.read_text())
    config = yaml.safe_load(Path(sys.argv[1]).read_text())

    try:
        jsonschema.validate(config, schema)
        print(f"✓ {sys.argv[1]} is valid")
    except jsonschema.ValidationError as e:
        print(f"✗ Validation error: {e.message}")
        print(f"  Path: {' > '.join(str(p) for p in e.path)}")
        sys.exit(1)

if __name__ == "__main__":
    main()
