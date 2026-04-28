// Local Go module for the codegen smoke-import sample. A `replace` directive
// points at the sibling `generated/go` module so `go build` resolves the
// import without needing a published version of the generated package.
//
// This module is build-tag-gated (`protocodegen_smoke`) so it does NOT
// participate in the workspace-wide `go build ./...` walk by default — only
// CI's smoke job (`go build -tags=protocodegen_smoke ./codegen/...`) reaches
// inside.

module github.com/aibank/platform/packages/proto/codegen

go 1.22

require github.com/aibank/platform/packages/proto/generated/go v0.0.0

replace github.com/aibank/platform/packages/proto/generated/go => ../generated/go
