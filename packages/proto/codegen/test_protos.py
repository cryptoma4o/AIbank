"""Tests for the canonical AIbank gRPC contracts.

These tests validate the *.proto sources and the codegen toolchain without
themselves invoking buf — the generation step runs in CI where the
bufbuild/buf-action provides the binary. Locally, tests skip gracefully when
buf or the Go toolchain is unavailable, mirroring the pattern in
``packages/domain-model/codegen/test_generators.py``.

Verified properties:
  * The 6 canonical .proto files exist at the package root.
  * Each declares a proto3 ``package aibank...`` statement.
  * Each declares at least one ``service`` block (gRPC, not just messages).
  * `buf lint` exits 0 when buf is installed (skipped otherwise).
  * Generated Go module has a valid go.mod and passes `go vet ./...` after
    codegen has run (skipped if codegen has not yet produced *.pb.go files).
"""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
PACKAGE_ROOT = HERE.parent

# Canonical .proto files per ADR-0006 (one per bounded context).
CANONICAL_PROTOS = (
    "tenant.proto",
    "identity.proto",
    "document.proto",
    "onboarding.proto",
    "risk.proto",
    "audit.proto",
)

PACKAGE_DECL_RE = re.compile(r"^\s*package\s+(aibank(?:\.[a-z0-9_]+)*)\s*;", re.MULTILINE)
SERVICE_DECL_RE = re.compile(r"^\s*service\s+([A-Z][A-Za-z0-9_]*)\s*\{", re.MULTILINE)


@pytest.fixture(scope="module")
def proto_files() -> list[Path]:
    return [PACKAGE_ROOT / name for name in CANONICAL_PROTOS]


def test_six_canonical_protos_exist(proto_files: list[Path]) -> None:
    missing = [p for p in proto_files if not p.is_file()]
    assert not missing, (
        "Missing canonical proto files: "
        + ", ".join(str(p.relative_to(PACKAGE_ROOT)) for p in missing)
    )


def test_each_proto_has_aibank_package(proto_files: list[Path]) -> None:
    for proto in proto_files:
        text = proto.read_text(encoding="utf-8")
        match = PACKAGE_DECL_RE.search(text)
        assert match is not None, f"{proto.name} missing 'package aibank...' declaration"
        # Either aibank.v1 (current flat layout) or aibank.<context>(.vN) is allowed.
        pkg = match.group(1)
        assert pkg.startswith("aibank"), f"{proto.name} package must start with 'aibank' (got {pkg!r})"


def test_each_proto_declares_a_service(proto_files: list[Path]) -> None:
    """Sanity check: every canonical .proto exposes at least one gRPC service.

    Per ADR-0006, the canonical contract IS the gRPC service surface — files
    that only carry messages are not contracts and don't belong here.
    """
    services_per_file: dict[str, list[str]] = {}
    for proto in proto_files:
        text = proto.read_text(encoding="utf-8")
        services = SERVICE_DECL_RE.findall(text)
        services_per_file[proto.name] = services
    missing = [name for name, svcs in services_per_file.items() if not svcs]
    assert not missing, (
        "These canonical protos declare no service: "
        + ", ".join(missing)
        + f"\n(found: {services_per_file})"
    )


def test_buf_lint_passes_when_buf_installed() -> None:
    if shutil.which("buf") is None:
        pytest.skip("buf not installed; CI runs buf lint via bufbuild/buf-action")

    res = subprocess.run(
        ["buf", "lint"],
        cwd=PACKAGE_ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    assert res.returncode == 0, (
        "buf lint failed:\nstdout=" + res.stdout + "\nstderr=" + res.stderr
    )


def test_generated_go_module_declared() -> None:
    """The committed go.mod scaffolding must declare the expected module path.

    Downstream services import as
    ``github.com/aibank/platform/packages/proto/generated/go``; if this path
    drifts, every consumer breaks.
    """
    go_mod = PACKAGE_ROOT / "generated" / "go" / "go.mod"
    assert go_mod.exists(), "generated/go/go.mod missing"
    text = go_mod.read_text(encoding="utf-8")
    assert "module github.com/aibank/platform/packages/proto/generated/go" in text


def test_generated_go_passes_vet_when_toolchain_available() -> None:
    if shutil.which("go") is None:
        pytest.skip("go toolchain unavailable")

    go_dir = PACKAGE_ROOT / "generated" / "go"
    if not (go_dir / "go.mod").exists():
        pytest.skip("generated/go/go.mod missing — codegen has not yet run")

    # Skip if codegen has not produced any *.pb.go files yet — otherwise
    # `go vet` walks an empty module which is fine, but we still gate on the
    # presence of the module declaration only (already covered above).
    res = subprocess.run(
        ["go", "vet", "./..."],
        cwd=go_dir,
        capture_output=True,
        text=True,
        check=False,
        env={"GOFLAGS": "-mod=mod", "PATH": __import__("os").environ.get("PATH", ""),
             "HOME": __import__("os").environ.get("HOME", ""),
             "GOCACHE": __import__("os").environ.get("GOCACHE", ""),
             "GOPATH": __import__("os").environ.get("GOPATH", "")},
    )
    # Tolerate "no Go files" (codegen not run) but fail on real vet errors.
    if res.returncode != 0 and "no Go files" not in res.stderr and "no Go files" not in res.stdout:
        pytest.fail("go vet failed:\nstdout=" + res.stdout + "\nstderr=" + res.stderr)


def test_python_pyproject_declares_aibank_proto() -> None:
    pyproject = PACKAGE_ROOT / "generated" / "python" / "pyproject.toml"
    assert pyproject.exists(), "generated/python/pyproject.toml missing"
    text = pyproject.read_text(encoding="utf-8")
    assert 'name = "aibank-proto"' in text
    assert "grpcio" in text
    assert "protobuf" in text


def test_ts_package_json_declares_aibank_proto() -> None:
    pkg = PACKAGE_ROOT / "generated" / "ts" / "package.json"
    assert pkg.exists(), "generated/ts/package.json missing"
    text = pkg.read_text(encoding="utf-8")
    assert '"@aibank/proto"' in text
