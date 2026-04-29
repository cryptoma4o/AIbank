module github.com/aibank/platform/tools/audit-verifier

go 1.23

require (
	github.com/aibank/platform/packages/signature v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.10.9
)

replace github.com/aibank/platform/packages/signature => ../../packages/signature
