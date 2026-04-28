# aibank-proto (Python)

Generated Python gRPC clients and message types for the AIbank platform
canonical contracts. Produced by `buf generate` (see
`packages/proto/buf.gen.yaml`).

Install (from monorepo root, editable):

```bash
pip install -e packages/proto/generated/python
```

Until `buf generate` has run in CI, this package contains only the package
metadata and an empty `aibank_proto/__init__.py`. Once codegen produces
`*_pb2.py` / `*_pb2_grpc.py` modules they are committed under
`aibank_proto/aibank/...` mirroring the canonical proto package paths.
